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
} //@name Evidence

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
