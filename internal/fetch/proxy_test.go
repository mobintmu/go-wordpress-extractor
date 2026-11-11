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

func TestProxyStillBlocksPrivateDestinations(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)

	private := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}
	target, _ := url.Parse("http://shop.example/product/1")
	_, fail := testFetcher(pu, private).Fetch(context.Background(), testSite(), target)
	if fail == nil || fail.Code != diagnose.URLNotAllowed {
		t.Fatalf("want URL_NOT_ALLOWED, got %+v", fail)
	}
	if hits.Load() != 0 {
		t.Fatal("request reached the proxy")
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

func TestCheckDestination(t *testing.T) {
	d := NewSafeDialer(publicResolver, nil)
	if err := d.CheckDestination(context.Background(), "shop.example", 443); err != nil {
		t.Fatalf("public: %v", err)
	}
	var na *NotAllowedError
	if err := d.CheckDestination(context.Background(), "shop.example", 22); !errors.As(err, &na) {
		t.Fatalf("blocked port: %v", err)
	}
	if err := d.CheckDestination(context.Background(), "127.0.0.1", 80); !errors.As(err, &na) {
		t.Fatalf("loopback: %v", err)
	}
}
