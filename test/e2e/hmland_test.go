package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// hostHMLand stands in for hmland.ir in the fake shop.
const hostHMLand = "hmland.test"

// hmlandPath is the product path of the saved page, decoded.
const hmlandPath = "/product/کره-بادام-زمینی/"

// hmlandSite loads the real hmland.ir config and points it at the fake
// shop. The real domains stay, so the page's canonical link
// (https://hmland.ir/...) is honoured exactly as in production.
func hmlandSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "hmland.ir.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostHMLand)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// hmlandFixture returns the saved live page and its expected product.
func hmlandFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "hmland")
	html, err := os.ReadFile(filepath.Join(dir, "peanut-butter-variable.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "peanut-butter-variable.json"))
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

var priceBlock = regexp.MustCompile(`(?s)<p class="price">.*?</p>`)

// withPrice replaces the page's price block with inner.
func withPrice(t *testing.T, html, inner string) string {
	t.Helper()
	if !priceBlock.MatchString(html) {
		t.Fatal("price block not found in the saved page")
	}
	return priceBlock.ReplaceAllLiteralString(html, `<p class="price">`+inner+`</p>`)
}

// withoutButton removes the add-to-cart button's class so the order
// button selector no longer matches.
func withoutButton(t *testing.T, html string) string {
	t.Helper()
	if !strings.Contains(html, "single_add_to_cart_button") {
		t.Fatal("add-to-cart button not found in the saved page")
	}
	return strings.ReplaceAll(html, "single_add_to_cart_button", "wd-disabled-button")
}

func rialAmount(n string) string {
	return `<span class="woocommerce-Price-amount amount"><bdi><span class="woocommerce-Price-currencySymbol">ریال</span>&nbsp;` + n + `</bdi></span>`
}

// TestHMLand runs the real hmland.ir config against its saved live page
// and variants of it, through the full HTTP service.
func TestHMLand(t *testing.T) {
	page, want := hmlandFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostHMLand: hmlandSite(t)},
		// The saved page is about 370 KB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostHMLand, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(hmlandPath, page)

	t.Run("saved live page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(hmlandPath), "request_id": "hmland-1"})
		r.Success(t)
		if r.Str("site_id") != "hmland" || r.Str("request_id") != "hmland-1" {
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
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %s", r.Raw)
		}
	})

	t.Run("variable product: lowest variant price, rial to toman", func(t *testing.T) {
		r := e.Extract(productURL(hmlandPath))
		r.Success(t)
		if r.Str("product.title") != "کره بادام زمینی" {
			t.Errorf("title = %q", r.Str("product.title"))
		}
		// 5,470,000 – 9,140,000 rial
		if r.Int("product.price.current") != 547000 || r.Str("product.price.currency") != "IRT" {
			t.Errorf("price = %d %s, want 547000 IRT", r.Int("product.price.current"), r.Str("product.price.currency"))
		}
		if !r.Has("product.price.regular") || r.At("product.price.regular") != nil {
			t.Errorf("regular must be present and null: %s", r.Raw)
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
	})

	t.Run("canonical on hmland.ir wins over the fetched host", func(t *testing.T) {
		r := e.Extract(productURL(hmlandPath))
		r.Success(t)
		if got := r.Str("product.url"); !strings.HasPrefix(got, "https://hmland.ir/product/") {
			t.Errorf("product.url = %s, want the canonical hmland.ir URL", got)
		}
		if !strings.HasPrefix(r.Str("product.image_url"), "https://hmland.ir/wp-content/uploads/") {
			t.Errorf("image_url = %s", r.Str("product.image_url"))
		}
	})

	t.Run("sale price in rial", func(t *testing.T) {
		path := "/product/hmland-sale/"
		s.Handle(path, withPrice(t, page, `<del aria-hidden="true">`+rialAmount("6,000,000")+`</del> <ins>`+rialAmount("5,470,000")+`</ins>`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Int("product.price.current") != 547000 || r.Int("product.price.regular") != 600000 {
			t.Errorf("prices = %d/%d, want 547000/600000", r.Int("product.price.current"), r.Int("product.price.regular"))
		}
		if r.Str("product.price.state") != "available" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
	})

	t.Run("out of stock keeps the shown price", func(t *testing.T) {
		path := "/product/hmland-out-of-stock/"
		html := strings.Replace(withoutButton(t, page), `<p class="price">`, `<p class="stock out-of-stock">ناموجود</p><p class="price">`, 1)
		s.Handle(path, html)
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" || r.Int("product.price.current") != 547000 {
			t.Errorf("price = %d %s, want 547000 out_of_stock", r.Int("product.price.current"), r.Str("product.price.state"))
		}
	})

	t.Run("call for price", func(t *testing.T) {
		path := "/product/hmland-call/"
		s.Handle(path, withPrice(t, withoutButton(t, page), `تماس بگیرید`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if r.Str("product.price.state") != "call_for_price" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
		if !r.Has("product.price.current") || r.At("product.price.current") != nil {
			t.Errorf("current must be null, never 0: %s", r.Raw)
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/product/hmland-changed/"
		s.Handle(path, strings.ReplaceAll(page, "type-product", "type-renamed"))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "hmland" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("category page is not fetched", func(t *testing.T) {
		path := "/product-category/nuts/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/product/hmland-gone/"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s: %s", r.Str("error.code"), r.Raw)
		}
	})
}
