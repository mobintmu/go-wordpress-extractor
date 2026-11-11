package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// hostSherwinWilliams stands in for www.sherwin-williams.com in the fake shop.
const hostSherwinWilliams = "sherwin-williams.test"

// sherwinWilliamsPath is the product path of the saved page. The colour is
// a query string, which match ignores.
const (
	sherwinWilliamsPath  = "/homeowners/products/emerald-interior-acrylic-latex-paint"
	sherwinWilliamsQuery = "?colorPartNumber=SW6617&colorType=PALETTE"
)

// sherwinWilliamsSite loads the real sherwin-williams.com config and points
// it at the fake shop. The real domains stay, so the page's canonical link
// (https://www.sherwin-williams.com/...) is honoured exactly as in production.
func sherwinWilliamsSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "sherwin-williams.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostSherwinWilliams)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// sherwinWilliamsFixture returns the saved page and its expected product.
func sherwinWilliamsFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "sherwin-williams")
	html, err := os.ReadFile(filepath.Join(dir, "emerald-interior-js-price.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "emerald-interior-js-price.json"))
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

// sherwinWilliamsReplace replaces every old, and fails if it is missing.
func sherwinWilliamsReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// sherwinWilliamsWarnings returns the warning codes of r.
func sherwinWilliamsWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// TestSherwinWilliams runs the real sherwin-williams.com config against its
// saved page and variants of it, through the full HTTP service. The site
// loads the main product's price and stock with JavaScript, so the server
// HTML has no price: every product returns state "unknown".
func TestSherwinWilliams(t *testing.T) {
	page, want := sherwinWilliamsFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostSherwinWilliams: sherwinWilliamsSite(t)},
		// The saved page is about 770 KB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostSherwinWilliams, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(sherwinWilliamsPath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{
			"url": productURL(sherwinWilliamsPath) + sherwinWilliamsQuery, "request_id": "sw-1"})
		r.Success(t)
		if r.Str("site_id") != "sherwin-williams" || r.Str("request_id") != "sw-1" {
			t.Errorf("site_id/request_id = %q/%q", r.Str("site_id"), r.Str("request_id"))
		}
		for _, f := range []string{"title", "url", "description"} {
			if got, exp := r.Str("product."+f), want[f].(string); got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
		}
		// og:image is protocol-relative (//sherwin.scene7.com/...), so it
		// takes the scheme of the fetched page: https in production, http
		// from the fake shop.
		if got, exp := r.Str("product.image_url"), "http:"+strings.TrimPrefix(want["image_url"].(string), "https:"); got != exp {
			t.Errorf("product.image_url = %q, want %q", got, exp)
		}
		price := want["price"].(map[string]any)
		for _, f := range []string{"currency", "raw", "state"} {
			if got, exp := r.Str("product.price."+f), price[f].(string); got != exp {
				t.Errorf("product.price.%s = %q, want %q", f, got, exp)
			}
		}
		if got := sherwinWilliamsWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want only PRICE_MISSING", got)
		}
	})

	t.Run("sample product: no price in the server HTML", func(t *testing.T) {
		r := e.Extract(productURL(sherwinWilliamsPath) + sherwinWilliamsQuery)
		r.Success(t)
		if r.Str("product.title") != "Emerald Interior Acrylic Latex Paint" {
			t.Errorf("title = %q", r.Str("product.title"))
		}
		if !strings.HasPrefix(r.Str("product.description"), "This acrylic latex paint offers exceptional washability") {
			t.Errorf("description starts %.60q", r.Str("product.description"))
		}
		if img := r.Str("product.image_url"); !strings.HasPrefix(img, "http://sherwin.scene7.com/is/image/sw/") ||
			!strings.Contains(img, "Emerald-Interior-Acrylic-Latex-Paint") {
			t.Errorf("image_url = %s, want the absolute scene7 Emerald image", img)
		}
		// The page lists recommended products at $18.59 - $24.89 and more;
		// none of those may become this product's price.
		for _, f := range []string{"current", "regular"} {
			if !r.Has("product.price."+f) || r.At("product.price."+f) != nil {
				t.Errorf("price.%s must be present and null, got %v", f, r.At("product.price."+f))
			}
		}
		if r.Str("product.price.state") != "unknown" || r.Str("product.price.currency") != "USD" {
			t.Errorf("price = %s %s, want unknown USD", r.Str("product.price.state"), r.Str("product.price.currency"))
		}
	})

	t.Run("canonical on www.sherwin-williams.com wins over the fetched URL", func(t *testing.T) {
		r := e.Extract(productURL(sherwinWilliamsPath) + sherwinWilliamsQuery)
		r.Success(t)
		if got := r.Str("product.url"); got != "https://www.sherwin-williams.com"+sherwinWilliamsPath {
			t.Errorf("product.url = %s, want the canonical URL without the colour query", got)
		}
	})

	t.Run("missing og:image is a warning, not a failure", func(t *testing.T) {
		path := "/homeowners/products/sw-no-image"
		s.Handle(path, sherwinWilliamsReplace(t, page, `<meta property="og:image"`, `<meta property="og:removed"`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.image_url") && r.At("product.image_url") != nil {
			t.Errorf("image_url = %v, want none", r.At("product.image_url"))
		}
		got := strings.Join(sherwinWilliamsWarnings(r), ",")
		if !strings.Contains(got, "FIELD_MISSING") || !strings.Contains(got, "PRICE_MISSING") {
			t.Errorf("warnings = %s, want FIELD_MISSING and PRICE_MISSING", got)
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/homeowners/products/sw-changed"
		s.Handle(path, sherwinWilliamsReplace(t, page, `class = "pdp__title"`, `class = "pdp__name"`))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "sherwin-williams" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("product family page is not fetched", func(t *testing.T) {
		path := "/homeowners/products/families/emerald_family"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/homeowners/products/sw-gone"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
