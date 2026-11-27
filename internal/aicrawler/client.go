// Package aicrawler talks to an LLM over an OpenAI-compatible API for the
// AI engines: "ai-crawler-avalai" (AvalAI) and "ai-crawler-cloudflare"
// (Cloudflare AI Gateway). It cleans a product page, builds the prompts,
// sends them with the official openai-go SDK and parses the model's JSON.
// What differs between providers is kept in a Provider profile. It does not
// fetch pages and does not decide what goes in a response; the fetch and
// extract packages do that.
package aicrawler

import (
	"context"
	"encoding/json"
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

// Settings configure the client, whatever the provider. Production fills
// them from the shared APP_AI_CRAWLER_* settings and the provider's own
// minimum interval.
type Settings struct {
	Timeout         time.Duration
	MaxOutputTokens int
	Temperature     float64
	Retries         int
	MinInterval     time.Duration
	ResponseFormat  string
	// HTTPClient replaces the SDK's HTTP client, for tests. Optional.
	HTTPClient *http.Client
	// Middleware can inspect individual provider requests in local diagnostics.
	Middleware option.Middleware
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
	// Engine is ai-crawler-avalai or ai-crawler-cloudflare.
	Engine  string
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
	c.s.Observe(Call{Engine: c.p.Engine, Model: c.p.Model, Purpose: ci.purpose, Host: ci.host, Outcome: "ok", Cached: true})
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
	// Model is the model that answered when the provider says so
	// (cf-aig-model, for a Cloudflare dynamic route); "" otherwise.
	Model string
	// GatewayCache is Cloudflare's cf-aig-cache-status (HIT or MISS).
	GatewayCache string
	// Sources are the web pages a search model used (Perplexity's
	// citations and search_results, or url_citation annotations), in order,
	// without duplicates. Empty for a normal call.
	Sources []string
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

// Client sends chat completions to one provider. It is safe for
// concurrent use.
type Client struct {
	oc openai.Client
	p  Provider
	s  Settings

	gateMu sync.Mutex
	next   time.Time
}

// New builds a client for provider p. The key and base URL are passed
// explicitly, so the SDK's own OPENAI_API_KEY and OPENAI_BASE_URL variables
// are never used.
func New(p Provider, s Settings) *Client {
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(p.BaseURL, "/") + "/"),
		option.WithAPIKey(p.APIKey),
		option.WithMaxRetries(s.Retries),
	}
	// An empty key sends no Authorization header (compat with a key stored
	// in the gateway, or Workers AI).
	for k, v := range p.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}
	if s.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(s.Timeout))
	}
	if s.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(s.HTTPClient))
	}
	if s.Middleware != nil {
		opts = append(opts, option.WithMiddleware(s.Middleware))
	}
	return &Client{oc: openai.NewClient(opts...), p: p, s: s}
}

// Model is the only model this client uses.
func (c *Client) Model() string { return c.p.Model }

// Engine is the engine this client serves.
func (c *Client) Engine() string { return c.p.Engine }

// Provider is the provider's name: avalai or cloudflare.
func (c *Client) Provider() string { return c.p.Name }

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
	return c.observed(ctx, c.p.Model, func() (*Completion, *CallError) {
		return c.complete(ctx, c.p.Model, true, msgs, schemaName, schema)
	})
}

// SearchModel is the model used for ai_fetch "web_search", or "" when the
// provider has none.
func (c *Client) SearchModel() string { return c.p.SearchModel }

// CompleteWeb sends msgs to the provider's search model, which looks the
// product up on the web itself. No response_format is sent: search models
// such as Perplexity's sonar reject json_object. The answer's sources are
// returned in Completion.Sources.
func (c *Client) CompleteWeb(ctx context.Context, msgs []Message) (*Completion, *CallError) {
	return c.observed(ctx, c.p.SearchModel, func() (*Completion, *CallError) {
		return c.complete(ctx, c.p.SearchModel, false, msgs, "", nil)
	})
}

// observed runs one call and reports it to Observe.
func (c *Client) observed(ctx context.Context, model string, call func() (*Completion, *CallError)) (*Completion, *CallError) {
	start := time.Now()
	comp, cerr := call()
	if c.s.Observe != nil {
		ci := infoOf(ctx)
		call := Call{Engine: c.p.Engine, Model: model, Purpose: ci.purpose, Host: ci.host, Outcome: "ok", Duration: time.Since(start)}
		if comp != nil {
			call.InputTokens, call.OutputTokens = comp.InputTokens, comp.OutputTokens
			if comp.Model != "" {
				call.Model = comp.Model
			}
		}
		if cerr != nil {
			call.Outcome = cerr.Code
		}
		c.s.Observe(call)
	}
	return comp, cerr
}

