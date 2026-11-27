package diagnose

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

// MaxSampleBytes is the largest HTML sample returned in a failure.
const MaxSampleBytes = 16 * 1024

// Evidence describes what the service saw. Every field is optional.
type Evidence struct {
	FailedURL   string   `json:"failed_url,omitempty" format:"uri" example:"https://badomjip.com/product/example/"`
	FinalURL    string   `json:"final_url,omitempty" format:"uri" example:"https://badomjip.com/product/example/"`
	Redirects   []string `json:"redirects"`
	HTTPStatus  int      `json:"http_status,omitempty" example:"200"`
	Attempts    int      `json:"attempts,omitempty" example:"1"`
	ContentType string   `json:"content_type,omitempty" example:"text/html; charset=UTF-8"`
	BodyBytes   int64    `json:"body_bytes,omitempty" example:"184233"`
	BodySHA256  string   `json:"body_sha256,omitempty" example:"9f2c0d7ab1"`
	PageTitle   string   `json:"page_title,omitempty" example:"Example product - Badomjip"`
	// Allow-listed headers only: server, content-type, retry-after, cf-ray, cf-mitigated, location. Never Set-Cookie.
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	// Match count for each step of the failing selector path.
	SelectorTrace []TraceStep `json:"selector_trace,omitempty"`
	HTMLSample    *HTMLSample `json:"html_sample,omitempty"`
	// Raw text found for each field.
	FieldSamples map[string]string `json:"field_samples,omitempty"`
	// What a search engine searched for. Set only for the search engines (duckduckgo, brave, mojeek, yahoo, yandex).
	Search *SearchEvidence `json:"search,omitempty"`
	// What the AI crawler sent to and got back from the LLM. Set only for the AI engines (AvalAI, Cloudflare or GapGPT). Never holds the API key, the token or the prompt.
	AI *AIEvidence `json:"ai,omitempty"`
} //@name Evidence

// AIEvidence describes one call to the LLM by an AI engine.
type AIEvidence struct {
	Model string `json:"model" binding:"required" example:"deepseek-v4.1-flash"`
	// Who answered: avalai, cloudflare, or gapgpt.
	Provider string `json:"provider,omitempty" enums:"avalai,cloudflare,gapgpt" example:"avalai"`
	// The provider's request id, for support tickets: avalai-request-id (AvalAI), or cf-aig-log-id, the id to search for in the Cloudflare dashboard.
	ProviderRequestID string `json:"provider_request_id,omitempty" example:"req_0123456789"`
	// HTTP status of the provider's last answer; 0 when no answer came.
	HTTPStatus int `json:"http_status,omitempty" example:"429"`
	// The provider's error code, such as rate_limit_exceeded or invalid_api_key, or Cloudflare's numeric code (2009).
	ProviderCode string `json:"provider_code,omitempty" example:"rate_limit_exceeded"`
	// Cloudflare only: cf-aig-cache-status (HIT or MISS). The gateway cache is skipped, so it should be MISS.
	GatewayCache string `json:"gateway_cache,omitempty" example:"MISS"`
	FinishReason string `json:"finish_reason,omitempty" example:"length"`
	InputTokens  int64  `json:"input_tokens,omitempty" example:"8421"`
	OutputTokens int64  `json:"output_tokens,omitempty" example:"512"`
	// Characters of cleaned page sent to the model.
	InputChars int `json:"input_chars,omitempty" example:"24000"`
	// Start of the model's answer, at most 500 characters. Untrusted text.
	Output string `json:"output,omitempty" example:"{is_product: true, name: Oak Chair ..."`
} //@name AIEvidence

// SearchEvidence describes the queries a search engine sent and what came back.
type SearchEvidence struct {
	Engine string `json:"engine" binding:"required" example:"duckduckgo"`
	// True when the answer came from the instance's cache and no query was sent for this request.
	Cached  bool          `json:"cached" binding:"required" example:"false"`
	Queries []SearchQuery `json:"queries" binding:"required"`
	// Up to 3 result URLs on the same site, for a person to check. Never used as the product.
	Closest []string `json:"closest,omitempty"`
} //@name SearchEvidence

// SearchQuery is one query a search engine sent.
type SearchQuery struct {
	Query string `json:"query" binding:"required" example:"site:hafele.com.de universalverbinder haefele ixconnect uc 16 64"`
	// Number of web results that came back.
	Results int  `json:"results" example:"10"`
	Matched bool `json:"matched" example:"false"`
} //@name SearchQuery

// TraceStep is the match count for one prefix of a selector path.
type TraceStep struct {
	Selector string `json:"selector" binding:"required" example:"div.main-page-wrapper"`
	Matches  int    `json:"matches" binding:"required" example:"1"`
} //@name SelectorTraceStep

// HTMLSample is a bounded piece of untrusted third-party HTML.
type HTMLSample struct {
	Reason string `json:"reason" binding:"required" example:"Deepest element matched by the failing selector path"`
	// Untrusted third-party HTML. Escape before displaying.
	Content        string `json:"content" binding:"required" example:"<div class=\"site-content\">...</div>"`
	Truncated      bool   `json:"truncated" binding:"required" example:"true"`
	OriginalLength int    `json:"original_length,omitempty" example:"48211"`
} //@name HtmlSample

// Sample reasons.
const (
	ReasonBodyStart    = "Start of the response body"
	ReasonDeepestMatch = "Deepest element matched by the failing selector path"
	ReasonProductRoot  = "Product root element"
)

// NewSample truncates content to MaxSampleBytes on a UTF-8 boundary.
func NewSample(reason, content string) *HTMLSample {
	s := &HTMLSample{Reason: reason, OriginalLength: len(content)}
	if len(content) <= MaxSampleBytes {
		s.Content = content
		return s
	}
	cut := MaxSampleBytes
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	s.Content = content[:cut]
	s.Truncated = true
	return s
}

// allowedHeaders are the only response headers returned as evidence.
// Set-Cookie is never returned.
var allowedHeaders = []string{"server", "content-type", "retry-after", "cf-ray", "cf-mitigated", "location"}

// FilterHeaders returns the allow-listed response headers.
func FilterHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	out := map[string]string{}
	for _, name := range allowedHeaders {
		if v := h.Get(name); v != "" {
			out[name] = truncate(v, 256)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// TruncateText shortens a field sample to n bytes.
func TruncateText(s string, n int) string { return truncate(strings.TrimSpace(s), n) }
