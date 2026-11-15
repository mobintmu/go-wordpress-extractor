package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The mojeek, yahoo and yandex engines read a search engine's results page
// through github.com/jcalvert/metawebsearch and never contact the product's
// site. These tests run the real service with the three engines on and
// point each engine at the fake shop, which plays the search engine: it
// serves a results page in that engine's markup for the shop's own product
// URLs. The fake shop also stands in for the product site, which must never
// receive a request.

// Hosts: one shop per engine (its config selects that engine), and one host
// per search engine.
const (
	hostMojeekShop = "mojeekshop.test"
	hostYahooShop  = "yahooshop.test"
	hostYandexShop = "yandexshop.test"
	hostMojeek     = "mojeek.test"
	hostYahoo      = "yahoo.test"
	hostYandex     = "yandex.test"

	// scrapedPath is the product the results pages list; scrapedOther is a
	// product they do not.
	scrapedPath  = "/p/duravit-starck-3-undermount-sink"
	scrapedOther = "/p/duravit-vero-air-washbasin"
	scrapedUA    = "go-wordpress-extractor/e2e"
)

// scrapedSite is a shop whose config selects a search engine. It is never
// fetched; the selectors exist so that image_url, price and specs are fields
// the site extracts, which a search result then cannot provide.
func scrapedSite(id, host, engine string, alternates ...string) map[string]any {
	fetch := map[string]any{
		"engine": engine, "timeout_seconds": 5,
		"search": map[string]any{"region": "us-en", "title_strip": []string{" | Search Shop"}},
	}
	if len(alternates) > 0 {
		fetch["alternate_engines"] = alternates
	}
	return map[string]any{
		"schema_version": 1,
		"site_id":        id,
		"version":        "2026-11-14.1",
		"domains":        []string{host},
		"fetch":          fetch,
		"locale":         map[string]any{"price_unit": "usd"},
		"pages": map[string]any{"product": map[string]any{
			"match": []string{`^/p/[a-z0-9-]+/?$`},
			"root":  "main",
			"fields": map[string]any{
				"title":         map[string]any{"selector": "h1"},
				"image_url":     map[string]any{"selector": "img.product", "attr": "src"},
				"price_current": map[string]any{"selector": ".price"},
				"description":   map[string]any{"selector": ".description"},
			},
			"specs": map[string]any{"row": "table.specs tr", "name": "th", "value": "td"},
		}},
	}
}

// Results pages in each engine's markup, listing scrapedPath of shop at
// rank 2 after another product of the same shop.
func mojeekResults(shop string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>Mojeek Search</title></head><body>
<ul class="results-standard">
<li><h2><a href="https://%[1]s/p/duravit-starck-3-pedestal">Duravit Starck 3 Pedestal | Search Shop</a></h2><p class="s">Pedestal for the Starck 3 washbasin.</p></li>
<li><h2><a href="https://%[1]s%[2]s">Duravit Starck 3 Undermount Sink | Search Shop</a></h2><p class="s">Duravit 030549-0HOLE: ceramic undermount bathroom sink with overflow.</p></li>
</ul></body></html>`, shop, scrapedPath)
}

func yahooResults(shop string) string {
	wrap := func(u string) string {
		return "https://r.search.yahoo.com/_ylt=A0;_ylu=Y29/RV=2/RE=1/RO=10/RU=" + strings.NewReplacer(":", "%3a", "/", "%2f").Replace(u) + "/RK=2/RS=abc-"
	}
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>Yahoo Search Results</title></head><body><ol>
<li><div class="algo relsrch"><div class="compTitle Title"><h3><a href="https://www.bing.com/aclick?ld=ad">Sinks - Sponsored</a></h3></div><div class="compText Text">An ad.</div></div></li>
<li><div class="algo relsrch"><div class="compTitle Title"><h3><a href="%s">Duravit Starck 3 Pedestal | Search Shop</a></h3></div><div class="compText Text">Pedestal.</div></div></li>
<li><div class="algo relsrch"><div class="compTitle Title"><h3><a href="%s">Duravit Starck 3 Undermount Sink | Search Shop</a></h3></div><div class="compText Text">Duravit 030549-0HOLE: ceramic undermount bathroom sink with overflow.</div></div></li>
</ol></body></html>`, wrap("https://"+shop+"/p/duravit-starck-3-pedestal"), wrap("https://"+shop+scrapedPath))
}

