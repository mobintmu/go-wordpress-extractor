package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// hostLLFlooring stands in for www.llflooring.com in the fake shop.
const hostLLFlooring = "llflooring.test"

// llflooringPath is the product path of the saved page.
const llflooringPath = "/p/coreluxe-5mm-w-pad-daydream-meadow-oak-6-in-wide-x-48-in-long-10055236/"

// llflooringSite loads the real llflooring.com config and points it at the
// fake shop. The real domains stay, so the page's canonical link
// (https://www.llflooring.com/p/...) is honoured exactly as in production.
func llflooringSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "llflooring.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostLLFlooring)
	f, _ := cfg["fetch"].(map[string]any)
	if f == nil {
		f = map[string]any{}
	}
	f["delay_ms"], f["timeout_seconds"], f["retries"], f["respect_robots_txt"] = 0, 5, 2, true
	cfg["fetch"] = f
	return cfg
}

// llflooringFixture returns the saved live page and its expected product.
func llflooringFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "llflooring")
	html, err := os.ReadFile(filepath.Join(dir, "daydream-meadow-oak-available.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "daydream-meadow-oak-available.json"))
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

// llflooringReplace replaces old once, and fails if it is missing.
func llflooringReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if n := strings.Count(html, old); n != 1 {
		t.Fatalf("%q found %d times in the saved page, want 1", old, n)
	}
	return strings.Replace(html, old, new, 1)
}

// llflooringBoxPrice is the hidden element that holds only the box price.
const llflooringBoxPrice = `<div aria-hidden="true" class="d-none" id="product-content">51.75</div>`

// llflooringWithoutButton renames the main Add to Cart button's class so the
// order button selector no longer matches.
func llflooringWithoutButton(t *testing.T, html string) string {
	t.Helper()
	return llflooringReplace(t, html, `class="add-to-cart btn`, `class="notify-me btn`)
}

func llflooringList(v any) []any {
	l, _ := v.([]any)
	return l
}

// TestLLFlooring runs the real llflooring.com config against its saved live
// page and variants of it, through the full HTTP service. The site uses the
// free http engine, so nothing is mocked.
func TestLLFlooring(t *testing.T) {
	page, want := llflooringFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostLLFlooring: llflooringSite(t)},
		// The saved page is about 950 KB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostLLFlooring, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(llflooringPath, page)

	t.Run("sample product with the http engine", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(llflooringPath), "request_id": "llflooring-1"})
		r.Success(t)
		if r.Str("site_id") != "llflooring" || r.Str("request_id") != "llflooring-1" {
			t.Errorf("site_id/request_id = %q/%q", r.Str("site_id"), r.Str("request_id"))
		}
		if r.Str("source.engine") != "http" {
			t.Errorf("source.engine = %q, want http", r.Str("source.engine"))
		}
	})

	t.Run("saved live page matches the fixture", func(t *testing.T) {
		r := e.Extract(productURL(llflooringPath))
		r.Success(t)
		for _, f := range []string{"title", "url", "image_url", "description"} {
			if got, exp := r.Str("product."+f), want[f].(string); got != exp {
				t.Errorf("product.%s = %q, want %q", f, got, exp)
			}
		}
		price := want["price"].(map[string]any)
		for _, f := range []string{"currency", "raw", "state"} {
			if got, exp := r.Str("product.price."+f), price[f].(string); got != exp {
				t.Errorf("product.price.%s = %q, want %q", f, got, exp)
			}
		}
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %d", n)
		}
	})

	t.Run("sample product: box price returned in cents", func(t *testing.T) {
		r := e.Extract(productURL(llflooringPath))
		r.Success(t)
		if r.Str("product.title") != "CoreLuxe 5mm w/pad Daydream Meadow Oak Waterproof Rigid Vinyl Plank Flooring 6 in. Wide x 48 in. Long" {
			t.Errorf("title = %q", r.Str("product.title"))
		}
		// $51.75 per box (the page also shows $2.19 / Sq Ft)
		if r.Int("product.price.current") != 5175 || r.Str("product.price.currency") != "USD" {
			t.Errorf("price = %d %s, want 5175 USD", r.Int("product.price.current"), r.Str("product.price.currency"))
		}
		if !r.Has("product.price.regular") || r.At("product.price.regular") != nil {
			t.Errorf("regular must be present and null, got %v", r.At("product.price.regular"))
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
		if !strings.HasPrefix(r.Str("product.image_url"), "https://media.llflooring.com/image/upload/") {
			t.Errorf("image_url = %s", r.Str("product.image_url"))
		}
	})

	t.Run("canonical on www.llflooring.com wins over the fetched host", func(t *testing.T) {
		r := e.Extract(productURL(llflooringPath))
		r.Success(t)
		if got := r.Str("product.url"); !strings.HasPrefix(got, "https://www.llflooring.com/p/") {
			t.Errorf("product.url = %s, want the canonical www.llflooring.com URL", got)
		}
	})

	t.Run("specs: grouped name/value pairs, tooltips left out", func(t *testing.T) {
		r := e.Extract(productURL(llflooringPath))
		r.Success(t)
		if got, exp := r.At("product.specs"), want["specs"]; !reflect.DeepEqual(got, exp) {
			t.Errorf("product.specs differs from the fixture: got %d rows, want %d", len(llflooringList(got)), len(llflooringList(exp)))
		}
		specs := map[string]map[string]any{}
		for _, s := range llflooringList(r.At("product.specs")) {
			m := s.(map[string]any)
			specs[m["name"].(string)] = m
		}
		for name, exp := range map[string][2]string{
			"Article Number":            {"Details", "RVP5DMO-A"},
			"Color":                     {"Details", "Gray"},
			"Installation Type":         {"Details", "Click"},
			"Width":                     {"Dimensions", "5.9 in."},
			"Light Commercial Warranty": {"Warranties & Certifications", "10 Year Warranty"},
		} {
			got := specs[name]
			if got == nil || got["group"] != exp[0] || got["value"] != exp[1] {
				t.Errorf("spec %q = %v, want group %q value %q", name, got, exp[0], exp[1])
			}
		}
		for name := range specs {
			if strings.Contains(name, "What is") {
				t.Errorf("spec name %q carries the tooltip text", name)
			}
		}
		if n := len(llflooringList(r.At("product.specs"))); n != 40 {
			t.Errorf("got %d specs, want 40", n)
		}
	})

	t.Run("specs are optional: a page without them still succeeds", func(t *testing.T) {
		path := "/p/llflooring-no-specs-10000001/"
		s.Handle(path, llflooringReplace(t, page, `<div class="product-specifications">`, `<div class="product-specs-removed">`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.specs") {
			t.Errorf("specs must be omitted, got %d rows", len(llflooringList(r.At("product.specs"))))
		}
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %d", n)
		}
		if r.Int("product.price.current") != 5175 {
			t.Errorf("price = %d", r.Int("product.price.current"))
		}
	})

	// No sale subtest: no sale page was found on the site, so price_regular
	// is not configured. No call-for-price subtest: the site shows no such
	// wording, so call_for_price_words is not configured.

	t.Run("price with cents and thousands separator", func(t *testing.T) {
		path := "/p/llflooring-big-box-10000002/"
		s.Handle(path, llflooringReplace(t, page, llflooringBoxPrice,
			`<div aria-hidden="true" class="d-none" id="product-content">1,378.50</div>`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Int("product.price.current") != 137850 {
			t.Errorf("price = %d, want 137850", r.Int("product.price.current"))
		}
	})

	t.Run("no Add to Cart button is out of stock, price kept", func(t *testing.T) {
		path := "/p/llflooring-out-of-stock-10000003/"
		s.Handle(path, llflooringWithoutButton(t, page))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" || r.Int("product.price.current") != 5175 {
			t.Errorf("price = %d %s, want 5175 out_of_stock", r.Int("product.price.current"), r.Str("product.price.state"))
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/p/llflooring-changed-10000004/"
		// Every page carries the sign-in modal's reCAPTCHA widget
		// (class="g-recaptcha"). When root is missing, the service checks
		// the page for challenge markers and would report BOT_PROTECTION, so
		// the widget is removed here to test the selector failure itself.
		html := llflooringReplace(t, page, `<div id="maincontent"`, `<div id="renamedcontent"`)
		html = llflooringReplace(t, html, `class="g-recaptcha"`, `class="login-widget"`)
		s.Handle(path, html)
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "llflooring" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("category page is not fetched", func(t *testing.T) {
		path := "/c/rigid-vinyl-plank/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/p/llflooring-gone-10000005/"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
