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

// hostThreeForm stands in for www.3-form.com in the fake shop.
const hostThreeForm = "3-form.test"

// threeFormPath is the product path of the saved page.
const threeFormPath = "/products/edge-lit-chroma-countertop"

// threeFormSite loads the real 3-form.com config and points it at the fake
// shop. The real domains stay, so the page's canonical link
// (https://www.3-form.com/...) is honoured exactly as in production.
func threeFormSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "3-form.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostThreeForm)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// threeFormFixture returns the saved page and its expected product.
func threeFormFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "3form")
	html, err := os.ReadFile(filepath.Join(dir, "edge-lit-chroma-countertop.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "edge-lit-chroma-countertop.json"))
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

// threeFormReplace replaces every old, and fails if it is missing.
func threeFormReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// threeFormWarnings returns the warning codes of r.
func threeFormWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// TestThreeForm runs the real 3-form.com config against its saved page and
// variants of it, through the full HTTP service. 3form sells architectural
// products by quote: the page has no price and no stock signal, so price is
// not configured and every product returns state "unknown" with a
// PRICE_MISSING warning.
func TestThreeForm(t *testing.T) {
	page, want := threeFormFixture(t)
	e := Start(t, Options{ExtraSites: map[string]map[string]any{hostThreeForm: threeFormSite(t)}})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostThreeForm, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(threeFormPath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(threeFormPath), "request_id": "3form-1"})
		r.Success(t)
		if r.Str("site_id") != "3form" || r.Str("request_id") != "3form-1" || r.Str("config_version") != "2026-11-12.1" {
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
		if got := threeFormWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want only PRICE_MISSING", got)
		}
	})

	t.Run("sample product: quote pricing only", func(t *testing.T) {
		r := e.Extract(productURL(threeFormPath))
		r.Success(t)
		// The page has no h1 and no "Solutions | " branding prefix in the
		// product block.
		if got := r.Str("product.title"); got != "Edge-Lit Chroma Countertop" {
			t.Errorf("title = %q", got)
		}
		if !strings.HasPrefix(r.Str("product.description"), "Made with our light-loving Chroma material") {
			t.Errorf("description starts %.60q", r.Str("product.description"))
		}
		// The gallery is filled by JavaScript; og:image is the only
		// server-side image.
		if img := r.Str("product.image_url"); !strings.HasPrefix(img, "https://img.imageboss.me/mkt-images-prod/width/1200/") {
			t.Errorf("image_url = %s, want the og:image URL", img)
		}
		for _, f := range []string{"current", "regular"} {
			if !r.Has("product.price."+f) || r.At("product.price."+f) != nil {
				t.Errorf("price.%s must be present and null, got %v", f, r.At("product.price."+f))
			}
		}
		if r.Str("product.price.state") != "unknown" || r.Str("product.price.currency") != "USD" {
			t.Errorf("price = %s %s, want unknown USD", r.Str("product.price.state"), r.Str("product.price.currency"))
		}
		if r.Has("product.specs") {
			t.Errorf("product.specs = %v, want absent (no spec rows on this page)", r.At("product.specs"))
		}
	})

	t.Run("canonical URL wins over a query string", func(t *testing.T) {
		r := e.Extract(productURL(threeFormPath) + "?utm_source=newsletter")
		r.Success(t)
		if got := r.Str("product.url"); got != "https://www.3-form.com"+threeFormPath {
			t.Errorf("product.url = %s, want the canonical URL without the query", got)
		}
	})

	t.Run("trailing slash is accepted", func(t *testing.T) {
		path := threeFormPath + "/"
		s.Handle(path, page)
		r := e.Extract(productURL(path))
		r.Success(t)
		if got := r.Str("product.title"); got != "Edge-Lit Chroma Countertop" {
			t.Errorf("title = %q", got)
		}
	})

	t.Run("missing og:image is a warning, not a failure", func(t *testing.T) {
		path := "/products/3form-no-image"
		s.Handle(path, threeFormReplace(t, page, `<meta property="og:image"`, `<meta property="og:removed"`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Has("product.image_url") && r.At("product.image_url") != nil {
			t.Errorf("image_url = %v, want none", r.At("product.image_url"))
		}
		if got := threeFormWarnings(r); len(got) != 2 || got[0] != "FIELD_MISSING" || got[1] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want [FIELD_MISSING PRICE_MISSING]", got)
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/products/3form-renamed-block"
		s.Handle(path, threeFormReplace(t, page, "<walter-block", "<walter-panel"))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "3form" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("gallery page is not fetched", func(t *testing.T) {
		// Inspiration articles share the host but carry no product fields.
		path := "/gallery/1230"
		r := e.Extract(productURL(path) + "?filterOptions=3")
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("product index is not fetched", func(t *testing.T) {
		path := "/products/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/products/3form-gone"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
