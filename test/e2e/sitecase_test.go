package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// edit is one change made to the saved page to build a variant. Old must be
// found exactly once, or every time when All is set.
type edit struct {
	Old, New string
	All      bool
}

// siteCase describes one site's end-to-end test: the real config in
// configs/sites/<Domain>.json, the saved page in
// testdata/sites/<SiteID>/<Fixture>.html, and the variants built from it.
// runSiteCase turns it into the subtests every site needs. Leave a variant's
// edits nil to skip that subtest (say why in the site's test file).
type siteCase struct {
	SiteID, Domain, Fixture string
	// Path is the sample product's decoded path; CategoryPath is a path the
	// match patterns reject; GonePath matches but is not served.
	Path, CategoryPath, GonePath string
	// Variant is the path template for variants of the page, with {v} for
	// the variant name and {n} for its number (for sites whose product paths
	// are numeric); it must match the product patterns. Default:
	// Path + "-{v}".
	Variant string
	// Engine is the engine the config selects.
	Engine string

	Title string
	// Price is price.current in cents, or -1 when the site shows no price
	// (then price.current is checked to be null).
	Price int64
	// Regular is price.regular in cents, or 0 for present-and-null.
	Regular int64
	State   string
	// Canonical is the prefix of product.url; empty when there is no
	// canonical link.
	Canonical string

	// MissingTitleCode, when set, expects REQUIRED_FIELD_MISSING for a
	// layout change (the root still matches but the title is gone).
	MissingTitleCode bool

	// Warnings are the warning codes expected on the sample page, in order.
	Warnings []string

	// SpecCount and Specs (name -> {group, value}) are checked when the
	// config has specs.
	SpecCount int
	Specs     map[string][2]string

	// NoSpecs removes the specs block; Layout breaks root.
	NoSpecs, Layout []edit
	// Sale replaces the price block with a sale price SaleCurrent/SaleRegular.
	Sale                     []edit
	SaleCurrent, SaleRegular int64
	// OutOfStock and CallForPrice make those page states.
	OutOfStock, CallForPrice []edit
}

func (c siteCase) host() string { return strings.ReplaceAll(c.SiteID, "_", "-") + ".test" }

// config loads the real site config and points it at the fake shop. The real
// domains stay, so a canonical link on them is honoured as in production.
func (c siteCase) config(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", c.Domain+".json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, c.host())
	f, _ := cfg["fetch"].(map[string]any)
	if f == nil {
		f = map[string]any{}
	}
	if f["engine"] == "browser" {
		// The suite has no Chrome; the fixture is the page Chrome got.
		f["engine"] = "http"
	}
	f["delay_ms"], f["timeout_seconds"], f["retries"], f["respect_robots_txt"] = 0, 5, 2, true
	cfg["fetch"] = f
	return cfg
}