func yandexResults(shop string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>Yandex</title></head><body><ul>
<li class="serp-item"><h3><a href="https://%[1]s/p/duravit-starck-3-pedestal">Duravit Starck 3 Pedestal | Search Shop</a></h3><div class="b-serp-item__text">Pedestal.</div></li>
<li class="serp-item"><h3><a href="https://%[1]s%[2]s">Duravit Starck 3 Undermount Sink | Search Shop</a></h3><div class="b-serp-item__text">Duravit 030549-0HOLE: ceramic undermount bathroom sink with overflow.</div></li>
</ul></body></html>`, shop, scrapedPath)
}

// scrapedEndpoints points each engine at the fake shop. The engines keep
// their own paths: Mojeek /search, Yahoo /search;_ylt=..., Yandex
// /search/site/.
func scrapedEndpoints(s *Shop) map[string]string {
	return map[string]string{
		"mojeek": s.URL(hostMojeek, ""),
		"yahoo":  s.URL(hostYahoo, ""),
		"yandex": s.URL(hostYandex, ""),
	}
}

func scrapedEnv(extra map[string]string) map[string]string {
	env := map[string]string{
		"APP_MOJEEK_ENABLED": "true", "APP_MOJEEK_MIN_INTERVAL_MS": "0",
		"APP_YAHOO_ENABLED": "true", "APP_YAHOO_MIN_INTERVAL_MS": "0",
		"APP_YANDEX_ENABLED": "true", "APP_YANDEX_MIN_INTERVAL_MS": "0",
		"APP_SEARCH_USER_AGENT": scrapedUA,
	}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// unavailableReasons maps unavailable_fields to field -> reason.
func unavailableReasons(r *Result) map[string]string {
	out := map[string]string{}
	fields, _ := r.At("unavailable_fields").([]any)
	for _, f := range fields {
		m, _ := f.(map[string]any)
		field, _ := m["field"].(string)
		reason, _ := m["reason"].(string)
		out[field] = reason
	}
	return out
}

func TestScrapedSearchEngines(t *testing.T) {
	e := Start(t, Options{
		Env:             scrapedEnv(nil),
		SearchEndpoints: scrapedEndpoints,
		ExtraSites: map[string]map[string]any{
			hostMojeekShop: scrapedSite("mojeekshop", hostMojeekShop, "mojeek"),
			hostYahooShop:  scrapedSite("yahooshop", hostYahooShop, "yahoo"),
			hostYandexShop: scrapedSite("yandexshop", hostYandexShop, "yandex"),
		},
	})
	s := e.Shop
	s.Handle("/search", mojeekResults(hostMojeekShop))
	s.HandleFunc("/search;", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, yahooResults(hostYahooShop))
	})
	s.Handle("/search/site/", yandexResults(hostYandexShop))

	for _, c := range []struct{ engine, shop, searchPath string }{
		{"mojeek", hostMojeekShop, "/search"},
		{"yahoo", hostYahooShop, ""},
		{"yandex", hostYandexShop, "/search/site/"},
	} {
		t.Run(c.engine+" builds the product from the result", func(t *testing.T) {
			productURL := "https://" + c.shop + scrapedPath
			r := e.Extract(productURL)
			r.Success(t)
			if got := r.Str("source.engine"); got != c.engine {
				t.Errorf("source.engine = %q, want %s", got, c.engine)
			}
			if r.At("source.site_contacted") != false || r.Int("source.result_rank") != 2 {
				t.Errorf("source = %v", r.At("source"))
			}
			if q := r.Str("source.query"); !strings.HasPrefix(q, "site:"+c.shop+" ") {
				t.Errorf("source.query = %q", q)
			}
			if got := r.Str("product.title"); got != "Duravit Starck 3 Undermount Sink" {
				t.Errorf("product.title = %q (title_strip must remove the shop name)", got)
			}
			if got := r.Str("product.url"); got != productURL {
				t.Errorf("product.url = %q", got)
			}
			if !strings.HasPrefix(r.Str("product.description"), "Duravit 030549-0HOLE") {
				t.Errorf("product.description = %q", r.Str("product.description"))
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
			if reasons["availability"] != "NOT_CONFIGURED" || len(reasons) != 4 {
				t.Errorf("unavailable_fields = %v", reasons)
			}
			if s.Hits(scrapedPath) != 0 {
				t.Error("the product page was fetched; a search engine must never contact the site")
			}
			if c.searchPath == "" {
				return
			}
			h := s.RequestHeaders(c.searchPath)
			if h == nil {
				t.Fatalf("no request reached %s", c.searchPath)
			}
			if got := h.Get("User-Agent"); got != scrapedUA {
				t.Errorf("User-Agent = %q, want the honest %q", got, scrapedUA)
			}
			for _, name := range []string{"Sec-Fetch-Mode", "Sec-Fetch-Dest", "Sec-Fetch-Site", "Sec-Fetch-User", "Sec-CH-UA", "Sec-CH-UA-Platform", "Upgrade-Insecure-Requests"} {
				if v := h.Get(name); v != "" {
					t.Errorf("%s = %q: the request must not claim to be a browser", name, v)
				}
			}
		})
	}

	t.Run("mojeek sends the region as cookies", func(t *testing.T) {
		req := &http.Request{Header: s.RequestHeaders("/search")}
		got := map[string]string{}
		for _, c := range req.Cookies() {
			got[c.Name] = c.Value
		}
		if got["arc"] != "US" || got["lb"] != "en" {
			t.Errorf("cookies = %v, want arc=US lb=en for region us-en", got)
		}
	})

	t.Run("a repeated request is answered from the cache", func(t *testing.T) {
		before := s.Hits("/search/site/")
		r := e.Extract("https://" + hostYandexShop + scrapedPath)
		r.Success(t)
		if r.At("source.cached") != true || s.Hits("/search/site/") != before {
			t.Errorf("cached = %v, yandex hits %d -> %d", r.At("source.cached"), before, s.Hits("/search/site/"))
		}
	})

	t.Run("a product the engine does not list is SEARCH_NO_RESULT", func(t *testing.T) {
		before := s.Hits("/search")
		r := e.Extract("https://" + hostMojeekShop + scrapedOther)
		r.Failed(t, "SEARCH_NO_RESULT", "fetch", "source_site", false)
		if got := r.Str("evidence.search.engine"); got != "mojeek" {
			t.Errorf("evidence.search.engine = %q", got)
		}
		queries, _ := r.At("evidence.search.queries").([]any)
		if len(queries) != 2 || s.Hits("/search")-before != 2 {
			t.Errorf("queries = %v, mojeek hits = %d (site_slug, then url)", queries, s.Hits("/search")-before)
		}
		closest, _ := r.At("evidence.search.closest").([]any)
		if len(closest) != 2 {
			t.Errorf("closest = %v, want the two listed products of the shop", closest)
		}
		if s.Hits(scrapedOther) != 0 {
			t.Error("the product page was fetched")
		}
	})
}

// When a site's engine is off on the instance, the first enabled alternate
// serves it; with none enabled the request fails before any fetch.
func TestScrapedSearchEngineAlternates(t *testing.T) {
	e := Start(t, Options{
		Env:             scrapedEnv(map[string]string{"APP_MOJEEK_ENABLED": "false", "APP_YAHOO_ENABLED": "false"}),
		SearchEndpoints: scrapedEndpoints,
		ExtraSites: map[string]map[string]any{
			hostMojeekShop: scrapedSite("mojeekshop", hostMojeekShop, "mojeek", "yandex"),
			hostYahooShop:  scrapedSite("yahooshop", hostYahooShop, "yahoo"),
		},
	})
	e.Shop.Handle("/search/site/", yandexResults(hostMojeekShop))

	r := e.Extract("https://" + hostMojeekShop + scrapedPath)
	r.Success(t)
	if got := r.Str("source.engine"); got != "yandex" {
		t.Errorf("source.engine = %q, want the alternate yandex", got)
	}

	r = e.Extract("https://" + hostYahooShop + scrapedPath)
	r.Failed(t, "ENGINE_UNAVAILABLE", "fetch", "service", true)
	if !strings.Contains(r.Str("error.root_cause"), `"yahoo"`) {
		t.Errorf("root_cause = %q", r.Str("error.root_cause"))
	}
	if e.Shop.TotalHits() != 1 {
		t.Errorf("shop hits = %d, want only the yandex query", e.Shop.TotalHits())
	}
}

// A rate-limit answer is reported once. The library would retry it three
// times with growing waits; the service does not work around a block.
func TestScrapedSearchEngineBlocked(t *testing.T) {
	e := Start(t, Options{
		Env:             scrapedEnv(nil),
		SearchEndpoints: scrapedEndpoints,
		ExtraSites:      map[string]map[string]any{hostMojeekShop: scrapedSite("mojeekshop", hostMojeekShop, "mojeek")},
	})
	e.Shop.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "slow down")
	})

	r := e.Extract("https://" + hostMojeekShop + scrapedPath)
	r.Failed(t, "SEARCH_PROVIDER_BLOCKED", "fetch", "service", true)
	if !strings.Contains(r.Str("error.root_cause"), "Mojeek answered HTTP 429") {
		t.Errorf("root_cause = %q", r.Str("error.root_cause"))
	}
	if n := e.Shop.Hits("/search"); n != 1 {
		t.Errorf("mojeek was queried %d times, want 1: a block is not retried", n)
	}
	if r.Elapsed > 3e9 {
		t.Errorf("the request took %s; the library's retry backoff must be off", r.Elapsed)
	}

	// A second request is not answered from the cache: blocks are not cached.
	r = e.Extract("https://" + hostMojeekShop + scrapedPath)
	r.Failed(t, "SEARCH_PROVIDER_BLOCKED", "fetch", "service", true)
	if n := e.Shop.Hits("/search"); n != 2 {
		t.Errorf("mojeek hits = %d, want 2", n)
	}
}

// The service does not start with an interval outside the allowed range.
func TestScrapedSearchEngineBadInterval(t *testing.T) {
	keys := filepath.Join(t.TempDir(), "api_keys.json")
	must(t, os.WriteFile(keys, []byte("[]"), 0o600))
	for k, v := range map[string]string{
		"APP_ENV": "test", "APP_HTTP_PORT": "1", "APP_HTTP_ADDRESS": "127.0.0.1", "APP_API_KEY_PEPPER": testPepper,
		"APP_SITE_CONFIG_DIR": filepath.Join(repoRoot, "configs"), "APP_API_KEYS_FILE": keys,
		"APP_YAHOO_ENABLED": "true", "APP_YAHOO_MIN_INTERVAL_MS": "99999",
	} {
		t.Setenv(k, v)
	}
	if err := newAppForStartupTest().Err(); err == nil || !strings.Contains(err.Error(), "APP_YAHOO_MIN_INTERVAL_MS") {
		t.Fatalf("startup error = %v, want the interval named", err)
	}
}
