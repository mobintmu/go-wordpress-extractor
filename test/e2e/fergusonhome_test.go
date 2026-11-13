package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// hostFergusonHome stands in for www.fergusonhome.com in the fake shop.
const hostFergusonHome = "fergusonhome.test"

// fergusonPath is the product path of the saved page. The page itself was
// fetched with ?uid=3938155, the white variant of the sink.
const fergusonPath = "/duravit-030549-0hole/s1666394"

// fergusonSite loads the real fergusonhome.com config and points it at the
// fake shop. The real domains stay, so the page's canonical link
// (https://www.fergusonhome.com/...) is honoured exactly as in production.
// The live site uses a search engine (brave, alternate duckduckgo) and is
// never fetched; the saved page is static, so the suite fetches it with the
// default http engine to keep checking the selectors. It sends a browser
// user agent, so the request also carries the client hints and Sec-Fetch
// headers the http engine adds for one.
func fergusonSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "fergusonhome.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostFergusonHome)
	cfg["fetch"] = map[string]any{
		"user_agent": fergusonUserAgent(t, cfg), "delay_ms": 0, "timeout_seconds": 5,
		"retries": 2, "respect_robots_txt": true,
	}
	return cfg
}

// fergusonBrowserUA is the browser user agent the config used before the
// site moved to a search engine.
const fergusonBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// fergusonUserAgent returns the browser user agent the test fetches with.
func fergusonUserAgent(t *testing.T, _ map[string]any) string {
	t.Helper()
	return fergusonBrowserUA
}

