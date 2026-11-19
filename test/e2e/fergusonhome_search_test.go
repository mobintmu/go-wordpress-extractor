package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ferguson answers plain automated requests with HTTP 403. From 2026-11-13
// to 2026-11-19 the live config used a search engine, which never fetches
// the site; it now uses ai-crawler-avalai in Chrome. These tests keep the
// search route working: they send the client's sample request through every
// search engine with the real config and its former search settings, and the fake shop replays what each engine really
// answered for it (testdata/search/, saved 2026-11-13 for DuckDuckGo and
// 2026-11-15 for the others):
//
//	duckduckgo  the product at rank 1 of the first query
//	yahoo       the product at rank 1 of the first query
//	mojeek      a CAPTCHA page for the honest client
//	yandex      a CAPTCHA page; on another try, other Ferguson products only
//
// Brave needs an API key and is covered with a synthetic answer elsewhere.

// fergusonSampleRequest is the request clients send for the sample product.
var fergusonSampleRequest = map[string]any{
	"request_id": "ferguson-1",
	"url":        "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155",
}

const fergusonCanonical = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394"

// Hosts the search engines are moved to in the fake shop.
const (
	hostDDGEngine    = "duckduckgo-engine.test"
	hostMojeekEngine = "mojeek-engine.test"
	hostYahooEngine  = "yahoo-engine.test"
	hostYandexEngine = "yandex-engine.test"
)

func searchFixture(t *testing.T, engine, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "search", engine, name))
	must(t, err)
	return string(b)
}

// fergusonSearchSettings is the fetch.search block fergusonhome.com used
// while it was served by duckduckgo (2026-11-13 to 2026-11-19). The live
// config now uses ai-crawler-avalai; these settings are kept here, and in
// the config's $comment, so the site can be switched back.
func fergusonSearchSettings() map[string]any {
	return map[string]any{
		"region":        "us-en",
		"title_strip":   []string{" | Ferguson Home"},
		"ignore_params": []string{"uid"},
		"queries":       []string{"site_slug", "url"},
	}
}

// fergusonSearchSite is the real fergusonhome.com config with engine as its
// only engine and the search settings above.
func fergusonSearchSite(t *testing.T, engine string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "fergusonhome.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	fetch, _ := cfg["fetch"].(map[string]any)
	if fetch == nil {
		t.Fatal("fergusonhome.com.json has no fetch block")
	}
	fetch["engine"] = engine
	fetch["search"] = fergusonSearchSettings()
	delete(fetch, "alternate_engines")
	delete(fetch, "ai_fetch")
	return cfg
}

// startFergusonSearch starts the service with only engine enabled and its
// answers served by serve.
func startFergusonSearch(t *testing.T, engine string, serve func(*Shop)) *Env {
	t.Helper()
	env := map[string]string{"APP_SEARCH_USER_AGENT": scrapedUA}
	key := "APP_" + strings.ToUpper(engine)
	env[key+"_ENABLED"], env[key+"_MIN_INTERVAL_MS"] = "true", "0"
	e := Start(t, Options{
		Env: env,
		SearchEndpoints: func(s *Shop) map[string]string {
			return map[string]string{
				"duckduckgo": s.URL(hostDDGEngine, "/html/"),
				"mojeek":     s.URL(hostMojeekEngine, ""),
				"yahoo":      s.URL(hostYahooEngine, ""),
				"yandex":     s.URL(hostYandexEngine, ""),
			}
		},
		// The real domains stay, so the result URL matches the request.
		ExtraSites: map[string]map[string]any{"fergusonhome.com": fergusonSearchSite(t, engine)},
	})
	serve(e.Shop)
	return e
}

// searchPaths are where each engine sends its queries.
var searchPaths = map[string]string{
	"duckduckgo": "/html/",
	"mojeek":     "/search",
	"yahoo":      "/search;", // /search;_ylt=<random>;_ylu=<random>
	"yandex":     "/search/site/",
}

func serveSearchPage(engine, page string) func(*Shop) {
	return func(s *Shop) {
		s.HandleFunc(searchPaths[engine], func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
		})
	}
}

