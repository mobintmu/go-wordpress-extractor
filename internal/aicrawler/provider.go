package aicrawler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go-wordpress/internal/diagnose"
)

// Engine names, as in siteconfig. They are repeated here so this package
// does not import siteconfig.
const (
	EngineAvalAI     = "ai-crawler-avalai"
	EngineCloudflare = "ai-crawler-cloudflare"
)

// Provider names, for logs, metrics and evidence.
const (
	ProviderAvalAI     = "avalai"
	ProviderCloudflare = "cloudflare"
)

// Cloudflare AI Gateway endpoints (APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT).
const (
	CloudflareREST   = "rest"
	CloudflareCompat = "compat"
)

// Provider is everything that differs between two OpenAI-compatible
// providers: where the call goes, how it is authenticated, which headers
// are added, and how errors are mapped. Prompts, grounding, the cache and
// learning are shared.
type Provider struct {
	// Engine is the fetch engine this provider serves: ai-crawler-avalai or
	// ai-crawler-cloudflare.
	Engine string
	// Name is "avalai" or "cloudflare".
	Name    string
	BaseURL string
	Model   string
	// APIKey goes into Authorization: Bearer. Secret.
	APIKey string
	// Headers are sent on every call (cf-aig-gateway-id, ...). Values may be
	// secret (cf-aig-authorization); they are redacted from errors.
	Headers map[string]string
	// SecretHeaders names the headers in Headers whose values are secrets.
	SecretHeaders []string
	// RequestIDHeaders are read, in order, for the provider's request id.
	RequestIDHeaders []string
	// ReasoningEffort is sent as reasoning_effort when not empty.
	ReasoningEffort string
	// SearchModel is the model that searches the web itself, for ai_fetch
	// "web_search" (AvalAI: Perplexity's sonar). "" means the provider
	// offers none here.
	SearchModel string
	// Metadata sends cf-aig-metadata (host, purpose, prompt version) on
	// every call, so the gateway's logs line up with ours.
	Metadata bool
	// MapError maps a failed answer to a diagnose code. "" means the shared
	// OpenAI-style mapping applies.
	MapError func(e ProviderError) string
}

// ProviderError is a failed answer as the profiles see it.
type ProviderError struct {
	Status int
	// Code is the OpenAI-style error.code (a string such as invalid_api_key).
	Code string
	// CFCode is Cloudflare's numeric code (errors[0].code), 0 when absent.
	CFCode  int
	Message string
}

// secrets lists every secret the provider sends, so none ends up in an
// error message.
func (p Provider) secrets() []string {
	out := []string{p.APIKey}
	for _, h := range p.SecretHeaders {
		if v := p.Headers[h]; v != "" {
			out = append(out, strings.TrimPrefix(v, "Bearer "))
		}
	}
	return out
}

// AvalAI is the profile of the ai-crawler-avalai engine. searchModel is
// used for ai_fetch "web_search".
func AvalAI(baseURL, model, apiKey, searchModel string) Provider {
	return Provider{
		SearchModel:      searchModel,
		Engine:           EngineAvalAI,
		Name:             ProviderAvalAI,
		BaseURL:          baseURL,
		Model:            model,
		APIKey:           apiKey,
		RequestIDHeaders: []string{"avalai-request-id", "x-request-id"},
	}
}

// CloudflareSettings configure the ai-crawler-cloudflare profile. They come
// from APP_AI_CRAWLER_CLOUDFLARE_*.
type CloudflareSettings struct {
	AccountID string
	GatewayID string
	// Endpoint is "rest" or "compat".
	Endpoint string
	// BaseURL overrides the URL built from the endpoint. Optional.
	BaseURL         string
	Model           string
	ReasoningEffort string
	// APIToken is the Cloudflare API token (rest) or the gateway token
	// (compat). Secret.
	APIToken string
	// ProviderAPIKey is the provider's own key (compat only). Secret.
	ProviderAPIKey   string
	NoUnifiedBilling bool
	CollectLog       bool
}

// CloudflareBaseURL is the base URL of an endpoint, without the override.
func CloudflareBaseURL(endpoint, accountID, gatewayID string) string {
	if endpoint == CloudflareCompat {
		return "https://gateway.ai.cloudflare.com/v1/" + accountID + "/" + gatewayID + "/compat"
	}
	return "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1"
}

