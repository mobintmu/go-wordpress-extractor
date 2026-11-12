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

// hostFloorAndDecor stands in for www.flooranddecor.com in the fake shop.
const hostFloorAndDecor = "flooranddecor.test"

// flooranddecorPath is the product path of the saved page.
const flooranddecorPath = "/ceramic-tile/zellige-oat-high-gloss-ceramic-tile-101130060.html"

// flooranddecorPrice is the main product's price element in the saved page.
const flooranddecorPrice = `<span class="b-pdp_price-cost"><span data-js-pdp-refinement-price>$1.49</span>`

// flooranddecorButton is the product form's Add to Cart button class. The
// smart-nav bar later in the page repeats it, so only the first one (inside
// the product form) is replaced.
const flooranddecorButton = `class="b-pdp_btns-button m-add-tocart"`

// flooranddecorSite loads the real flooranddecor.com config and points it
// at the fake shop. The real domains stay, so the page's canonical link
// (https://www.flooranddecor.com/...) is honoured exactly as in production.
func flooranddecorSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "flooranddecor.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostFloorAndDecor)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// flooranddecorFixture returns the saved page and its expected product.
func flooranddecorFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "flooranddecor")
	html, err := os.ReadFile(filepath.Join(dir, "zellige-oat-available.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "zellige-oat-available.json"))
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

// flooranddecorReplace replaces the first old, and fails if it is missing.
func flooranddecorReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.Replace(html, old, new, 1)
}

// flooranddecorWithPrice replaces the main product's price element with
// inner, which must open a <span class="b-pdp_price-cost"> like the page.
func flooranddecorWithPrice(t *testing.T, html, inner string) string {
	t.Helper()
	if n := strings.Count(html, flooranddecorPrice); n != 1 {
		t.Fatalf("price element found %d times in the saved page, want 1", n)
	}
	return strings.Replace(html, flooranddecorPrice, inner, 1)
}

// flooranddecorWithoutButton renames the product form's Add to Cart button
// so the order button selector no longer matches.
func flooranddecorWithoutButton(t *testing.T, html string) string {
	t.Helper()
	return flooranddecorReplace(t, html, flooranddecorButton, `class="b-pdp_btns-button m-notify"`)
}

// TestFloorAndDecor runs the real flooranddecor.com config against its
// saved page and variants of it, through the full HTTP service.
func TestFloorAndDecor(t *testing.T) {
	page, want := flooranddecorFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostFloorAndDecor: flooranddecorSite(t)},
		// The saved page is about 1.2 MB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostFloorAndDecor, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(flooranddecorPath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(flooranddecorPath), "request_id": "fnd-1"})
		r.Success(t)
		if r.Str("site_id") != "flooranddecor" || r.Str("request_id") != "fnd-1" {
			t.Errorf("site_id/request_id = %q/%q", r.Str("site_id"), r.Str("request_id"))
		}
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
		if got, exp := r.At("product.specs"), want["specs"]; !reflect.DeepEqual(got, exp) {
			t.Errorf("product.specs differs from the fixture: got %d rows, want %d", len(tilebarList(got)), len(tilebarList(exp)))
		}
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %d", n)
		}
	})

	t.Run("sample product: dollars returned in cents", func(t *testing.T) {
		r := e.Extract(productURL(flooranddecorPath))
		r.Success(t)
		if r.Str("product.title") != "Zellige Oat High Gloss Ceramic Tile" {
			t.Errorf("title = %q", r.Str("product.title"))
		}
		// $1.49 / piece
		if r.Int("product.price.current") != 149 || r.Str("product.price.currency") != "USD" {
			t.Errorf("price = %d %s, want 149 USD", r.Int("product.price.current"), r.Str("product.price.currency"))
		}
		if r.Str("product.price.raw") != "$1.49 / piece" {
			t.Errorf("raw = %q", r.Str("product.price.raw"))
		}
		if !r.Has("product.price.regular") || r.At("product.price.regular") != nil {
			t.Errorf("regular must be present and null, got %v", r.At("product.price.regular"))
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
		if !strings.Contains(r.Str("product.description"), "Pressed edges deliver a crisp finished line") {
			t.Error("description must include the bullet list items")
		}
	})

	t.Run("canonical on www.flooranddecor.com wins over the fetched host", func(t *testing.T) {
		r := e.Extract(productURL(flooranddecorPath))
		r.Success(t)
		if got := r.Str("product.url"); !strings.HasPrefix(got, "https://www.flooranddecor.com/ceramic-tile/") {
			t.Errorf("product.url = %s, want the canonical www.flooranddecor.com URL", got)
		}
	})

	t.Run("specs: grouped pairs, tooltips left out of names", func(t *testing.T) {
		r := e.Extract(productURL(flooranddecorPath))
		r.Success(t)
		list := tilebarList(r.At("product.specs"))
		specs := map[string]map[string]any{}
		for _, s := range list {
			m := s.(map[string]any)
			specs[m["name"].(string)] = m
			if strings.Contains(m["name"].(string), ".") {
				t.Errorf("spec name %q still holds tooltip text", m["name"])
			}
		}
		for name, exp := range map[string][2]string{
			"Size":                 {"Dimensions", "2 1/2 x 8"},
			"Box Weight":           {"Dimensions", "14.53 lbs"},
			"Look":                 {"Details", "Handmade Look"},
			"Country of Origin":    {"Details", "Spain"},
			"Installation Options": {"Installation & Warranty", "Wall Only"},
		} {
			got := specs[name]
			if got == nil || got["group"] != exp[0] || got["value"] != exp[1] {
				t.Errorf("spec %q = %v, want group %q value %q", name, got, exp[0], exp[1])
			}
		}
		if n := len(list); n != 27 {
			t.Errorf("got %d specs, want 27", n)
		}
	})

	t.Run("specs are optional: a page without them still succeeds", func(t *testing.T) {
		path := "/ceramic-tile/no-specs-tile-101130061.html"
		s.Handle(path, flooranddecorReplace(t, page, `id="specifications-section"`, `id="specifications-removed"`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.specs") {
			t.Errorf("specs must be omitted, got %d rows", len(tilebarList(r.At("product.specs"))))
		}
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %d", n)
		}
		if r.Int("product.price.current") != 149 {
			t.Errorf("price = %d", r.Int("product.price.current"))
		}
	})

	t.Run("sale price with a crossed-out regular price", func(t *testing.T) {
		path := "/ceramic-tile/sale-tile-101130062.html"
		s.Handle(path, flooranddecorWithPrice(t, page,
			`<del>$1,249.99</del><span class="b-pdp_price-cost"><span data-js-pdp-refinement-price>$1,099.50</span>`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Int("product.price.current") != 109950 || r.Int("product.price.regular") != 124999 {
			t.Errorf("prices = %d/%d, want 109950/124999", r.Int("product.price.current"), r.Int("product.price.regular"))
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
	})

	t.Run("out of stock keeps the shown price", func(t *testing.T) {
		path := "/ceramic-tile/sold-out-tile-101130063.html"
		s.Handle(path, flooranddecorWithoutButton(t, page))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" || r.Int("product.price.current") != 149 {
			t.Errorf("price = %d %s, want 149 out_of_stock", r.Int("product.price.current"), r.Str("product.price.state"))
		}
	})

	t.Run("call for price", func(t *testing.T) {
		path := "/ceramic-tile/call-tile-101130064.html"
		s.Handle(path, flooranddecorWithPrice(t, flooranddecorWithoutButton(t, page),
			`<span class="b-pdp_price-cost"><span data-js-pdp-refinement-price>Call for price</span>`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "call_for_price" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
		if !r.Has("product.price.current") || r.At("product.price.current") != nil {
			t.Errorf("current must be null, never 0, got %v", r.At("product.price.current"))
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/ceramic-tile/changed-tile-101130065.html"
		s.Handle(path, flooranddecorReplace(t, page, `<section class="l-pdp-product">`, `<section class="l-pdp-renamed">`))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "flooranddecor" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("category page is not fetched", func(t *testing.T) {
		path := "/ceramic-tile"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/ceramic-tile/gone-tile-100000000.html"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
