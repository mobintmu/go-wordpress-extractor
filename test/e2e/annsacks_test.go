package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// Ann Sacks follows the default AI engine (fetch.engine "default", resolved
// from APP_DEFAULT_ENGINE, which this test sets to ai-crawler-avalai). AvalAI
// is never called here: a fake LLM replays the answer recorded on 2026-11-19
// (testdata/sites/annsacks/savoy-box-liner-trim.ai-crawler-avalai.json).
const (
	hostAnnSacks     = "annsacks.test"
	hostAnnSacksHTTP = "annsacks-http.test"
	annSacksPath     = "/p/trim/savoy-0-5-x-8-box-liner-trim-as11682-trim-boxliner"
	annSacksFixture  = "savoy-box-liner-trim"
)

// annSacksSite loads the real config for host. engine, when set, replaces
// fetch.engine (the http copy checks the fallback selectors).
func annSacksSite(t *testing.T, host, engine string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "annsacks.kohler.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, host)
	f, _ := cfg["fetch"].(map[string]any)
	if engine != "" {
		// A second config may not claim the real domains or site_id.
		f["engine"] = engine
		cfg["domains"] = []any{host}
		cfg["site_id"] = "annsacks-" + engine
	}
	f["delay_ms"], f["timeout_seconds"], f["retries"], f["respect_robots_txt"] = 0, 5, 2, true
	cfg["fetch"] = f
	return cfg
}

// annSacksRecorded returns the live AvalAI answer recorded for the sample.
func annSacksRecorded(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "sites", "annsacks", annSacksFixture+".ai-crawler-avalai.json"))
	must(t, err)
	var rec map[string]any
	must(t, json.Unmarshal(raw, &rec))
	p, _ := rec["product"].(map[string]any)
	if p == nil {
		t.Fatal("recorded answer has no product")
	}
	return p
}

// annSacksAnswer turns the recorded product into the model's JSON answer.
func annSacksAnswer(p map[string]any) string {
	price := p["price"].(map[string]any)
	b, _ := json.Marshal(map[string]any{
		"is_product": true, "name": p["title"], "url": p["url"], "image": p["image_url"], "description": p["description"],
		"price":    map[string]any{"current_text": price["raw"], "regular_text": nil, "currency": "USD", "state": price["state"]},
		"specs":    []any{},
		"features": []any{},
	})
	return string(b)
}

