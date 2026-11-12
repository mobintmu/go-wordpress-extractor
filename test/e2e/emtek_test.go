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

// hostEmtek stands in for www.emtek.com in the fake shop.
const hostEmtek = "emtek.test"

// emtekPath is the product path of the saved page. The live URL was fetched
// with ?443=371254&444=384300, two ids that only choose which finish and stem
// the showcase photo shows, so the path alone identifies the product.
const emtekPath = "/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/"

// emtekSite loads the real emtek.com config and points it at the fake shop.
// The real domains stay. The page has no canonical link, so product.url is
// whatever host was requested, which the test checks.
func emtekSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "emtek.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostEmtek)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// emtekFixture returns the saved page and its expected product.
func emtekFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "emtek")
	html, err := os.ReadFile(filepath.Join(dir, "select-lsquare-tubular-entry-set.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "select-lsquare-tubular-entry-set.json"))
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

// emtekReplace replaces every old, and fails if it is missing, so a variant can
// never silently test the unmodified page.
func emtekReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// emtekWarnings returns the warning codes of r, in response order.
func emtekWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// TestEmtek runs the real emtek.com config against its saved page and variants
// of it, through the full HTTP service. The page is a Next.js storefront whose
// class names come from CSS modules and carry a build hash, so the config
// matches class prefixes and the test checks that a deploy which only rehashes
// those names changes nothing. Emtek is a trade brand: the product page has no
// price, no cart and no stock text, only a "Find a Dealer" link, so the product
// comes back whole with price.state unknown and one PRICE_MISSING warning
// instead of a number nobody published.
func TestEmtek(t *testing.T) {
	page, want := emtekFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostEmtek: emtekSite(t)},
		// The saved page is about 550 KB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostEmtek, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(emtekPath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(emtekPath), "request_id": "emtek-1"})
		r.Success(t)
		if r.Str("site_id") != "emtek" || r.Str("request_id") != "emtek-1" || r.Str("config_version") != "2026-11-12.1" {
			t.Errorf("site_id/request_id/config_version = %q/%q/%q",
				r.Str("site_id"), r.Str("request_id"), r.Str("config_version"))
		}
		for _, f := range []string{"title", "image_url", "description"} {
			exp, _ := want[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("%s = %.120q, want %.120q", f, got, exp)
			}
		}
		wantPrice, _ := want["price"].(map[string]any)
		for _, f := range []string{"raw", "state", "currency"} {
			exp, _ := wantPrice[f].(string)
			if got := r.Str("product.price." + f); got != exp {
				t.Errorf("price.%s = %q, want %q", f, got, exp)
			}
		}
		if r.Has("product.price.current") && r.At("product.price.current") != nil {
			t.Errorf("price.current = %v, want null", r.At("product.price.current"))
		}
		if got := emtekWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want one PRICE_MISSING", got)
		}
	})

	t.Run("no canonical link, so the requested URL is returned", func(t *testing.T) {
		if strings.Contains(page, `rel="canonical"`) {
			t.Fatal("the saved page now has a canonical link; assert it instead")
		}
		r := e.Extract(productURL(emtekPath))
		r.Success(t)
		if got, exp := r.Str("product.url"), productURL(emtekPath); got != exp {
			t.Errorf("product.url = %q, want the requested URL %q", got, exp)
		}
	})

	t.Run("sample product details", func(t *testing.T) {
		r := e.Extract(productURL(emtekPath))
		r.Success(t)
		if got := r.Str("product.title"); got != "SELECT L-Square Stem, Smooth Grip Full Length Tubular Entry Set" {
			t.Errorf("title = %q", got)
		}
		// Emtek publishes no price at all: no number, no call-for-price words,
		// no stock text. The state must say so instead of inventing a price.
		if got := r.Str("product.price.state"); got != "unknown" {
			t.Errorf("price.state = %q, want unknown", got)
		}
		if r.At("product.price.current") != nil {
			t.Errorf("price.current = %v, want null", r.At("product.price.current"))
		}
		if got := r.Str("product.price.raw"); got != "" {
			t.Errorf("price.raw = %q, want empty", got)
		}
	})

	t.Run("the showcase image follows the chosen finish", func(t *testing.T) {
		r := e.Extract(productURL(emtekPath))
		r.Success(t)
		// The page's src is a Cloudinary URL that resizes the Salsify original
		// of the finish in the query string (US15 satin nickel here).
		if got := r.Str("product.image_url"); !strings.HasPrefix(got, "https://res.cloudinary.com/emtek/image/fetch/") ||
			!strings.Contains(got, "4914US15.LS15.SM15_EM_KO_3Q-EXT") {
			t.Errorf("image_url = %q", got)
		}
	})

	t.Run("rehashing the CSS module class names changes nothing", func(t *testing.T) {
		path := "/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/emtek-rehashed/"
		html := emtekReplace(t, page, "product-showcase_figure__H6Rwh", "product-showcase_figure__a1B2c")
		html = emtekReplace(t, html, "product-page_productShowcase__j8cyZ", "product-page_productShowcase__d3E4f")
		html = emtekReplace(t, html, "rich-text_richText__gCsHl", "rich-text_richText__g5H6i")
		s.Handle(path, html)
		r := e.Extract(productURL(path))
		r.Success(t)
		if got := r.Str("product.title"); !strings.HasPrefix(got, "SELECT L-Square Stem") {
			t.Errorf("title = %q", got)
		}
		if got := r.Str("product.description"); got == "" {
			t.Error("description is empty after a rehash")
		}
		if got := r.Str("product.image_url"); !strings.Contains(got, "res.cloudinary.com") {
			t.Errorf("image_url = %q", got)
		}
	})

	t.Run("a page without the rich-text block keeps the image", func(t *testing.T) {
		path := "/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/emtek-no-description/"
		s.Handle(path, emtekReplace(t, page, "rich-text_richText__gCsHl", "rich-text_rtext__gCsHl"))
		r := e.Extract(productURL(path))
		r.Success(t)
		if got := emtekWarnings(r); len(got) != 2 || got[0] != "FIELD_MISSING" || got[1] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want FIELD_MISSING and PRICE_MISSING", got)
		}
		if r.Str("product.title") == "" || r.Str("product.image_url") == "" {
			t.Errorf("title/image_url = %q/%q, want both", r.Str("product.title"), r.Str("product.image_url"))
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/emtek-changed/"
		s.Handle(path, emtekReplace(t, page, "productShowcase", "productHero"))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "emtek" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("shallow category page is not fetched", func(t *testing.T) {
		path := "/all-products/door-hardware/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Error("a non-product URL must not be fetched")
		}
	})

	// Emtek nests collections as deep as products (/all-products/accessories/
	// door-accessories/house-numbers/7qt-modern/ is a landing page), so match
	// cannot reject them by shape. They are fetched and fail cleanly instead,
	// which this subtest pins down.
	t.Run("a nested collection page is fetched and fails cleanly", func(t *testing.T) {
		path := "/all-products/accessories/door-accessories/door-bells/"
		s.Handle(path, emtekReplace(t, page, "productShowcase", "productHero"))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if s.Hits(path) != 1 {
			t.Errorf("hits = %d, want the page fetched once", s.Hits(path))
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/emtek-gone/"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
