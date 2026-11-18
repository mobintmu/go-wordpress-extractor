// Package aicrawler talks to an LLM over an OpenAI-compatible API (AvalAI
// by default) for the "ai-crawler-avalai" engine: it cleans a product page,
// builds the prompts, sends them with the official openai-go SDK and parses
// the model's JSON. It does not fetch pages and does not decide what goes in
// a response; the fetch and extract packages do that.
package aicrawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-wordpress/internal/diagnose"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// Response formats, as set in APP_AI_CRAWLER_RESPONSE_FORMAT.
const (
	FormatJSONObject = "json_object"
	FormatJSONSchema = "json_schema"
)

// Settings configure the client. Production fills them from APP_AI_CRAWLER_*.
type Settings struct {
	BaseURL         string
	Model           string
	APIKey          string
	Timeout         time.Duration
	MaxOutputTokens int
	Temperature     float64
	Retries         int
	MinInterval     time.Duration
	ResponseFormat  string
	// HTTPClient replaces the SDK's HTTP client, for tests. Optional.
	HTTPClient *http.Client
	// Observe, when set, is called after every model call and cache hit,
	// for the /metrics counters. Optional.
	Observe func(Call)
}

// Purposes of a model call, for metrics.
const (
	PurposeExtract = "extract" // first product extraction call
	PurposeRepair  = "repair"  // second call after an answer that was not JSON
	PurposeLearn   = "learn"   // selector learning
)

// Call describes one model call (or one cache hit) for metrics.
type Call struct {
	Model   string
	Purpose string
	Host    string
	// Outcome is "ok", or the failure code (AI_RATE_LIMITED, ...).
	Outcome      string
	InputTokens  int64
	OutputTokens int64
	Duration     time.Duration
	// Cached is true for an answer served from the cache: no call was made
	// and no tokens were used.
	Cached bool
}

type callKey struct{}

type callInfo struct{ purpose, host string }

// WithCall labels the model calls made with ctx for metrics.
func WithCall(ctx context.Context, purpose, host string) context.Context {
	return context.WithValue(ctx, callKey{}, callInfo{purpose, host})
}

func infoOf(ctx context.Context) callInfo {
	if ci, ok := ctx.Value(callKey{}).(callInfo); ok {
		return ci
	}
	return callInfo{purpose: PurposeExtract}
}

// CallHost is the host ctx's calls are labeled with.
func CallHost(ctx context.Context) string { return infoOf(ctx).host }

// CacheHit records an answer served from the cache.
func (c *Client) CacheHit(ctx context.Context) {
	if c.s.Observe == nil {
		return
	}
	ci := infoOf(ctx)
	c.s.Observe(Call{Model: c.s.Model, Purpose: ci.purpose, Host: ci.host, Outcome: "ok", Cached: true})
}

// Message is one chat message.
type Message struct {
	Role    string // "system", "user" or "assistant"
	Content string
}

// Completion is the model's answer to one call.
type Completion struct {
	Content      string
	FinishReason string
	Refusal      string
	RequestID    string
	Status       int
	InputTokens  int64
	OutputTokens int64
}

// CallError is a failed call, already mapped to a failure code.
type CallError struct {
	// Code is a diagnose code: AI_UNAVAILABLE, AI_RATE_LIMITED, ...
	Code         string
	Status       int
	ProviderCode string
	RequestID    string
	// Message is safe to show: it never holds the API key.
	Message string
}

func (e *CallError) Error() string { return e.Code + ": " + e.Message }

// Client sends chat completions. It is safe for concurrent use.
type Client struct {
	oc openai.Client
	s  Settings

	gateMu sync.Mutex
	next   time.Time
}

// New builds a client. The key and base URL are passed explicitly, so the
// SDK's own OPENAI_API_KEY and OPENAI_BASE_URL variables are never used.
func New(s Settings) *Client {
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(s.BaseURL, "/") + "/"),
		option.WithAPIKey(s.APIKey),
		option.WithMaxRetries(s.Retries),
	}
	if s.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(s.Timeout))
	}
	if s.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(s.HTTPClient))
	}
	return &Client{oc: openai.NewClient(opts...), s: s}
}

// Model is the only model this client uses.
func (c *Client) Model() string { return c.s.Model }