func TestAnnSacks(t *testing.T) {
	page, want := siteCase{SiteID: "annsacks", Fixture: annSacksFixture}.fixture(t)
	rec := annSacksRecorded(t)
	answer := annSacksAnswer(rec)

	llm := newFakeLLM(t)
	llm.respond = func(w http.ResponseWriter, c llmCall, n int) {
		switch {
		case strings.HasSuffix(c.PageURL, "-ratelimited"):
			chatError(w, 429, "rate_limit_exceeded", "Rate limit reached", map[string]string{"Retry-After": "0"})
		case strings.HasSuffix(c.PageURL, "-down"):
			chatError(w, 503, "model_overloaded", "Model overloaded", nil)
		default:
			chatOK(w, answer, "stop")
		}
	}
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{
			hostAnnSacks:     annSacksSite(t, hostAnnSacks, ""),
			hostAnnSacksHTTP: annSacksSite(t, hostAnnSacksHTTP, "http"),
		},
		Env:   aiEnv(llm, map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-avalai"}),
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(host, path string) string {
		return s.URL(host, (&url.URL{Path: path}).EscapedPath())
	}
	s.Handle(annSacksPath, page)
	calls := func() int { return llm.count(false) }

	t.Run("config selects the default engine", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "annsacks.kohler.com.json"))
		must(t, err)
		var cfg struct {
			Fetch struct {
				Engine string `json:"engine"`
				UA     string `json:"user_agent"`
			} `json:"fetch"`
		}
		must(t, json.Unmarshal(raw, &cfg))
		if cfg.Fetch.Engine != "default" {
			t.Errorf("fetch.engine = %q, want default", cfg.Fetch.Engine)
		}
		if !strings.Contains(cfg.Fetch.UA, "Chrome/") {
			t.Errorf("fetch.user_agent = %q, want a Chrome User-Agent (the default one is tarpitted)", cfg.Fetch.UA)
		}
	})

	t.Run("sample product with the mocked AI engine", func(t *testing.T) {
		before := calls()
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(hostAnnSacks, annSacksPath), "request_id": "annsacks-1"})
		r.Success(t)
		if r.Str("site_id") != "annsacks" || r.Str("source.engine") != "ai-crawler-avalai" {
			t.Errorf("site_id/engine = %q/%q", r.Str("site_id"), r.Str("source.engine"))
		}
		for _, f := range []string{"title", "url", "image_url", "description"} {
			if got, exp := r.Str("product."+f), rec[f].(string); got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
		}
		// $21.95, as on the page (JSON-LD offer)
		if r.Int("product.price.current") != 2195 || r.Str("product.price.currency") != "USD" || r.Str("product.price.state") != "available" {
			t.Errorf("price = %d %s %s, want 2195 USD available", r.Int("product.price.current"), r.Str("product.price.currency"), r.Str("product.price.state"))
		}
		if !hasWarning(r, "AI_EXTRACTED") {
			t.Error("want an AI_EXTRACTED warning")
		}
		if n := calls() - before; n != 1 {
			t.Errorf("LLM calls = %d, want 1", n)
		}
	})

	t.Run("saved page matches the fixture with the http fallback selectors", func(t *testing.T) {
		before := calls()
		r := e.Extract(productURL(hostAnnSacksHTTP, annSacksPath))
		r.Success(t)
		for _, f := range []string{"title", "description"} {
			if got, exp := r.Str("product."+f), want[f].(string); got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
		}
		if r.Str("product.price.state") != "unknown" || !hasWarning(r, "PRICE_MISSING") {
			t.Errorf("state = %s, want unknown with PRICE_MISSING", r.Str("product.price.state"))
		}
		if calls() != before {
			t.Error("the http engine must not call the LLM")
		}
	})

	t.Run("canonical URL on annsacks.kohler.com", func(t *testing.T) {
		r := e.Extract(productURL(hostAnnSacks, annSacksPath))
		r.Success(t)
		if got := r.Str("product.url"); !strings.HasPrefix(got, "https://annsacks.kohler.com/p/trim/") {
			t.Errorf("product.url = %s", got)
		}
	})

	t.Run("layout change: fallback selectors report SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/p/trim/savoy-changed"
		s.Handle(path, applyEdits(t, page, []edit{{Old: `type="application/ld+json"`, New: `type="application/json"`, All: true}}))
		r := e.Extract(productURL(hostAnnSacksHTTP, path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
	})

	t.Run("non-product page is not fetched and the LLM is not called", func(t *testing.T) {
		before := calls()
		path := "/tile/all-tile-products"
		r := e.Extract(productURL(hostAnnSacks, path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 || calls() != before {
			t.Errorf("hits = %d, LLM calls = %d; want none", s.Hits(path), calls()-before)
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND without an LLM call", func(t *testing.T) {
		before := calls()
		r := e.Extract(productURL(hostAnnSacks, "/p/trim/gone"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
		if calls() != before {
			t.Error("the LLM must not be called for a missing page")
		}
	})

	t.Run("provider errors", func(t *testing.T) {
		for _, tc := range []struct {
			suffix, code string
			retryable    bool
		}{
			{"ratelimited", "AI_RATE_LIMITED", true},
			{"down", "AI_UNAVAILABLE", true},
		} {
			path := "/p/trim/savoy-" + tc.suffix
			s.Handle(path, page)
			r := e.Extract(productURL(hostAnnSacks, path))
			r.Failed(t, tc.code, "extract", "service", tc.retryable)
			if r.Has("product") && r.At("product") != nil {
				t.Errorf("%s: want no product", tc.suffix)
			}
		}
	})
}
