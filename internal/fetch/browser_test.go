package fetch

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"go.uber.org/zap"
)

// startBrowser launches the browser engine, or skips the test when the
// Playwright driver or a browser is not installed on this machine.
// Install them with:
//
//	go run github.com/playwright-community/playwright-go/cmd/playwright install chromium
func startBrowser(t *testing.T, resolve Resolver, allowed []*net.IPNet) *BrowserFetcher {
	t.Helper()
	if testing.Short() {
		t.Skip("browser tests are skipped in -short mode")
	}
	lim := limiter.New(&config.Config{MaxConcurrentRequests: 4, MaxConcurrentPerSite: 1, SiteWaitTimeout: 5 * time.Second})
	opts := DefaultOptions()
	opts.Resolver = resolve
	opts.AllowedNets = allowed
	opts.BackoffBase = 10 * time.Millisecond
	bopts := DefaultBrowserOptions()
	bopts.Channel = os.Getenv("APP_BROWSER_CHANNEL")
	b := NewBrowserFetcher(opts, bopts, lim, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Skipf("browser engine unavailable: %v", err)
	}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })
	return b
}

func browserSite() *siteconfig.Site {
	s := testSite()
	s.Fetch.Engine = EngineBrowser
	s.Domains = []string{"shop.example"}
	s.Fetch.TimeoutSeconds = 15
	s.Pages.Product.Root = "main.product"
	return s
}

func shopURL(t *testing.T, srv *httptest.Server, path string) *url.URL {
	t.Helper()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	u, _ := url.Parse("http://shop.example:" + port + path)
	return u
}

func fetchCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestBrowserRendersJavaScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><html><body><div id="app"></div><script>
setTimeout(function () {
  document.getElementById('app').innerHTML = '<main class="product"><h1>Rendered by JS</h1></main>';
}, 300);
</script></body></html>`)
	}))
	defer srv.Close()

	b := startBrowser(t, shopResolver, loopbackNets())
	page, fail := b.Fetch(fetchCtx(t), browserSite(), shopURL(t, srv, "/product/1"))
	if fail != nil {
		t.Fatalf("fetch failed: %+v", fail)
	}
	if page.Status != 200 || !strings.Contains(string(page.Body), "Rendered by JS") {
		t.Fatalf("status %d, body %q", page.Status, page.Body)
	}
	if page.FinalURL == nil || page.FinalURL.Hostname() != "shop.example" {
		t.Fatalf("final URL %v", page.FinalURL)
	}
}

// A challenge page answers 403, sets a clearance cookie with JavaScript
// and reloads; the real page then comes back 200. The engine must report
// the real page, and the next fetch of the same site reuses the cookie.
func TestBrowserPassesSelfReloadingChallenge(t *testing.T) {
	var challenges, products atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if c, err := r.Cookie("clearance"); err != nil || c.Value != "ok" {
			challenges.Add(1)
			w.Header().Set("cf-mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<html><head><title>Just a moment...</title></head><body><script>
setTimeout(function () { document.cookie = 'clearance=ok; path=/'; location.reload(); }, 200);
</script></body></html>`)
			return
		}
		products.Add(1)
		io.WriteString(w, `<html><body><main class="product"><h1>Real product</h1></main></body></html>`)
	}))
	defer srv.Close()

	b := startBrowser(t, shopResolver, loopbackNets())
	site := browserSite()
	page, fail := b.Fetch(fetchCtx(t), site, shopURL(t, srv, "/product/1"))
	if fail != nil {
		t.Fatalf("fetch failed: %+v", fail)
	}
	if page.Status != 200 || page.Headers.Get("cf-mitigated") != "" || !strings.Contains(string(page.Body), "Real product") {
		t.Fatalf("status %d, cf-mitigated %q, body %q", page.Status, page.Headers.Get("cf-mitigated"), page.Body)
	}
	if challenges.Load() != 1 {
		t.Fatalf("challenges served = %d, want 1", challenges.Load())
	}

	// Second fetch: the site's cookie is reused, so no challenge.
	if _, fail := b.Fetch(fetchCtx(t), site, shopURL(t, srv, "/product/2")); fail != nil {
		t.Fatalf("second fetch failed: %+v", fail)
	}
	if challenges.Load() != 1 || products.Load() != 2 {
		t.Fatalf("after second fetch: challenges %d, products %d", challenges.Load(), products.Load())
	}

	// Another site never sees that cookie.
	other := browserSite()
	other.SiteID = "other"
	if _, fail := b.Fetch(fetchCtx(t), other, shopURL(t, srv, "/product/3")); fail != nil {
		t.Fatalf("other site fetch failed: %+v", fail)
	}
	if challenges.Load() != 2 {
		t.Fatalf("cookie leaked across sites: challenges %d", challenges.Load())
	}
}

