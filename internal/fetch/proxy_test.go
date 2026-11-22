package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"go.uber.org/zap"
)

func testSite() *siteconfig.Site {
	zero, f := 0, false
	return &siteconfig.Site{
		SiteID:  "shop",
		Domains: []string{"shop.example"},
		Fetch:   siteconfig.Fetch{UserAgent: "test", DelayMS: &zero, TimeoutSeconds: 5, Retries: &zero, RespectRobotsTxt: &f},
	}
}

func testFetcher(proxy *url.URL, resolve Resolver) *Fetcher {
	lim := limiter.New(&config.Config{MaxConcurrentRequests: 2, MaxConcurrentPerSite: 1, SiteWaitTimeout: time.Second})
	opts := DefaultOptions()
	opts.Resolver = resolve
	opts.Proxy = proxy
	opts.BackoffBase = time.Millisecond
	return New(opts, lim, zap.NewNop())
}

func publicResolver(context.Context, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("93.184.216.34")}, nil
}

// The proxy listens on loopback, which the safe dialer would refuse; the
// configured proxy must still be reachable and receive the full URL.
func TestFetchThroughProxy(t *testing.T) {
	var seen atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.String())
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)

	target, _ := url.Parse("http://shop.example/product/1")
	page, fail := testFetcher(pu, publicResolver).Fetch(context.Background(), testSite(), target)
	if fail != nil {
		t.Fatalf("fetch failed: %+v", fail)
	}
	if page.Status != 200 || seen.Load() != "http://shop.example/product/1" {
		t.Fatalf("status %d, proxy saw %v", page.Status, seen.Load())
	}
}

// Through a configured proxy the proxy resolves hostnames: a name whose
// local resolution is poisoned to a private address is still sent to the
// proxy, and the local resolver is never asked.
func TestProxyResolvesHostnames(t *testing.T) {
	var seen atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.String())
		w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)

	var resolved atomic.Int32
	poisoned := func(context.Context, string) ([]net.IP, error) {
		resolved.Add(1)
		return []net.IP{net.ParseIP("10.10.34.36")}, nil
	}
	target, _ := url.Parse("http://shop.example/product/1")
	page, fail := testFetcher(pu, poisoned).Fetch(context.Background(), testSite(), target)
	if fail != nil || page.Status != 200 {
		t.Fatalf("want the proxy to serve the page, got %+v", fail)
	}
	if resolved.Load() != 0 {
		t.Fatalf("local resolver was asked %d times; the proxy must resolve", resolved.Load())
	}
}

// Private, loopback and metadata destinations stay refused even through a
// proxy: as IP literals and as loopback or metadata names. None reaches it.
func TestProxyStillBlocksPrivateLiteralsAndLoopbackNames(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)

	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/admin",
		"http://127.0.0.1/admin",
		"http://localhost/admin",
		"http://metadata.google.internal/computeMetadata",
	} {
		target, _ := url.Parse(raw)
		_, fail := testFetcher(pu, publicResolver).Fetch(context.Background(), testSite(), target)
		if fail == nil || fail.Code != diagnose.URLNotAllowed {
			t.Errorf("%s: want URL_NOT_ALLOWED, got %+v", raw, fail)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("%d request(s) reached the proxy", hits.Load())
	}
}

func TestProxyUnreachable(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	target, _ := url.Parse("http://shop.example/product/1")
	_, fail := testFetcher(&url.URL{Scheme: "http", Host: addr}, publicResolver).Fetch(context.Background(), testSite(), target)
	if fail == nil || fail.Code != diagnose.ConnectionFailed {
		t.Fatalf("want CONNECTION_FAILED, got %+v", fail)
	}
}

func TestCheckProxiedDestination(t *testing.T) {
	d := NewSafeDialer(nil, nil)
	var na *NotAllowedError
	if err := d.CheckProxiedDestination("shop.example", 443); err != nil {
		t.Fatalf("hostname: %v", err)
	}
	if err := d.CheckProxiedDestination("www.homedepot.com", 443); err != nil {
		t.Fatalf("hostname that the local resolver would not see: %v", err)
	}
	if err := d.CheckProxiedDestination("shop.example", 22); !errors.As(err, &na) {
		t.Fatalf("blocked port: %v", err)
	}
	for _, h := range []string{"127.0.0.1", "10.10.34.36", "169.254.169.254", "localhost", "api.localhost", "metadata.google.internal", "box.local"} {
		if err := d.CheckProxiedDestination(h, 443); !errors.As(err, &na) {
			t.Errorf("%s: want refused, got %v", h, err)
		}
	}
}