// Cloudflare is the profile of the ai-crawler-cloudflare engine. On rest
// the API token goes in Authorization and the gateway is named by
// cf-aig-gateway-id. On compat the gateway is in the path, the gateway
// token goes in cf-aig-authorization, and Authorization carries the
// provider's key when one is set.
func Cloudflare(s CloudflareSettings) Provider {
	p := Provider{
		Engine:           EngineCloudflare,
		Name:             ProviderCloudflare,
		BaseURL:          s.BaseURL,
		Model:            s.Model,
		ReasoningEffort:  s.ReasoningEffort,
		Headers:          map[string]string{"cf-aig-skip-cache": "true"},
		RequestIDHeaders: []string{"cf-aig-log-id", "cf-aig-request-id"},
		Metadata:         true,
		MapError:         cloudflareError,
	}
	if p.BaseURL == "" {
		p.BaseURL = CloudflareBaseURL(s.Endpoint, s.AccountID, s.GatewayID)
	}
	if s.Endpoint == CloudflareCompat {
		p.APIKey = s.ProviderAPIKey
		if s.APIToken != "" {
			p.Headers["cf-aig-authorization"] = "Bearer " + s.APIToken
			p.SecretHeaders = []string{"cf-aig-authorization"}
		}
	} else {
		p.APIKey = s.APIToken
		p.Headers["cf-aig-gateway-id"] = s.GatewayID
	}
	if s.NoUnifiedBilling {
		p.Headers["cf-aig-no-wholesale"] = "true"
	}
	if !s.CollectLog {
		p.Headers["cf-aig-collect-log"] = "false"
	}
	return p
}

// cloudflareError maps Cloudflare AI Gateway's documented errors
// (developers.cloudflare.com/ai-gateway, troubleshooting and
// unified-billing pages). "" leaves the shared mapping in charge.
func cloudflareError(e ProviderError) string {
	msg := strings.ToLower(e.Message)
	switch e.CFCode {
	case 2009, 2044, 2049, 2042, 2036, 10000:
		return diagnose.AIAuthFailed
	case 2052, 2021:
		return diagnose.AIQuotaExceeded
	case 2018:
		return diagnose.AIRateLimited
	case 7003:
		return diagnose.AIInputTooLarge
	case 7000:
		return diagnose.AIModelUnavailable
	case 2002, 2005, 7005:
		return diagnose.AIUnavailable
	}
	switch {
	case e.Status == http.StatusPaymentRequired:
		return diagnose.AIQuotaExceeded
	case e.Status == http.StatusUnavailableForLegalReasons,
		e.Status == http.StatusNotFound,
		strings.Contains(msg, "model not found"), strings.Contains(msg, "model access denied"),
		strings.Contains(msg, "model unavailable"), strings.Contains(msg, "dynamic route not found"),
		strings.Contains(msg, "no route for that uri"):
		return diagnose.AIModelUnavailable
	case strings.Contains(msg, "input is too large"):
		return diagnose.AIInputTooLarge
	}
	return ""
}

// readProviderError reads the codes of a failed answer. The OpenAI shape
// ({"error": {"code": ...}}) is decoded by the SDK; Cloudflare's own shape
// ({"errors": [{"code": 2009, "message": ...}]}) is read from the body,
// which the SDK keeps for this.
func readProviderError(status int, code, message string, resp *http.Response) ProviderError {
	pe := ProviderError{Status: status, Code: code, Message: message}
	if n, err := strconv.Atoi(code); err == nil {
		pe.CFCode = n
	}
	if resp == nil || resp.Body == nil {
		return pe
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return pe
	}
	var body struct {
		Errors []struct {
			Code    json.Number `json:"code"`
			Message string      `json:"message"`
		} `json:"errors"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(b, &body) != nil {
		return pe
	}
	if len(body.Errors) > 0 {
		if n, err := body.Errors[0].Code.Int64(); err == nil {
			pe.CFCode = int(n)
			if pe.Code == "" {
				pe.Code = body.Errors[0].Code.String()
			}
		}
		if pe.Message == "" {
			pe.Message = body.Errors[0].Message
		}
	}
	// Some gateway errors put a bare string in "error".
	if pe.Message == "" && len(body.Error) > 0 {
		var s string
		if json.Unmarshal(body.Error, &s) == nil {
			pe.Message = s
		}
	}
	return pe
}