func TestFergusonHomeSearchEngines(t *testing.T) {
	for _, c := range []struct{ engine, page string }{
		{"duckduckgo", "fergusonhome-site-slug.html"},
		{"yahoo", "fergusonhome-site-slug.html"},
	} {
		t.Run(c.engine+" finds the sample product", func(t *testing.T) {
			e := startFergusonSearch(t, c.engine, serveSearchPage(c.engine, searchFixture(t, c.engine, c.page)))
			r := e.Post("/api/v1/extract", e.Key, fergusonSampleRequest)
			r.Success(t)
			if r.Str("request_id") != "ferguson-1" || r.Str("site_id") != "fergusonhome" {
				t.Errorf("request_id/site_id = %q/%q", r.Str("request_id"), r.Str("site_id"))
			}
			if r.Str("source.engine") != c.engine || r.At("source.site_contacted") != false || r.Int("source.result_rank") != 1 {
				t.Errorf("source = %v", r.At("source"))
			}
			if q := r.Str("source.query"); q != "site:fergusonhome.com duravit 030549 0hole" {
				t.Errorf("source.query = %q, want the site_slug query (the first one matched)", q)
			}
			if got := r.Str("product.url"); got != fergusonCanonical {
				t.Errorf("product.url = %q, want the result URL without the uid variant", got)
			}
			title := r.Str("product.title")
			if !strings.HasPrefix(title, "Duravit 0305490017 Starck 3") || strings.HasSuffix(title, "...") || strings.Contains(title, "Ferguson Home") {
				t.Errorf("product.title = %q", title)
			}
			if r.Str("product.description") == "" {
				t.Error("product.description is empty; it is the result snippet")
			}
			if r.Str("product.price.currency") != "USD" || r.Str("product.price.state") != "unknown" {
				t.Errorf("price = %v", r.At("product.price"))
			}
			if r.Str("completeness") != "partial" {
				t.Errorf("completeness = %q", r.Str("completeness"))
			}
			reasons := unavailableReasons(r)
			for _, f := range []string{"image_url", "price", "specs"} {
				if reasons[f] != "NOT_IN_SEARCH_RESULTS" {
					t.Errorf("unavailable_fields[%s] = %q", f, reasons[f])
				}
			}
			if n := e.Shop.Hits(searchPathHit(e.Shop, c.engine)); n != 1 {
				t.Errorf("%s was queried %d times, want 1 (the first query matched)", c.engine, n)
			}
		})
	}

	for _, engine := range []string{"mojeek", "yandex"} {
		t.Run(engine+" CAPTCHA is SEARCH_PROVIDER_BLOCKED", func(t *testing.T) {
			e := startFergusonSearch(t, engine, serveSearchPage(engine, searchFixture(t, engine, "captcha.html")))
			r := e.Post("/api/v1/extract", e.Key, fergusonSampleRequest)
			r.Failed(t, "SEARCH_PROVIDER_BLOCKED", "fetch", "service", true)
			if !strings.Contains(r.Str("error.root_cause"), "bot check") || r.Str("evidence.search.engine") != engine {
				t.Errorf("root_cause %q, evidence %v", r.Str("error.root_cause"), r.At("evidence.search"))
			}
			if n := e.Shop.Hits(searchPaths[engine]); n != 1 {
				t.Errorf("%s was queried %d times, want 1: a block stops the request and is not retried", engine, n)
			}
		})
	}

	t.Run("yandex without the product is SEARCH_NO_RESULT, never another product", func(t *testing.T) {
		e := startFergusonSearch(t, "yandex", serveSearchPage("yandex", searchFixture(t, "yandex", "fergusonhome-no-match.html")))
		r := e.Post("/api/v1/extract", e.Key, fergusonSampleRequest)
		r.Failed(t, "SEARCH_NO_RESULT", "fetch", "source_site", false)
		queries, _ := r.At("evidence.search.queries").([]any)
		if len(queries) != 2 || e.Shop.Hits("/search/site/") != 2 {
			t.Errorf("queries = %v, yandex hits = %d: site_slug, then url", queries, e.Shop.Hits("/search/site/"))
		}
		closest, _ := r.At("evidence.search.closest").([]any)
		if len(closest) == 0 {
			t.Errorf("closest = %v, want the other Ferguson product pages Yandex listed", closest)
		}
		for _, u := range closest {
			if s, _ := u.(string); !strings.HasPrefix(s, "https://www.fergusonhome.com/") || strings.Contains(s, "s1666394") {
				t.Errorf("closest %q", s)
			}
		}
	})
}

// searchPathHit returns the path the shop recorded for engine's query.
// Yahoo's path carries random tokens, so it is looked up by prefix.
func searchPathHit(s *Shop, engine string) string {
	p := searchPaths[engine]
	if !strings.HasSuffix(p, ";") {
		return p
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for path := range s.hits {
		if strings.HasPrefix(path, p) {
			return path
		}
	}
	return p
}