// fixture returns the saved page and its expected product.
func (c siteCase) fixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", c.SiteID)
	html, err := os.ReadFile(filepath.Join(dir, c.Fixture+".html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, c.Fixture+".json"))
	must(t, err)
	var fx struct {
		Expect struct {
			Product map[string]any `json:"product"`
		} `json:"expect"`
	}
	must(t, json.Unmarshal(raw, &fx))
	if fx.Expect.Product == nil {
		t.Fatal("fixture has no expected product")
	}
	return string(html), fx.Expect.Product
}

// applyEdits applies edits to html and fails when a target is missing.
func applyEdits(t *testing.T, html string, edits []edit) string {
	t.Helper()
	for _, e := range edits {
		n := strings.Count(html, e.Old)
		switch {
		case n == 0:
			t.Fatalf("%q not found in the saved page", e.Old)
		case n > 1 && !e.All:
			t.Fatalf("%q found %d times in the saved page, want 1", e.Old, n)
		}
		html = strings.ReplaceAll(html, e.Old, e.New)
	}
	return html
}

func listOf(v any) []any {
	l, _ := v.([]any)
	return l
}

// runSiteCase runs the standard subtests for one site through the full HTTP
// service, against the saved page and variants of it.
func runSiteCase(t *testing.T, c siteCase) {
	t.Helper()
	page, want := c.fixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{c.host(): c.config(t)},
		// Saved pages are often larger than the suite's 256 KB default.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 4 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(c.host(), (&url.URL{Path: path}).EscapedPath())
	}
	variant := func(name string, edits []edit) string {
		path := strings.TrimSuffix(c.Path, "/") + "-" + name
		if c.Variant != "" {
			n := map[string]string{"no-specs": "1", "sale": "2", "out-of-stock": "3", "call": "4", "changed": "5"}[name]
			path = strings.NewReplacer("{v}", name, "{n}", n).Replace(c.Variant)
		}
		s.Handle(path, applyEdits(t, page, edits))
		return productURL(path)
	}
	s.Handle(c.Path, page)

	t.Run("sample product with the selected engine", func(t *testing.T) {
		if raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", c.Domain+".json")); err == nil {
			var cfg struct {
				Fetch struct {
					Engine string `json:"engine"`
				} `json:"fetch"`
			}
			must(t, json.Unmarshal(raw, &cfg))
			if got := cmp(cfg.Fetch.Engine, "http"); got != c.Engine {
				t.Errorf("config fetch.engine = %q, want %q", got, c.Engine)
			}
		}
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(c.Path), "request_id": c.SiteID + "-1"})
		r.Success(t)
		if r.Str("site_id") != c.SiteID || r.Str("request_id") != c.SiteID+"-1" {
			t.Errorf("site_id/request_id = %q/%q", r.Str("site_id"), r.Str("request_id"))
		}
		if r.Str("source.engine") != "http" {
			t.Errorf("source.engine = %q, want http", r.Str("source.engine"))
		}
	})

	t.Run("saved live page matches the fixture", func(t *testing.T) {
		r := e.Extract(productURL(c.Path))
		r.Success(t)
		for _, f := range []string{"title", "url", "image_url", "description"} {
			if f == "url" && c.Canonical == "" {
				continue // product.url is the requested (fake) URL
			}
			exp, _ := want[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
		}
		price, _ := want["price"].(map[string]any)
		for _, f := range []string{"currency", "raw", "state"} {
			exp, _ := price[f].(string)
			if got := r.Str("product.price." + f); got != exp {
				t.Errorf("product.price.%s = %q, want %q", f, got, exp)
			}
		}
		if got, exp := r.At("product.specs"), want["specs"]; !reflect.DeepEqual(got, exp) {
			t.Errorf("product.specs differs from the fixture: got %d rows, want %d", len(listOf(got)), len(listOf(exp)))
		}
		var codes []string
		for _, w := range listOf(r.At("warnings")) {
			codes = append(codes, w.(map[string]any)["code"].(string))
		}
		if strings.Join(codes, ",") != strings.Join(c.Warnings, ",") {
			t.Errorf("warnings = %v, want %v", codes, c.Warnings)
		}
	})

	t.Run("sample product details", func(t *testing.T) {
		r := e.Extract(productURL(c.Path))
		r.Success(t)
		if r.Str("product.title") != c.Title {
			t.Errorf("title = %q, want %q", r.Str("product.title"), c.Title)
		}
		if c.Price < 0 {
			if !r.Has("product.price.current") || r.At("product.price.current") != nil {
				t.Errorf("current must be null, got %v", r.At("product.price.current"))
			}
		} else if r.Int("product.price.current") != c.Price {
			t.Errorf("price = %d, want %d", r.Int("product.price.current"), c.Price)
		}
		if c.Regular == 0 {
			if r.Has("product.price.regular") && r.At("product.price.regular") != nil {
				t.Errorf("regular must be null, got %v", r.At("product.price.regular"))
			}
		} else if r.Int("product.price.regular") != c.Regular {
			t.Errorf("regular = %d, want %d", r.Int("product.price.regular"), c.Regular)
		}
		if r.Str("product.price.state") != c.State {
			t.Errorf("state = %s, want %s", r.Str("product.price.state"), c.State)
		}
	})

	if c.Canonical != "" {
		t.Run("canonical URL wins over the fetched host", func(t *testing.T) {
			r := e.Extract(productURL(c.Path))
			r.Success(t)
			if got := r.Str("product.url"); !strings.HasPrefix(got, c.Canonical) {
				t.Errorf("product.url = %s, want prefix %s", got, c.Canonical)
			}
		})
	}

	if c.SpecCount > 0 {
		t.Run("specs", func(t *testing.T) {
			r := e.Extract(productURL(c.Path))
			r.Success(t)
			specs := map[string]map[string]any{}
			for _, s := range listOf(r.At("product.specs")) {
				m := s.(map[string]any)
				specs[m["name"].(string)] = m
			}
			for name, exp := range c.Specs {
				got := specs[name]
				group, _ := got["group"].(string)
				if got == nil || group != exp[0] || got["value"] != exp[1] {
					t.Errorf("spec %q = %v, want group %q value %q", name, got, exp[0], exp[1])
				}
			}
			if n := len(listOf(r.At("product.specs"))); n != c.SpecCount {
				t.Errorf("got %d specs, want %d", n, c.SpecCount)
			}
		})
	}

	if c.NoSpecs != nil {
		t.Run("specs are optional", func(t *testing.T) {
			r := e.Extract(variant("no-specs", c.NoSpecs))
			r.Success(t)
			if r.Has("product.specs") {
				t.Errorf("specs must be omitted, got %d rows", len(listOf(r.At("product.specs"))))
			}
			if r.Str("product.title") != c.Title {
				t.Errorf("title = %q", r.Str("product.title"))
			}
			if c.Price >= 0 && r.Int("product.price.current") != c.Price {
				t.Errorf("price = %d", r.Int("product.price.current"))
			}
		})
	}

	if c.Sale != nil {
		t.Run("sale price", func(t *testing.T) {
			r := e.Extract(variant("sale", c.Sale))
			r.Success(t)
			if r.Int("product.price.current") != c.SaleCurrent || r.Int("product.price.regular") != c.SaleRegular {
				t.Errorf("prices = %d/%d, want %d/%d", r.Int("product.price.current"), r.Int("product.price.regular"), c.SaleCurrent, c.SaleRegular)
			}
		})
	}

	if c.OutOfStock != nil {
		t.Run("out of stock keeps the price", func(t *testing.T) {
			r := e.Extract(variant("out-of-stock", c.OutOfStock))
			r.Success(t)
			if r.Str("product.price.state") != "out_of_stock" {
				t.Errorf("state = %s, want out_of_stock", r.Str("product.price.state"))
			}
			if c.Price >= 0 && r.Int("product.price.current") != c.Price {
				t.Errorf("price = %d, want %d", r.Int("product.price.current"), c.Price)
			}
		})
	}

	if c.CallForPrice != nil {
		t.Run("call for price", func(t *testing.T) {
			r := e.Extract(variant("call", c.CallForPrice))
			r.Success(t)
			if r.Str("product.price.state") != "call_for_price" {
				t.Errorf("state = %s", r.Str("product.price.state"))
			}
			if !r.Has("product.price.current") || r.At("product.price.current") != nil {
				t.Errorf("current must be null, never 0, got %v", r.At("product.price.current"))
			}
		})
	}

	if c.Layout != nil {
		t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
			r := e.Extract(variant("changed", c.Layout))
			if c.MissingTitleCode {
				// The root still matches; only a required field is gone, which
				// the service reports at the validate stage.
				r.Failed(t, "REQUIRED_FIELD_MISSING", "validate", "site_config", false)
			} else {
				r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
			}
			if r.Str("site_id") != c.SiteID {
				t.Errorf("site_id = %q", r.Str("site_id"))
			}
		})

	}
	t.Run("non-product page is not fetched", func(t *testing.T) {
		r := e.Extract(productURL(c.CategoryPath))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(c.CategoryPath) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL(c.GonePath))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}

