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

// hostInterface stands in for shop.interface.com in the fake shop.
const hostInterface = "interface.test"

// interfacePath is the product path of the saved page, decoded. The
// requested A043R.html canonicalises to itself; the full-SKU variant
// (A04302144020103XX0.html) has the same shape.
const interfacePath = "/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043R.html"

// interfaceSite loads the real shop.interface.com config and points it at
// the fake shop. The real domains stay, so the page's canonical link
// (https://shop.interface.com/...) is honoured exactly as in production.
func interfaceSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "shop.interface.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostInterface)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// interfaceFixture returns the saved live page and its expected product.
func interfaceFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "interface")
	html, err := os.ReadFile(filepath.Join(dir, "cut-and-form-available.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "cut-and-form-available.json"))
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

// interfaceReplace replaces every old with new, and fails if old is missing.
func interfaceReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// interfaceWarnings returns the warning codes of r.
func interfaceWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// interfaceSpec returns the first spec with name, or fails.
func interfaceSpec(t *testing.T, specs []any, name string) map[string]any {
	t.Helper()
	for _, s := range specs {
		m, _ := s.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	t.Fatalf("spec %q not found", name)
	return nil
}

// TestInterface runs the real shop.interface.com config against its saved
// page and variants of it, through the full HTTP service. The page loads the
// main product's price with JavaScript (the server HTML only has a hidden
// "N/A" block), so price is not configured and every product returns state
// "unknown" with a PRICE_MISSING warning.
func TestInterface(t *testing.T) {
	page, want := interfaceFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostInterface: interfaceSite(t)},
		// The saved page is about 1 MB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 4 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostInterface, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(interfacePath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(interfacePath), "request_id": "interface-1"})
		r.Success(t)
		if r.Str("site_id") != "interface" || r.Str("request_id") != "interface-1" || r.Str("config_version") != "2026-11-12.1" {
			t.Errorf("site_id/request_id/config_version = %q/%q/%q",
				r.Str("site_id"), r.Str("request_id"), r.Str("config_version"))
		}
		for _, f := range []string{"title", "url", "image_url", "description"} {
			exp, _ := want[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("product.%s = %.80q, want %.80q", f, got, exp)
			}
		}
		if got := r.At("product.price"); !reflect.DeepEqual(got, want["price"]) {
			t.Errorf("product.price = %v, want %v", got, want["price"])
		}
		if got := r.At("product.specs"); !reflect.DeepEqual(got, want["specs"]) {
			t.Errorf("product.specs does not match the fixture")
		}
		if got := interfaceWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want only PRICE_MISSING", got)
		}
	})

	t.Run("sample product details", func(t *testing.T) {
		r := e.Extract(productURL(interfacePath))
		r.Success(t)
		if got := r.Str("product.title"); got != "Cut & Form 4.5 mm" {
			t.Errorf("title = %q", got)
		}
		if !strings.HasPrefix(r.Str("product.description"), "Cut & Form™ blends the timeless parquet look") {
			t.Errorf("description starts %.60q", r.Str("product.description"))
		}
		if img := r.Str("product.image_url"); !strings.HasPrefix(img, "https://s7d1.scene7.com/is/image/interfaceprd/CutAndForm_") {
			t.Errorf("image_url = %s, want an absolute Scene7 product image", img)
		}
		// The price block is hidden and holds "N/A"; no price may be read.
		for _, f := range []string{"current", "regular"} {
			if !r.Has("product.price."+f) || r.At("product.price."+f) != nil {
				t.Errorf("price.%s must be present and null, got %v", f, r.At("product.price."+f))
			}
		}
		if r.Str("product.price.state") != "unknown" || r.Str("product.price.currency") != "USD" {
			t.Errorf("price = %s %s, want unknown USD", r.Str("product.price.state"), r.Str("product.price.currency"))
		}
	})

	t.Run("canonical on shop.interface.com wins over the fetched host", func(t *testing.T) {
		r := e.Extract(productURL(interfacePath))
		r.Success(t)
		if got := r.Str("product.url"); got != "https://shop.interface.com"+interfacePath {
			t.Errorf("product.url = %s, want the canonical shop.interface.com URL", got)
		}
	})

	t.Run("specifications come from the five accordions", func(t *testing.T) {
		r := e.Extract(productURL(interfacePath))
		r.Success(t)
		specs, _ := r.At("product.specs").([]any)
		if len(specs) != 39 {
			t.Fatalf("specs = %d, want 39", len(specs))
		}
		cases := []struct{ name, value, group string }{
			{"Product Number", "A043", "Product Specifications"},
			{"Wear Layer Thickness", "22 mil", "Product Specifications"},
			{"Slip Resistance", "(ASTM D2047) >0.55 wet/dry, ADA Compliant", "Performance Specifications"},
			{"Quantity per Carton", "26.91 ft² (2.5 m²)", "Packaging"},
			{"Installation Method", "Monolithic Ashlar Brick", "Technical Information"},
		}
		for _, c := range cases {
			got := interfaceSpec(t, specs, c.name)
			if got["value"] != c.value || got["group"] != c.group {
				t.Errorf("spec %q = %v in %v, want %q in %q", c.name, got["value"], got["group"], c.value, c.group)
			}
		}
		counts := map[string]int{}
		for _, s := range specs {
			m, _ := s.(map[string]any)
			g, _ := m["group"].(string)
			counts[g]++
		}
		wantCounts := map[string]int{
			"Product Specifications":                8,
			"Performance Specifications":            13,
			"Health + Environmental Specifications": 8,
			"Packaging":                             5,
			"Technical Information":                 5,
		}
		if !reflect.DeepEqual(counts, wantCounts) {
			t.Errorf("spec groups = %v, want %v", counts, wantCounts)
		}
	})

	t.Run("specifications are optional", func(t *testing.T) {
		path := "/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043N.html"
		s.Handle(path, interfaceReplace(t, page, `class="b-specification"`, `class="b-spec-list"`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.specs") {
			t.Errorf("product.specs = %v, want absent", r.At("product.specs"))
		}
		if got := r.Str("product.title"); got != "Cut & Form 4.5 mm" {
			t.Errorf("title = %q", got)
		}
		if got := interfaceWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want only PRICE_MISSING", got)
		}
	})

	t.Run("out of stock without the order button", func(t *testing.T) {
		path := "/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043B.html"
		html := interfaceReplace(t, page, "b-order_sample", "b-order_unavailable")
		html = interfaceReplace(t, html, `data-available="true"`, `data-available="false"`)
		html = interfaceReplace(t, html, `class="b-product_details-availability"`,
			`class="b-product_details-availability">`+
				`<section class="b-availability m-outofstock" aria-label="Availability"></section>`)
		s.Handle(path, html)
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" {
			t.Errorf("state = %s, want out_of_stock", r.Str("product.price.state"))
		}
		if !r.Has("product.price.current") || r.At("product.price.current") != nil {
			t.Errorf("current must be present and null, got %v", r.At("product.price.current"))
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043C.html"
		s.Handle(path, interfaceReplace(t, page, `data-tau="product_details"`, `data-tau="product_changed"`))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "interface" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("category page is not fetched", func(t *testing.T) {
		path := "/US/en-US/resilient/lvt/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/US/en-US/resilient/lvt/cut-and-form-4.5-mm/ZZZZ9.html"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