func (c *Client) complete(ctx context.Context, model string, jsonFormat bool, msgs []Message, schemaName string, schema map[string]any) (*Completion, *CallError) {
	if err := c.wait(ctx); err != nil {
		return nil, &CallError{Code: diagnose.AIUnavailable, Message: "the request's time limit was reached while waiting for the AI rate gate"}
	}
	params := openai.ChatCompletionNewParams{
		Model:       model,
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
	// Without jsonFormat no response_format is sent: the prompt asks for
	// JSON and ParseProduct takes the outermost object.
	switch {
	case !jsonFormat:
	case c.s.ResponseFormat == FormatJSONSchema && schema != nil:
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   schemaName,
				Strict: openai.Bool(true),
				Schema: schema,
			}},
		}
	default:
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	if c.p.ReasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(c.p.ReasoningEffort)
	}

	var httpResp *http.Response
	reqOpts := []option.RequestOption{option.WithResponseInto(&httpResp)}
	if c.p.Metadata {
		reqOpts = append(reqOpts, option.WithHeader("cf-aig-metadata", metadata(ctx)))
	}
	res, err := c.oc.Chat.Completions.New(ctx, params, reqOpts...)
	reqID := c.requestID(httpResp)
	if err != nil {
		return nil, c.mapError(ctx, err, reqID, httpResp)
	}
	out := &Completion{RequestID: reqID, InputTokens: res.Usage.PromptTokens, OutputTokens: res.Usage.CompletionTokens}
	if httpResp != nil {
		out.Status = httpResp.StatusCode
		out.GatewayCache = httpResp.Header.Get("cf-aig-cache-status")
		if c.p.Name == ProviderCloudflare && strings.HasPrefix(c.p.Model, "dynamic/") {
			out.Model = httpResp.Header.Get("cf-aig-model")
		}
	}
	if len(res.Choices) == 0 {
		return out, &CallError{Code: diagnose.AIBadOutput, Status: out.Status, RequestID: reqID, Message: "the provider answered with no choices"}
	}
	ch := res.Choices[0]
	out.Content, out.FinishReason, out.Refusal = ch.Message.Content, ch.FinishReason, ch.Message.Refusal
	out.Sources = sources(res)
	return out, nil
}

// sources reads the pages a search model used: Perplexity's top-level
// "citations" (URLs) and "search_results" ([{url, ...}]), then OpenAI's
// url_citation annotations.
func sources(res *openai.ChatCompletion) []string {
	var out []string
	seen := map[string]bool{}
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	if f, ok := res.JSON.ExtraFields["citations"]; ok {
		var urls []string
		if json.Unmarshal([]byte(f.Raw()), &urls) == nil {
			for _, u := range urls {
				add(u)
			}
		}
	}
	if f, ok := res.JSON.ExtraFields["search_results"]; ok {
		var results []struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(f.Raw()), &results) == nil {
			for _, r := range results {
				add(r.URL)
			}
		}
	}
	for _, ch := range res.Choices {
		for _, a := range ch.Message.Annotations {
			add(a.URLCitation.URL)
		}
	}
	return out
}

// metadata is the cf-aig-metadata value: a flat JSON object (at most 5
// entries) that tags the gateway's log entry with the call's host, purpose
// and prompt version.
func metadata(ctx context.Context) string {
	ci := infoOf(ctx)
	pv := PromptVersion
	if ci.purpose == PurposeLearn {
		pv = LearnPromptVersion
	}
	m := map[string]string{"purpose": ci.purpose, "prompt_version": pv}
	if ci.host != "" {
		m["host"] = ci.host
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// requestID reads the provider's request id for support tickets.
func (c *Client) requestID(r *http.Response) string {
	if r == nil {
		return ""
	}
	for _, h := range c.p.RequestIDHeaders {
		if v := r.Header.Get(h); v != "" {
			return v
		}
	}
	return ""
}

// mapError turns an SDK error into a failure code. The provider's profile
// decides first; the shared mapping follows AvalAI's codes
// (docs.avalai.ir, error handling), which other OpenAI-compatible providers
// use for the common cases too.
//
// resp is the provider's answer when one came. An error body the SDK cannot
// decode (Cloudflare sometimes sends {"error": "text"}) still has a status
// and is mapped from it.
func (c *Client) mapError(ctx context.Context, err error, reqID string, resp *http.Response) *CallError {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) && resp != nil && resp.StatusCode >= 400 {
		apiErr = &openai.Error{StatusCode: resp.StatusCode, Response: resp}
	}
	if apiErr != nil {
		pe := readProviderError(apiErr.StatusCode, apiErr.Code, strings.TrimSpace(apiErr.Message), apiErr.Response)
		ce := &CallError{Status: apiErr.StatusCode, ProviderCode: pe.Code, RequestID: reqID}
		if ce.RequestID == "" {
			ce.RequestID = c.requestID(apiErr.Response)
		}
		msg := c.redact(pe.Message)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if c.p.MapError != nil {
			ce.Code = c.p.MapError(pe)
		}
		if ce.Code == "" {
			ce.Code = sharedError(pe)
		}
		ce.Message = fmt.Sprintf("the AI provider (%s) answered HTTP %d (%s): %s", c.p.Name, pe.Status, orDash(pe.Code), orDash(msg))
		return ce
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return &CallError{Code: diagnose.AIUnavailable, RequestID: reqID, Message: "the AI provider (" + c.p.Name + ") did not answer in time"}
	}
	return &CallError{Code: diagnose.AIUnavailable, RequestID: reqID, Message: "the AI provider (" + c.p.Name + ") could not be reached: " + c.redact(err.Error())}
}

// sharedError is the OpenAI-style mapping every provider falls back to.
func sharedError(e ProviderError) string {
	code := strings.ToLower(e.Code)
	switch {
	case code == "model_not_found":
		return diagnose.AIModelUnavailable
	case e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden:
		return diagnose.AIAuthFailed
	case e.Status == http.StatusTooManyRequests && (code == "insufficient_quota" || code == "quota_exceeded"):
		return diagnose.AIQuotaExceeded
	case e.Status == http.StatusTooManyRequests:
		return diagnose.AIRateLimited
	case code == "context_window_exceeded" || code == "context_length_exceeded" ||
		e.Status == http.StatusRequestEntityTooLarge:
		return diagnose.AIInputTooLarge
	default:
		return diagnose.AIUnavailable
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// redact removes every secret of the provider from s, in case an error
// echoes a header.
func (c *Client) redact(s string) string {
	for _, key := range c.p.secrets() {
		if key != "" {
			s = strings.ReplaceAll(s, key, "[redacted]")
		}
	}
	return s
}