// cmp returns v, or def when v is empty.
func cmp(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// aiSiteCase describes a site whose config follows the default AI engine
// (fetch.engine "default", resolved from APP_DEFAULT_ENGINE, which these tests
// set to ai-crawler-avalai). The fake LLM replays the answer recorded live in
// testdata/sites/<SiteID>/<Fixture>.ai-crawler-avalai.json; AvalAI is never
// called. A copy of the config with the http engine checks the fallback
// selectors against the saved page.
type aiSiteCase struct {
	SiteID, Domain, Fixture string
	Path, CategoryPath      string
	// Variant is a product path template ({v} = variant name) for the
	// layout-change and provider-error pages.
	Variant string
	Title   string
	// Price is price.current in cents, or -1 for null.
	Price     int64
	State     string
	Canonical string
	// Specs and Features are the counts in the recorded answer.
	Specs, Features int
	// Warnings are the expected warning codes besides AI_EXTRACTED.
	Warnings []string
	// Layout breaks the fallback selectors' root.
	Layout []edit
}

// recordedAI returns the recorded product.
func (c aiSiteCase) recordedAI(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "sites", c.SiteID, c.Fixture+".ai-crawler-avalai.json"))
	must(t, err)
	var rec map[string]any
	must(t, json.Unmarshal(raw, &rec))
	p, _ := rec["product"].(map[string]any)
	if p == nil {
		t.Fatal("recorded answer has no product")
	}
	return p
}

