package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// hostTilebar stands in for www.tilebar.com in the fake shop.
const hostTilebar = "tilebar.test"

// tilebarPath is the product path of the saved page.
const tilebarPath = "/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html"

// tilebarSite loads the real tilebar.com config and points it at the fake
// shop. The real domains stay, so the page's canonical link
// (https://www.tilebar.com/...) is honoured exactly as in production. The
// real 30 second crawl delay is replaced so the test runs fast.
func tilebarSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "tilebar.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostTilebar)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// tilebarFixture returns the saved live page and its expected product.
func tilebarFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "tilebar")
	html, err := os.ReadFile(filepath.Join(dir, "avanti-checkerboard-available.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "avanti-checkerboard-available.json"))
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

// tilebarFinalPrice is the Magento final price element of the main product.
var tilebarFinalPrice = regexp.MustCompile(`<span\s+id="product-price-30159"[^>]*><span class="price">\$420\.00</span></span>`)

// tilebarWithPrice replaces the main product's final price element with inner.
func tilebarWithPrice(t *testing.T, html, inner string) string {
	t.Helper()
	if n := len(tilebarFinalPrice.FindAllStringIndex(html, -1)); n != 1 {
		t.Fatalf("final price element found %d times in the saved page, want 1", n)
	}
	return tilebarFinalPrice.ReplaceAllLiteralString(html, inner)
}

// tilebarWithoutButton renames the add-to-cart button's id so the order
// button selector no longer matches.
func tilebarWithoutButton(t *testing.T, html string) string {
	t.Helper()
	if !strings.Contains(html, `id="product-addtocart-button"`) {
		t.Fatal("add-to-cart button not found in the saved page")
	}
	return strings.ReplaceAll(html, `id="product-addtocart-button"`, `id="product-notify-button"`)
}

// tilebarReplace replaces old once, and fails if it is missing.
func tilebarReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.Replace(html, old, new, 1)
}

// tilebarList returns v as a JSON array, or nil.
func tilebarList(v any) []any {
	l, _ := v.([]any)
	return l
}

func tilebarAmount(typ, amount string) string {
	return `<span id="product-price-30159" data-price-type="` + typ + `" class="price-wrapper "><span class="price">` + amount + `</span></span>`
}

// TestTilebar runs the real tilebar.com config against its saved live page
// and variants of it, through the full HTTP service.
func TestTilebar(t *testing.T) {
	page, want := tilebarFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostTilebar: tilebarSite(t)},
		// The saved page is about 550 KB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostTilebar, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(tilebarPath, page)

	t.Run("saved live page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(tilebarPath), "request_id": "tilebar-1"})
		r.Success(t)
		if r.Str("site_id") != "tilebar" || r.Str("request_id") != "tilebar-1" {
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

	t.Run("specs: grouped name/value pairs, hidden rows skipped", func(t *testing.T) {
		r := e.Extract(productURL(tilebarPath))
		r.Success(t)
		specs := map[string]map[string]any{}
		for _, s := range tilebarList(r.At("product.specs")) {
			m := s.(map[string]any)
			specs[m["name"].(string)] = m
		}
		for name, exp := range map[string][2]string{
			"Colorway":       {"Key Specs", "Green, Beige"},
			"Item Size":      {"Key Specs", `12" x 12"`},
			"Outdoor Use":    {"Details", "Wall | Floor"},
			"Tile Thickness": {"Details", "9.5 mm"},
			"Sample Size":    {"Dimensions", `2 Pieces of 6" x 6"`},
		} {
			got := specs[name]
			if got == nil || got["group"] != exp[0] || got["value"] != exp[1] {
				t.Errorf("spec %q = %v, want group %q value %q", name, got, exp[0], exp[1])
			}
		}
		if _, ok := specs["SKU"]; ok {
			t.Error("the hidden SKU row must be skipped")
		}
		if n := len(specs); n != 23 {
			t.Errorf("got %d specs, want 23", n)
		}
	})

	t.Run("specs are optional: a page without them still succeeds", func(t *testing.T) {
		path := "/product/tilebar-no-specs.html"
		s.Handle(path, tilebarReplace(t, page, `<div class="description-spec"`, `<div class="description-removed"`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.specs") {
			t.Errorf("specs must be omitted, got %d rows", len(tilebarList(r.At("product.specs"))))
		}
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %d", n)
		}
		if r.Int("product.price.current") != 42000 {
			t.Errorf("price = %d", r.Int("product.price.current"))
		}
	})

	t.Run("sample product: dollars returned in cents", func(t *testing.T) {
		r := e.Extract(productURL(tilebarPath))
		r.Success(t)
		if r.Str("product.title") != "Avanti Verde Green and Crema Marfil Beige 12x12 Tumbled Marble Checkerboard Tile" {
			t.Errorf("title = %q", r.Str("product.title"))
		}
		// $420.00 per kit
		if r.Int("product.price.current") != 42000 || r.Str("product.price.currency") != "USD" {
			t.Errorf("price = %d %s, want 42000 USD", r.Int("product.price.current"), r.Str("product.price.currency"))
		}
		if !r.Has("product.price.regular") || r.At("product.price.regular") != nil {
			t.Errorf("regular must be present and null, got %v", r.At("product.price.regular"))
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
	})

	t.Run("canonical on www.tilebar.com wins over the fetched host", func(t *testing.T) {
		r := e.Extract(productURL(tilebarPath))
		r.Success(t)
		if got := r.Str("product.url"); !strings.HasPrefix(got, "https://www.tilebar.com/product/") {
			t.Errorf("product.url = %s, want the canonical www.tilebar.com URL", got)
		}
	})

	t.Run("sale price with cents", func(t *testing.T) {
		path := "/product/tilebar-sale.html"
		s.Handle(path, tilebarWithPrice(t, page,
			`<span class="special-price">`+tilebarAmount("finalPrice", "$1,378.50")+`</span>`+
				`<span class="old-price">`+tilebarAmount("oldPrice", "$1,420.00")+`</span>`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Int("product.price.current") != 137850 || r.Int("product.price.regular") != 142000 {
			t.Errorf("prices = %d/%d, want 137850/142000", r.Int("product.price.current"), r.Int("product.price.regular"))
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
	})

	t.Run("out of stock keeps the shown price", func(t *testing.T) {
		path := "/product/tilebar-out-of-stock.html"
		html := tilebarReplace(t, tilebarWithoutButton(t, page), `<div class="product-info-main">`,
			`<div class="product-info-main"><div class="stock unavailable"><span>Out of stock</span></div>`)
		s.Handle(path, html)
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" || r.Int("product.price.current") != 42000 {
			t.Errorf("price = %d %s, want 42000 out_of_stock", r.Int("product.price.current"), r.Str("product.price.state"))
		}
	})

	t.Run("call for price", func(t *testing.T) {
		path := "/product/tilebar-call.html"
		s.Handle(path, tilebarWithPrice(t, tilebarWithoutButton(t, page), tilebarAmount("finalPrice", "Call for price")))
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
		path := "/product/tilebar-changed.html"
		s.Handle(path, tilebarReplace(t, page, `<div class="column main">`, `<div class="column renamed">`))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "tilebar" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("category page is not fetched", func(t *testing.T) {
		path := "/bathroom.html"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/product/tilebar-gone.html"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
