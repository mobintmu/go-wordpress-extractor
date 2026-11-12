package e2e

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"go-wordpress/internal/fetch"

	"go.uber.org/zap"
)

const hostRender = "render.test"

// renderedShell serves a page whose product markup exists only after
// JavaScript runs: the HTML holds the product page as a JSON string and a
// script swaps it in. The HTTP engine sees no product root; the browser
// engine sees the full product.
func renderedShell(t *testing.T) string {
	t.Helper()
	html, err := json.Marshal(productPage(page{title: "Rendered almonds", price: `<span class="amount">۴۰۰,۰۰۰ تومان</span>`}))
	if err != nil {
		t.Fatal(err)
	}
	return `<!doctype html><html><head><title>Loading</title></head><body><div id="root"></div><script>
var html = ` + string(html) + `;
setTimeout(function () { document.open(); document.write(html); document.close(); }, 200);
</script></body></html>`
}

func renderSite(engine string) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"site_id":        "render",
		"version":        "2026-11-12.1",
		"extends":        "woocommerce",
		"domains":        []string{hostRender},
		"fetch":          map[string]any{"engine": engine, "delay_ms": 0, "timeout_seconds": 15, "retries": 0, "respect_robots_txt": false},
		"pages": map[string]any{"product": map[string]any{
			"root": "div.main-page-wrapper div.site-content div.product.type-product",
		}},
	}
}

// requireBrowser skips when Playwright's driver or the browser is missing.
func requireBrowser(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("browser tests are skipped in -short mode")
	}
	bopts := fetch.DefaultBrowserOptions()
	bopts.Channel = os.Getenv("APP_BROWSER_CHANNEL")
	b := fetch.NewBrowserFetcher(fetch.DefaultOptions(), bopts, nil, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Skipf("browser engine unavailable: %v", err)
	}
	_ = b.Stop(ctx)
}

// The site config's fetch.engine selects the strategy: "browser" renders
// the page in Chrome, so a JavaScript-only product page is extracted.
func TestBrowserEngineExtractsJavaScriptPage(t *testing.T) {
	requireBrowser(t)
	channel := os.Getenv("APP_BROWSER_CHANNEL")
	if channel == "" {
		channel = "chromium"
	}
	e := Start(t, Options{
		Env:        map[string]string{"APP_BROWSER_ENABLED": "true", "APP_BROWSER_CHANNEL": channel},
		ExtraSites: map[string]map[string]any{hostRender: renderSite("browser")},
	})
	e.Shop.Handle("/product/rendered/", renderedShell(t))

	r := e.Extract(e.Shop.URL(hostRender, "/product/rendered/"))
	r.Success(t)
	if got := r.Str("product.title"); got != "Rendered almonds" {
		t.Errorf("product.title = %q", got)
	}
	if got := r.Str("product.price.state"); got != "available" {
		t.Errorf("product.price.state = %q", got)
	}
}

// The same page through the default "http" engine (Colly) is reported as a
// JavaScript shell, which is what the browser engine exists for.
func TestHTTPEngineReportsJavaScriptShell(t *testing.T) {
	e := Start(t, Options{ExtraSites: map[string]map[string]any{hostRender: renderSite("http")}})
	e.Shop.Handle("/product/rendered/", renderedShell(t))

	r := e.Extract(e.Shop.URL(hostRender, "/product/rendered/"))
	if r.Str("status") == "success" {
		t.Fatalf("the http engine should not see the rendered product: %s", r)
	}
}

// A site that selects "browser" on an instance without the browser engine
// fails clearly instead of silently falling back to HTTP.
func TestBrowserEngineDisabled(t *testing.T) {
	e := Start(t, Options{ExtraSites: map[string]map[string]any{hostRender: renderSite("browser")}})
	e.Shop.Handle("/product/rendered/", renderedShell(t))

	r := e.Extract(e.Shop.URL(hostRender, "/product/rendered/"))
	r.Failed(t, "ENGINE_UNAVAILABLE", "fetch", "service", true)
	if e.Shop.Hits("/product/rendered/") != 0 {
		t.Fatal("the page was fetched without the selected engine")
	}
}