func TestBrowserRefusesPrivateDestinations(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	b := startBrowser(t, shopResolver, nil) // loopback is not exempt here
	_, fail := b.Fetch(fetchCtx(t), browserSite(), shopURL(t, srv, "/product/1"))
	if fail == nil || fail.Code != diagnose.URLNotAllowed {
		t.Fatalf("want URL_NOT_ALLOWED, got %+v", fail)
	}
	if hits.Load() != 0 {
		t.Fatalf("Chrome reached the loopback server %d times", hits.Load())
	}
}

func TestBrowserBlocksSubresourcesToPrivateAddresses(t *testing.T) {
	var mu sync.Mutex
	var internalHit bool
	internal := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		internalHit = true
		mu.Unlock()
	}))
	defer internal.Close()
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><body><main class="product"><h1>p</h1><img src="`+internal.URL+`/secret.png"></main></body></html>`)
	}))
	defer shop.Close()

	// Only 127.0.0.2 is exempt: shop.example resolves there, while the
	// <img> points at 127.0.0.1, which the gate checks as-is and refuses.
	allowed := []*net.IPNet{{IP: net.ParseIP("127.0.0.2"), Mask: net.CIDRMask(32, 32)}}
	resolve := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.2")}, nil }
	b := startBrowser(t, resolve, allowed)

	// Serve the shop on 127.0.0.2 as well.
	l, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skipf("127.0.0.2 not available: %v", err)
	}
	srv2 := httptest.NewUnstartedServer(shop.Config.Handler)
	srv2.Listener.Close()
	srv2.Listener = l
	srv2.Start()
	defer srv2.Close()

	if _, fail := b.Fetch(fetchCtx(t), browserSite(), shopURL(t, srv2, "/product/1")); fail != nil {
		t.Fatalf("fetch failed: %+v", fail)
	}
	mu.Lock()
	defer mu.Unlock()
	if internalHit {
		t.Fatal("an <img> on the page reached a private address")
	}
}

func TestBrowserRefusesOffDomainRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "" && strings.HasPrefix(r.Host, "shop.example") {
			_, port, _ := net.SplitHostPort(r.Host)
			http.Redirect(w, r, "http://evil.example:"+port+"/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><body><main class="product">elsewhere</main></body></html>`)
	}))
	defer srv.Close()

	b := startBrowser(t, shopResolver, loopbackNets())
	_, fail := b.Fetch(fetchCtx(t), browserSite(), shopURL(t, srv, "/product/1"))
	if fail == nil || fail.Code != diagnose.RedirectNotAllowed {
		t.Fatalf("want REDIRECT_NOT_ALLOWED, got %+v", fail)
	}
}

func TestBrowserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `<html><body>gone</body></html>`)
	}))
	defer srv.Close()

	b := startBrowser(t, shopResolver, loopbackNets())
	start := time.Now()
	_, fail := b.Fetch(fetchCtx(t), browserSite(), shopURL(t, srv, "/product/1"))
	if fail == nil || fail.Code != diagnose.HTTPNotFound {
		t.Fatalf("want HTTP_NOT_FOUND, got %+v", fail)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("a 404 waited %s for the product root", d)
	}
}