// llmAnswer turns a recorded product into the model's JSON answer.
func llmAnswer(p map[string]any) string {
	price, _ := p["price"].(map[string]any)
	var current any
	if raw, _ := price["raw"].(string); raw != "" && price["current"] != nil {
		current = raw
	}
	specs := []any{}
	for _, s := range listOf(p["specs"]) {
		m := s.(map[string]any)
		specs = append(specs, map[string]any{"group": m["group"], "name": m["name"], "value": m["value"]})
	}
	features := listOf(p["features"])
	if features == nil {
		features = []any{}
	}
	b, _ := json.Marshal(map[string]any{
		"is_product": true, "name": p["title"], "url": p["url"], "image": p["image_url"], "description": p["description"],
		"price":    map[string]any{"current_text": current, "regular_text": nil, "currency": price["currency"], "state": price["state"]},
		"specs":    specs,
		"features": features,
	})
	return string(b)
}

// aiConfig loads the real config for host; engine, when set, replaces
// fetch.engine and the copy claims only host under its own site_id.
func (c aiSiteCase) aiConfig(t *testing.T, host, engine string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", c.Domain+".json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, host)
	f, _ := cfg["fetch"].(map[string]any)
	if engine != "" {
		f["engine"] = engine
		delete(f, "ai_fetch")
		cfg["domains"] = []any{host}
		cfg["site_id"] = c.SiteID + "-" + engine
	}
	if f["ai_fetch"] == "browser" {
		// The suite has no Chrome; the AI engine fetches the page itself.
		f["ai_fetch"] = "http"
	}
	f["delay_ms"], f["timeout_seconds"], f["retries"], f["respect_robots_txt"] = 0, 5, 2, true
	cfg["fetch"] = f
	return cfg
}

