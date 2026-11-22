package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// webSiteCase is a site that is blocked for every page-fetch engine and
// therefore follows the default AI engine (fetch.engine "default", resolved
// from APP_DEFAULT_ENGINE) with ai_fetch "web_search": the service never
// contacts the shop, and the search model answers with the product and its
// sources. The answer is replayed from the recorded run
// testdata/sites/<SiteID>/<Recorded>.json (no fixture page exists).
type webSiteCase struct {
	SiteID, Domain, Recorded string
	// Path is a product path on the real domain (never fetched); CategoryPath
	// is rejected by the match patterns; GonePath is a second product path.
	Path, CategoryPath, GonePath string
	Title                        string
	Price                        int64 // cents, or -1 for null
	State                        string
	Specs                        int
	Features                     int
}

// runWebSite runs the standard web_search subtests.
func runWebSite(t *testing.T, c webSiteCase) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "sites", c.SiteID, c.Recorded+".json"))
	must(t, err)
	var rec struct {
		Product map[string]any `json:"product"`
		Source  struct {
			Sources []any `json:"sources"`
		} `json:"source"`
	}
	must(t, json.Unmarshal(raw, &rec))
	if rec.Product == nil {
		t.Fatal("recorded answer has no product")
	}
	answer := llmAnswer(rec.Product)
	var citations []string
	for _, s := range rec.Source.Sources {
		if m, ok := s.(map[string]any); ok {
			if u, _ := m["url"].(string); u != "" {
				citations = append(citations, u)
			}
		}
	}
	if len(citations) == 0 {
		citations = []string{"https://" + c.Domain + c.Path}
	}

	llm := newFakeLLM(t)
	llm.respond = func(w http.ResponseWriter, call llmCall, n int) {
		if strings.Contains(call.PageURL, "ratelimited") {
			chatError(w, 429, "rate_limit_exceeded", "Rate limit reached", map[string]string{"Retry-After": "0"})
			return
		}
		sonarAnswer(w, answer, citations)
	}
	cfgRaw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", c.Domain+".json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(cfgRaw, &cfg))
	e := Start(t, Options{
		Env:        webEnv(llm, map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-avalai"}),
		ExtraSites: map[string]map[string]any{c.Domain: cfg},
	})
	calls := func() int { return llm.count(false) }
	product := "https://" + c.Domain + c.Path

	t.Run("config selects the default engine with ai_fetch web_search", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", c.Domain+".json"))
		must(t, err)
		var cfg struct {
			Fetch struct {
				Engine  string `json:"engine"`
				AIFetch string `json:"ai_fetch"`
			} `json:"fetch"`
		}
		must(t, json.Unmarshal(raw, &cfg))
		if cfg.Fetch.Engine != "default" || cfg.Fetch.AIFetch != "web_search" {
			t.Errorf("fetch = %+v, want default with ai_fetch web_search", cfg.Fetch)
		}
	})

	t.Run("sample product through the search model", func(t *testing.T) {
		before := calls()
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": product, "request_id": c.SiteID + "-1"})
		r.Success(t)
		if r.Str("site_id") != c.SiteID || r.Str("source.engine") != "ai-crawler-avalai" {
			t.Errorf("site_id/engine = %q/%q", r.Str("site_id"), r.Str("source.engine"))
		}
		if r.At("source.site_contacted") != false {
			t.Errorf("source.site_contacted = %v, want false: the shop must never be contacted", r.At("source.site_contacted"))
		}
		if r.Str("product.title") != c.Title {
			t.Errorf("title = %q, want %q", r.Str("product.title"), c.Title)
		}
		if c.Price < 0 {
			if r.At("product.price.current") != nil {
				t.Errorf("price.current = %v, want null", r.At("product.price.current"))
			}
		} else if r.Int("product.price.current") != c.Price {
			t.Errorf("price = %d, want %d", r.Int("product.price.current"), c.Price)
		}
		if r.Str("product.price.state") != c.State {
			t.Errorf("state = %s, want %s", r.Str("product.price.state"), c.State)
		}
		if n := len(listOf(r.At("product.specs"))); n != c.Specs {
			t.Errorf("specs = %d, want %d", n, c.Specs)
		}
		if n := len(listOf(r.At("product.features"))); n != c.Features {
			t.Errorf("features = %d, want %d", n, c.Features)
		}
		if !hasWarning(r, "AI_UNVERIFIED") {
			t.Error("want an AI_UNVERIFIED warning: the values were not checked against the page")
		}
		if n := calls() - before; n != 1 {
			t.Errorf("provider calls = %d, want 1", n)
		}
	})

	t.Run("non-product page is rejected without a provider call", func(t *testing.T) {
		before := calls()
		r := e.Extract("https://" + c.Domain + c.CategoryPath)
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if calls() != before {
			t.Error("the search model must not be called for a non-product URL")
		}
	})

	t.Run("provider rate limit is AI_RATE_LIMITED", func(t *testing.T) {
		r := e.Extract("https://" + c.Domain + c.GonePath + "-ratelimited")
		r.Failed(t, "AI_RATE_LIMITED", "extract", "service", true)
	})
}