// wait spaces calls by MinInterval, so the provider's per-minute limit is
// not reached by bursts.
func (c *Client) wait(ctx context.Context) error {
	if c.s.MinInterval <= 0 {
		return nil
	}
	c.gateMu.Lock()
	now := time.Now()
	at := c.next
	if at.Before(now) {
		at = now
	}
	c.next = at.Add(c.s.MinInterval)
	c.gateMu.Unlock()
	d := time.Until(at)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Complete sends msgs and returns the first choice. schema, when set and
// the response format is json_schema, is sent as a strict JSON schema;
// otherwise the model is asked for a JSON object. Retries of 408, 409, 429
// and 5xx answers (honoring Retry-After) are done by the SDK.
func (c *Client) Complete(ctx context.Context, msgs []Message, schemaName string, schema map[string]any) (*Completion, *CallError) {
	start := time.Now()
	comp, cerr := c.complete(ctx, msgs, schemaName, schema)
	if c.s.Observe != nil {
		ci := infoOf(ctx)
		call := Call{Model: c.s.Model, Purpose: ci.purpose, Host: ci.host, Outcome: "ok", Duration: time.Since(start)}
		if comp != nil {
			call.InputTokens, call.OutputTokens = comp.InputTokens, comp.OutputTokens
		}
		if cerr != nil {
			call.Outcome = cerr.Code
		}
		c.s.Observe(call)
	}
	return comp, cerr
}

func (c *Client) complete(ctx context.Context, msgs []Message, schemaName string, schema map[string]any) (*Completion, *CallError) {
	if err := c.wait(ctx); err != nil {
		return nil, &CallError{Code: diagnose.AIUnavailable, Message: "the request's time limit was reached while waiting for the AI rate gate"}
	}
	params := openai.ChatCompletionNewParams{
		Model:       c.s.Model,
		Temperature: openai.Float(c.s.Temperature),
		MaxTokens:   openai.Int(int64(c.s.MaxOutputTokens)),
	}
	for _, m := range msgs {
		switch m.Role {
		case "system":
			params.Messages = append(params.Messages, openai.SystemMessage(m.Content))
		case "assistant":
			params.Messages = append(params.Messages, openai.AssistantMessage(m.Content))
		default:
			params.Messages = append(params.Messages, openai.UserMessage(m.Content))
		}
	}
	if c.s.ResponseFormat == FormatJSONSchema && schema != nil {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   schemaName,
				Strict: openai.Bool(true),
				Schema: schema,
			}},
		}
	} else {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	var httpResp *http.Response
	res, err := c.oc.Chat.Completions.New(ctx, params, option.WithResponseInto(&httpResp))
	reqID := requestID(httpResp)
	if err != nil {
		return nil, c.mapError(ctx, err, reqID)
	}
	out := &Completion{RequestID: reqID, InputTokens: res.Usage.PromptTokens, OutputTokens: res.Usage.CompletionTokens}
	if httpResp != nil {
		out.Status = httpResp.StatusCode
	}
	if len(res.Choices) == 0 {
		return out, &CallError{Code: diagnose.AIBadOutput, Status: out.Status, RequestID: reqID, Message: "the provider answered with no choices"}
	}
	ch := res.Choices[0]
	out.Content, out.FinishReason, out.Refusal = ch.Message.Content, ch.FinishReason, ch.Message.Refusal
	return out, nil
}

// requestID reads the provider's request id for support tickets.
func requestID(r *http.Response) string {
	if r == nil {
		return ""
	}
	for _, h := range []string{"avalai-request-id", "x-request-id"} {
		if v := r.Header.Get(h); v != "" {
			return v
		}
	}
	return ""
}

// mapError turns an SDK error into a failure code. AvalAI's codes are
// documented at docs.avalai.ir (error handling); other OpenAI-compatible
// providers use the same names for the common cases.
func (c *Client) mapError(ctx context.Context, err error, reqID string) *CallError {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		ce := &CallError{Status: apiErr.StatusCode, ProviderCode: apiErr.Code, RequestID: reqID}
		if ce.RequestID == "" {
			ce.RequestID = requestID(apiErr.Response)
		}
		msg := strings.TrimSpace(apiErr.Message)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		code := strings.ToLower(apiErr.Code)
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
			ce.Code = diagnose.AIAuthFailed
		case apiErr.StatusCode == http.StatusTooManyRequests && (code == "insufficient_quota" || code == "quota_exceeded"):
			ce.Code = diagnose.AIQuotaExceeded
		case apiErr.StatusCode == http.StatusTooManyRequests:
			ce.Code = diagnose.AIRateLimited
		case code == "context_window_exceeded" || code == "context_length_exceeded" ||
			apiErr.StatusCode == http.StatusRequestEntityTooLarge:
			ce.Code = diagnose.AIInputTooLarge
		default:
			ce.Code = diagnose.AIUnavailable
		}
		ce.Message = fmt.Sprintf("the AI provider answered HTTP %d (%s): %s", apiErr.StatusCode, orDash(apiErr.Code), orDash(msg))
		return ce
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return &CallError{Code: diagnose.AIUnavailable, RequestID: reqID, Message: "the AI provider did not answer in time"}
	}
	return &CallError{Code: diagnose.AIUnavailable, RequestID: reqID, Message: "the AI provider could not be reached: " + redact(err.Error(), c.s.APIKey)}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// redact removes the API key from s, in case a transport error echoes a header.
func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[redacted]")
}