// runAISiteCase runs the subtests for a site that follows the default AI engine.
func runAISiteCase(t *testing.T, c aiSiteCase) {
	t.Helper()
	page, want := siteCase{SiteID: c.SiteID, Fixture: c.Fixture}.fixture(t)
	rec := c.recordedAI(t)
	answer := llmAnswer(rec)
	host := strings.ReplaceAll(c.SiteID, "_", "-") + ".test"
	hostHTTP := strings.ReplaceAll(c.SiteID, "_", "-") + "-http.test"
	variant := func(v string) string { return strings.ReplaceAll(c.Variant, "{v}", v) }

	llm := newFakeLLM(t)
	llm.respond = func(w http.ResponseWriter, call llmCall, n int) {
		switch {
		case strings.Contains(call.PageURL, variant("ratelimited")):
			chatError(w, 429, "rate_limit_exceeded", "Rate limit reached", map[string]string{"Retry-After": "0"})
		case strings.Contains(call.PageURL, variant("down")):
			chatError(w, 503, "model_overloaded", "Model overloaded", nil)
		default:
			chatOK(w, answer, "stop")
		}
	}
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{host: c.aiConfig(t, host, ""), hostHTTP: c.aiConfig(t, hostHTTP, "http")},
		Env:        aiEnv(llm, map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-avalai"}),
		Fetch:      func(o *fetch.Options) { o.MaxBodyBytes = 4 << 20 },
	})
	s := e.Shop
	productURL := func(h, path string) string { return s.URL(h, (&url.URL{Path: path}).EscapedPath()) }
	s.Handle(c.Path, page)
	calls := func() int { return llm.count(false) }

	t.Run("config selects the default engine", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", c.Domain+".json"))
		must(t, err)
		var cfg struct {
			Fetch struct {
				Engine string `json:"engine"`
			} `json:"fetch"`
		}
		must(t, json.Unmarshal(raw, &cfg))
		if cfg.Fetch.Engine != "default" {
			t.Errorf("fetch.engine = %q, want default", cfg.Fetch.Engine)
		}
	})

	t.Run("sample product with the mocked AI engine", func(t *testing.T) {
		before := calls()
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(host, c.Path), "request_id": c.SiteID + "-1"})
		r.Success(t)
		if r.Str("site_id") != c.SiteID || r.Str("source.engine") != "ai-crawler-avalai" {
			t.Errorf("site_id/engine = %q/%q", r.Str("site_id"), r.Str("source.engine"))
		}
		for _, f := range []string{"title", "url", "image_url", "description"} {
			exp, _ := rec[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
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
		for _, w := range append([]string{"AI_EXTRACTED"}, c.Warnings...) {
			if !hasWarning(r, w) {
				t.Errorf("want warning %s", w)
			}
		}
		if n := calls() - before; n != 1 {
			t.Errorf("LLM calls = %d, want 1", n)
		}
	})

	t.Run("saved page matches the fixture with the http fallback selectors", func(t *testing.T) {
		before := calls()
		r := e.Extract(productURL(hostHTTP, c.Path))
		r.Success(t)
		for _, f := range []string{"title", "image_url", "description"} {
			exp, _ := want[f].(string)
			got := r.Str("product." + f)
			if f == "image_url" {
				// The copy cannot claim the real domain, so a relative
				// image resolves against the fake host: compare paths.
				g, _ := url.Parse(got)
				x, _ := url.Parse(exp)
				got, exp = g.Path, x.Path
			}
			if got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
		}
		if calls() != before {
			t.Error("the http engine must not call the LLM")
		}
	})

	if c.Canonical != "" {
		t.Run("canonical URL wins over the fetched host", func(t *testing.T) {
			r := e.Extract(productURL(host, c.Path))
			r.Success(t)
			if got := r.Str("product.url"); !strings.HasPrefix(got, c.Canonical) {
				t.Errorf("product.url = %s, want prefix %s", got, c.Canonical)
			}
		})
	}

	if c.Layout != nil {
		t.Run("layout change: fallback selectors report SELECTOR_NOT_FOUND", func(t *testing.T) {
			path := variant("changed")
			s.Handle(path, applyEdits(t, page, c.Layout))
			r := e.Extract(productURL(hostHTTP, path))
			r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		})

	}
	t.Run("non-product page is not fetched and the LLM is not called", func(t *testing.T) {
		before := calls()
		r := e.Extract(productURL(host, c.CategoryPath))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(c.CategoryPath) != 0 || calls() != before {
			t.Errorf("hits = %d, LLM calls = %d; want none", s.Hits(c.CategoryPath), calls()-before)
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND without an LLM call", func(t *testing.T) {
		before := calls()
		r := e.Extract(productURL(host, variant("gone")))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
		if calls() != before {
			t.Error("the LLM must not be called for a missing page")
		}
	})

	t.Run("provider errors", func(t *testing.T) {
		for _, tc := range []struct {
			v, code   string
			retryable bool
		}{
			{"ratelimited", "AI_RATE_LIMITED", true},
			{"down", "AI_UNAVAILABLE", true},
		} {
			s.Handle(variant(tc.v), page)
			r := e.Extract(productURL(host, variant(tc.v)))
			r.Failed(t, tc.code, "extract", "service", tc.retryable)
		}
	})
}