// fergusonFixture returns the saved page and its expected product.
func fergusonFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "fergusonhome")
	html, err := os.ReadFile(filepath.Join(dir, "duravit-starck-3-undermount-sink.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "duravit-starck-3-undermount-sink.json"))
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

// fergusonReplace replaces every old, and fails if it is missing.
func fergusonReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// fergusonWarnings returns the warning codes of r.
func fergusonWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// fergusonSpec finds one spec pair by group and name.
func fergusonSpec(specs []any, group, name string) (string, bool) {
	for _, s := range specs {
		m, _ := s.(map[string]any)
		if m["group"] == group && m["name"] == name {
			v, _ := m["value"].(string)
			return v, true
		}
	}
	return "", false
}

// TestFergusonHome runs the real fergusonhome.com config against its saved
// page and variants of it, through the full HTTP service. The page is a
// server-rendered React storefront: price, stock text and the four
// specification tables are in the HTML, while the gallery and the variant
// widgets are rehydrated by JavaScript. Prices are US dollars and the saved
// product is on sale, so it carries both a current and a regular price.
func TestFergusonHome(t *testing.T) {
	page, want := fergusonFixture(t)
	cfg := fergusonSite(t)
	userAgent, _ := cfg["fetch"].(map[string]any)["user_agent"].(string)
	if userAgent == "" {
		t.Fatal("the fergusonhome fetch has no user agent")
	}
	e := Start(t, Options{ExtraSites: map[string]map[string]any{hostFergusonHome: cfg}})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostFergusonHome, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(fergusonPath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(fergusonPath), "request_id": "ferguson-1"})
		r.Success(t)
		if r.Str("site_id") != "fergusonhome" || r.Str("request_id") != "ferguson-1" || r.Str("config_version") != "2026-11-13.1" {
			t.Errorf("site_id/request_id/config_version = %q/%q/%q",
				r.Str("site_id"), r.Str("request_id"), r.Str("config_version"))
		}
		for _, f := range []string{"title", "url", "image_url", "description"} {
			exp, _ := want[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("%s = %.80q, want %.80q", f, got, exp)
			}
		}
		wantPrice, _ := want["price"].(map[string]any)
		for _, f := range []string{"raw", "state", "currency"} {
			exp, _ := wantPrice[f].(string)
			if got := r.Str("product.price." + f); got != exp {
				t.Errorf("price.%s = %q, want %q", f, got, exp)
			}
		}
		if exp, ok := wantPrice["current"].(float64); ok && r.Int("product.price.current") != int64(exp) {
			t.Errorf("price.current = %d, want %d", r.Int("product.price.current"), int64(exp))
		}
		if exp, ok := wantPrice["regular"].(float64); ok && r.Int("product.price.regular") != int64(exp) {
			t.Errorf("price.regular = %d, want %d", r.Int("product.price.regular"), int64(exp))
		}
		gotSpecs, _ := r.At("product.specs").([]any)
		wantSpecs, _ := want["specs"].([]any)
		if !reflect.DeepEqual(gotSpecs, wantSpecs) {
			t.Errorf("product.specs has %d pairs, want %d with the same values", len(gotSpecs), len(wantSpecs))
		}
		if got := fergusonWarnings(r); len(got) != 0 {
			t.Errorf("warnings = %v, want none", got)
		}
	})

	// With a browser user agent, the http engine sends the headers that
	// browser sends.
	t.Run("the request looks like the browser it claims to be", func(t *testing.T) {
		r := e.Extract(productURL(fergusonPath))
		r.Success(t)
		h := s.RequestHeaders(fergusonPath)
		for name, want := range map[string]string{
			"User-Agent":                userAgent,
			"Sec-CH-UA-Mobile":          "?0",
			"Sec-CH-UA-Platform":        `"Windows"`,
			"Sec-Fetch-Dest":            "document",
			"Sec-Fetch-Mode":            "navigate",
			"Sec-Fetch-Site":            "none",
			"Sec-Fetch-User":            "?1",
			"Upgrade-Insecure-Requests": "1",
		} {
			if got := h.Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
		if got := h.Get("Sec-CH-UA"); !strings.Contains(got, `"Google Chrome"`) || !strings.Contains(got, `"Chromium"`) {
			t.Errorf("Sec-CH-UA = %q", got)
		}
		if got := h.Get("Accept"); !strings.Contains(got, "text/html") {
			t.Errorf("Accept = %q", got)
		}
		if got := h.Get("Accept-Language"); !strings.HasPrefix(got, "en-US") {
			t.Errorf("Accept-Language = %q, want en-US first for a US store", got)
		}
	})

	t.Run("sample product details", func(t *testing.T) {
		r := e.Extract(productURL(fergusonPath))
		r.Success(t)
		if got := r.Str("product.title"); got != "Duravit Starck 3 20-7/8\" Rectangular Ceramic Undermount Bathroom Sink with Overflow and" {
			t.Errorf("title = %q", got)
		}
		// $140.99 on sale from $164.22, in cents.
		if cur := r.Int("product.price.current"); cur != 14099 {
			t.Errorf("price.current = %d, want 14099", cur)
		}
		if reg := r.Int("product.price.regular"); reg != 16422 {
			t.Errorf("price.regular = %d, want 16422", reg)
		}
		if got := r.Str("product.price.state"); got != "available" {
			t.Errorf("price.state = %q, want available", got)
		}
		if got := r.Str("product.price.currency"); got != "USD" {
			t.Errorf("price.currency = %q, want USD", got)
		}
		if !strings.Contains(r.Str("product.image_url"), "/product/duravit/duravit-0305490017-3993952.jpg") {
			t.Errorf("image_url = %s, want the CDN product image", r.Str("product.image_url"))
		}
		if !strings.HasPrefix(r.Str("product.description"), "Starck 3, designed by Philippe Starck") {
			t.Errorf("description starts %.60q", r.Str("product.description"))
		}
	})

	t.Run("canonical URL wins over the variant query", func(t *testing.T) {
		// ?uid= picks a finish; the canonical link drops it.
		r := e.Extract(productURL(fergusonPath) + "?uid=3938155")
		r.Success(t)
		if got := r.Str("product.url"); got != "https://www.fergusonhome.com"+fergusonPath {
			t.Errorf("product.url = %s, want the canonical URL without the query", got)
		}
	})

	t.Run("trailing slash is accepted", func(t *testing.T) {
		path := fergusonPath + "/"
		s.Handle(path, page)
		r := e.Extract(productURL(path))
		r.Success(t)
		if got := r.Str("product.title"); got != "Duravit Starck 3 20-7/8\" Rectangular Ceramic Undermount Bathroom Sink with Overflow and" {
			t.Errorf("title = %q", got)
		}
	})

	t.Run("specs come from the wide tables, once each", func(t *testing.T) {
		r := e.Extract(productURL(fergusonPath))
		r.Success(t)
		specs, _ := r.At("product.specs").([]any)
		// The page carries every table twice (wide and narrow screens); only
		// the wide copy is read, so 27 pairs instead of 54.
		if len(specs) != 27 {
			t.Errorf("product.specs has %d pairs, want 27", len(specs))
		}
		for _, c := range []struct{ group, name, value string }{
			{"Dimensions and Measurements", "Length", "20.875 in."},
			{"Included Components", "Drain Assembly Included", "No"},
			{"Characteristics and Features", "Material", "Ceramic"},
			{"Warranty and Product Information", "Manufacturer Warranty", "5 Years"},
		} {
			got, ok := fergusonSpec(specs, c.group, c.name)
			if !ok {
				t.Errorf("spec %q of group %q is missing", c.name, c.group)
				continue
			}
			if got != c.value {
				t.Errorf("spec %q = %q, want %q", c.name, got, c.value)
			}
		}
	})

	t.Run("specs are optional", func(t *testing.T) {
		path := "/duravit-030549-0hole/s1234567"
		s.Handle(path, fergusonReplace(t, page, `<div class="page-avoid-p">`, `<div class="page-avoid-x">`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.specs") {
			t.Errorf("product.specs = %v, want absent", r.At("product.specs"))
		}
		if got := fergusonWarnings(r); len(got) != 0 {
			t.Errorf("warnings = %v, want none", got)
		}
		if cur := r.Int("product.price.current"); cur != 14099 {
			t.Errorf("price.current = %d, want 14099", cur)
		}
	})

	t.Run("a product without a sale has no regular price", func(t *testing.T) {
		path := "/duravit-030549-0hole/s1234568"
		s.Handle(path, fergusonReplace(t, page, `Was <span class="strike">$164.22</span>`, ``))
		r := e.Extract(productURL(path))
		r.Success(t)
		if !r.Has("product.price.regular") || r.At("product.price.regular") != nil {
			t.Errorf("price.regular must be present and null, got %v", r.At("product.price.regular"))
		}
		if cur := r.Int("product.price.current"); cur != 14099 {
			t.Errorf("price.current = %d, want 14099", cur)
		}
		if got := r.Str("product.price.state"); got != "available" {
			t.Errorf("price.state = %q, want available", got)
		}
	})

	t.Run("a disabled Add to Cart is not an out-of-stock signal", func(t *testing.T) {
		// The saved page says "92 In Stock" yet renders the button disabled
		// until a variation is picked, which is why availability is not
		// configured for this site.
		if !strings.Contains(page, `disabled="" data-automation="add-to-cart-button"`) {
			t.Fatal("the saved page no longer renders a disabled Add to Cart button")
		}
		r := e.Extract(productURL(fergusonPath))
		r.Success(t)
		if got := r.Str("product.price.state"); got != "available" {
			t.Errorf("price.state = %q, want available", got)
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/duravit-030549-0hole/s1234569"
		s.Handle(path, fergusonReplace(t, page, `id="pdp-buysection"`, `id="pdp-buyarea"`))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "fergusonhome" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("category page is not fetched", func(t *testing.T) {
		path := "/duravit-sale/c82044866"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("collection page is not fetched", func(t *testing.T) {
		path := "/collection/Duravit/Starck 3"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/duravit-030549-0hole/s9999999"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
