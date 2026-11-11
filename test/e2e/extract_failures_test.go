package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestExtractFailures(t *testing.T) {
	e := Start(t, Options{})
	s := e.Shop

	type tc struct {
		name, url, code, stage, owner string
		retryable                     bool
		check                         func(t *testing.T, r *Result)
	}
	cases := []tc{
		// config
		{"unknown host", "http://unknown.test/product/x/", "SITE_NOT_CONFIGURED", "config", "site_config", false, func(t *testing.T, r *Result) {
			if r.Has("site_id") {
				t.Errorf("site_id must be absent when no config matched")
			}
			if r.Str("evidence.failed_url") != "http://unknown.test/product/x/" {
				t.Errorf("evidence.failed_url = %q", r.Str("evidence.failed_url"))
			}
		}},
		{"disabled site", s.URL(hostDisabled, "/product/ok/"), "SITE_DISABLED", "config", "site_config", false, nil},
		{"invalid config", s.URL(hostBroken, "/product/ok/"), "CONFIG_INVALID", "config", "site_config", false, func(t *testing.T, r *Result) {
			if !strings.Contains(r.Str("error.root_cause"), "version is required") || !strings.Contains(r.Str("error.root_cause"), "div[[[") {
				t.Errorf("root cause should list the problems: %s", r.Str("error.root_cause"))
			}
			if r.Str("site_id") != "broken" {
				t.Errorf("site_id = %q", r.Str("site_id"))
			}
		}},
		{"category page", s.URL(hostShop, "/category/nuts/"), "NOT_PRODUCT_URL", "config", "client", false, func(t *testing.T, r *Result) {
			if s.Hits("/category/nuts/") != 0 {
				t.Errorf("a non-product URL must not be fetched")
			}
		}},
		{"redirect to a category", s.URL(hostShop, "/product/redirect-category/"), "NOT_PRODUCT_URL", "config", "client", false, func(t *testing.T, r *Result) {
			if !strings.Contains(r.Str("error.root_cause"), "does not mean the product is gone") {
				t.Errorf("root cause: %s", r.Str("error.root_cause"))
			}
			if r.Str("evidence.redirects.0") == "" {
				t.Errorf("redirect chain missing: %s", r.Raw)
			}
		}},

		// fetch
		{"private address after DNS", s.URL(hostPrivate, "/product/ok/"), "URL_NOT_ALLOWED", "fetch", "client", false, func(t *testing.T, r *Result) {
			if !strings.Contains(r.Str("error.root_cause"), "10.0.0.5") {
				t.Errorf("root cause should name the address: %s", r.Str("error.root_cause"))
			}
		}},
		{"blocked port", "http://" + hostShop + ":6379/product/ok/", "URL_NOT_ALLOWED", "fetch", "client", false, nil},
		{"dns failure", s.URL(hostNoDNS, "/product/ok/"), "DNS_FAILED", "fetch", "source_site", true, func(t *testing.T, r *Result) {
			if r.Int("evidence.attempts") != 3 {
				t.Errorf("attempts = %d, want 3 (2 retries)", r.Int("evidence.attempts"))
			}
		}},
		{"connection refused", "http://" + hostClosed + ":" + s.ClosedPort + "/product/ok/", "CONNECTION_FAILED", "fetch", "source_site", true, func(t *testing.T, r *Result) {
			if r.Int("evidence.attempts") != 3 {
				t.Errorf("attempts = %d, want 3", r.Int("evidence.attempts"))
			}
		}},
		{"bad certificate", "https://" + hostTLS + ":" + s.TLSPort + "/product/ok/", "TLS_FAILED", "fetch", "source_site", false, nil},
		{"timeout", s.URL(hostSlow, "/product/slow/"), "FETCH_TIMEOUT", "fetch", "source_site", true, func(t *testing.T, r *Result) {
			if r.Elapsed > 2900*time.Millisecond {
				t.Errorf("took %s; the 1s site timeout was not applied", r.Elapsed)
			}
		}},
		{"http 404", s.URL(hostShop, "/product/gone/"), "HTTP_NOT_FOUND", "fetch", "client", false, func(t *testing.T, r *Result) {
			if r.Int("evidence.http_status") != 404 || r.Int("evidence.attempts") != 1 {
				t.Errorf("evidence: %s", r.Raw)
			}
		}},
		{"http 410", s.URL(hostShop, "/product/gone410/"), "HTTP_NOT_FOUND", "fetch", "client", false, nil},
		{"http 403", s.URL(hostShop, "/product/forbidden/"), "ACCESS_DENIED", "fetch", "source_site", false, func(t *testing.T, r *Result) {
			if r.Int("evidence.attempts") != 1 || r.Str("evidence.page_title") != "403 Forbidden" {
				t.Errorf("evidence: %s", r.Raw)
			}
		}},
		{"http 403 cloudflare challenge", s.URL(hostShop, "/product/cf/"), "BOT_PROTECTION", "page_check", "source_site", false, func(t *testing.T, r *Result) {
			if r.Str("error.confidence") != "high" || r.Int("evidence.attempts") != 1 {
				t.Errorf("got %s", r.Raw)
			}
			if r.Str("evidence.response_headers.cf-mitigated") != "challenge" || r.Str("evidence.response_headers.server") != "cloudflare" {
				t.Errorf("allow-listed headers missing: %s", r.Raw)
			}
			if r.Has("evidence.response_headers.set-cookie") || strings.Contains(string(r.Raw), "__cf_bm") {
				t.Errorf("Set-Cookie must never be returned")
			}
			if r.Str("evidence.html_sample.reason") != "Start of the response body" {
				t.Errorf("sample reason = %q", r.Str("evidence.html_sample.reason"))
			}
		}},
		{"http 200 captcha page", s.URL(hostShop, "/product/captcha/"), "BOT_PROTECTION", "page_check", "source_site", false, func(t *testing.T, r *Result) {
			if r.Str("error.confidence") != "medium" {
				t.Errorf("markup detection is a heuristic: %s", r.Str("error.confidence"))
			}
		}},
		{"http 429 after retries", s.URL(hostShop, "/product/rate/"), "RATE_LIMITED", "fetch", "source_site", true, func(t *testing.T, r *Result) {
			if r.Int("evidence.attempts") != 3 || s.Hits("/product/rate/") != 3 {
				t.Errorf("attempts = %d, hits = %d, want 3", r.Int("evidence.attempts"), s.Hits("/product/rate/"))
			}
		}},
		{"Retry-After beyond the deadline stops retries", s.URL(hostShop, "/product/rate-long/"), "RATE_LIMITED", "fetch", "source_site", true, func(t *testing.T, r *Result) {
			if r.Int("evidence.attempts") != 1 || r.Elapsed > 5*time.Second {
				t.Errorf("attempts = %d after %s", r.Int("evidence.attempts"), r.Elapsed)
			}
			if r.Str("evidence.response_headers.retry-after") != "3600" {
				t.Errorf("retry-after header missing from evidence")
			}
		}},
		{"http 500 after retries", s.URL(hostShop, "/product/error/"), "SOURCE_SERVER_ERROR", "fetch", "source_site", true, func(t *testing.T, r *Result) {
			if r.Int("evidence.attempts") != 3 || r.Int("evidence.http_status") != 500 {
				t.Errorf("evidence: %s", r.Raw)
			}
		}},
		{"unexpected status", s.URL(hostShop, "/product/teapot/"), "HTTP_ERROR", "fetch", "source_site", false, nil},
		{"robots.txt disallows", s.URL(hostShop, "/product/robots-blocked/"), "ROBOTS_DISALLOWED", "fetch", "site_config", false, func(t *testing.T, r *Result) {
			if s.Hits("/product/robots-blocked/") != 0 {
				t.Errorf("a disallowed page must not be fetched")
			}
		}},
		{"redirect off the site", s.URL(hostShop, "/product/redirect-out/"), "REDIRECT_NOT_ALLOWED", "fetch", "site_config", false, func(t *testing.T, r *Result) {
			if !strings.Contains(r.Str("evidence.redirects.0"), hostOther) {
				t.Errorf("redirect chain should show the target: %s", r.Raw)
			}
		}},
		{"redirect loop", s.URL(hostShop, "/product/loop/"), "TOO_MANY_REDIRECTS", "fetch", "source_site", false, func(t *testing.T, r *Result) {
			if n := len(r.At("evidence.redirects").([]any)); n != 10 {
				t.Errorf("redirects = %d, want 10", n)
			}
		}},
		{"not html", s.URL(hostShop, "/product/json/"), "UNSUPPORTED_CONTENT", "fetch", "source_site", false, func(t *testing.T, r *Result) {
			if r.Str("evidence.content_type") != "application/json" {
				t.Errorf("content_type = %q", r.Str("evidence.content_type"))
			}
		}},
		{"body over the limit, declared", s.URL(hostShop, "/product/huge/"), "RESPONSE_TOO_LARGE", "fetch", "service", false, func(t *testing.T, r *Result) {
			if r.Int("evidence.body_bytes") != 300*1024 {
				t.Errorf("body_bytes = %d", r.Int("evidence.body_bytes"))
			}
		}},
		{"body over the limit, streamed", s.URL(hostShop, "/product/huge-chunked/"), "RESPONSE_TOO_LARGE", "fetch", "service", false, nil},

		// page_check
		{"empty body", s.URL(hostShop, "/product/empty/"), "EMPTY_PAGE", "page_check", "source_site", true, nil},
		{"almost no text", s.URL(hostShop, "/product/blank/"), "EMPTY_PAGE", "page_check", "source_site", true, nil},
		{"javascript shell", s.URL(hostShop, "/product/spa/"), "JS_RENDERED_PAGE", "page_check", "site_config", false, func(t *testing.T, r *Result) {
			if r.Str("error.confidence") != "medium" || !strings.Contains(r.Str("error.root_cause"), "#root") {
				t.Errorf("got %s", r.Raw)
			}
		}},

		// extract
		{"layout changed", s.URL(hostShop, "/product/changed/"), "SELECTOR_NOT_FOUND", "extract", "site_config", false, func(t *testing.T, r *Result) {
			want := []struct {
				sel     string
				matches int64
			}{
				{"div.main-page-wrapper", 1},
				{"div.main-page-wrapper div.site-content", 1},
				{"div.main-page-wrapper div.site-content div.product.type-product", 0},
			}
			for i, w := range want {
				p := "evidence.selector_trace." + string(rune('0'+i))
				if r.Str(p+".selector") != w.sel || r.Int(p+".matches") != w.matches {
					t.Errorf("trace[%d] = %v, want %v", i, r.At(p), w)
				}
			}
			if r.Str("evidence.html_sample.reason") != "Deepest element matched by the failing selector path" ||
				!strings.HasPrefix(r.Str("evidence.html_sample.content"), `<div class="site-content">`) {
				t.Errorf("sample: %v", r.At("evidence.html_sample"))
			}
			if r.Str("evidence.page_title") != "Example product - Shop" || len(r.Str("evidence.body_sha256")) != 64 {
				t.Errorf("evidence: %s", r.Raw)
			}
			if r.At("partial.product") != nil {
				t.Errorf("partial.product must be null")
			}
		}},

		// normalize and validate
		{"price text without a number", s.URL(hostShop, "/product/bad-price/"), "PRICE_PARSE_FAILED", "normalize", "site_config", false, func(t *testing.T, r *Result) {
			if r.Str("partial.product.title") != "بادام درختی" {
				t.Errorf("partial product missing: %s", r.Raw)
			}
			if r.Str("evidence.field_samples.price_current") != "به زودی" {
				t.Errorf("field_samples: %v", r.At("evidence.field_samples"))
			}
		}},
		{"required title missing", s.URL(hostShop, "/product/missing-title/"), "REQUIRED_FIELD_MISSING", "validate", "site_config", false, func(t *testing.T, r *Result) {
			if r.Int("partial.product.price.current") != 400000 {
				t.Errorf("partial product should keep what was found: %s", r.Raw)
			}
			if r.Str("evidence.selector_trace.1.selector") != "h1.product_title" || r.Int("evidence.selector_trace.1.matches") != 0 {
				t.Errorf("trace: %v", r.At("evidence.selector_trace"))
			}
			if r.Str("evidence.html_sample.reason") != "Product root element" {
				t.Errorf("sample: %v", r.At("evidence.html_sample"))
			}
		}},
		{"required image missing", s.URL(hostStrict, "/product/no-image/"), "REQUIRED_FIELD_MISSING", "validate", "site_config", false, func(t *testing.T, r *Result) {
			if !strings.Contains(r.Str("error.root_cause"), "'image_url'") {
				t.Errorf("root cause: %s", r.Str("error.root_cause"))
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := e.Extract(c.url)
			r.Failed(t, c.code, c.stage, c.owner, c.retryable)
			if c.check != nil {
				c.check(t, r)
			}
		})
	}
}

func TestExtractRecoversAfterRetry(t *testing.T) {
	e := Start(t, Options{})
	for _, path := range []string{"/product/flaky/", "/product/rate-once/"} {
		t.Run(path, func(t *testing.T) {
			e.Extract(e.Shop.URL(hostShop, path)).Success(t)
			if got := e.Shop.Hits(path); got != 2 {
				t.Errorf("hits = %d, want 2", got)
			}
		})
	}
}

func TestDeadlineExceeded(t *testing.T) {
	e := Start(t, Options{Env: map[string]string{"APP_MAX_REQUEST_DURATION": "1500ms", "APP_SITE_WAIT_TIMEOUT": "1s"}})
	r := e.Extract(e.Shop.URL(hostHang, "/product/hang/"))
	r.Failed(t, "DEADLINE_EXCEEDED", "fetch", "service", true)
	if r.Elapsed > 3*time.Second {
		t.Errorf("took %s; the request deadline was not enforced", r.Elapsed)
	}
	select {
	case <-e.Shop.Cancelled:
	case <-time.After(2 * time.Second):
		t.Errorf("the outgoing fetch was not cancelled at the deadline")
	}
}
