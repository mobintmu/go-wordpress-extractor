package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// Shop is a fake WooCommerce-like site. Test host names resolve to it.
type Shop struct {
	t          *testing.T
	srv        *httptest.Server
	tlsSrv     *httptest.Server
	Port       string
	TLSPort    string
	ClosedPort string

	mu        sync.Mutex
	hits      map[string]int
	hitTimes  map[string][]time.Time
	Started   chan string // receives the path when a hanging handler starts
	Cancelled chan string // receives the path when a hanging request is cancelled
}

// Host names used by the tests.
const (
	hostShop     = "shop.test"
	hostWWW      = "www.shop.test"
	hostOther    = "other.test"
	hostRial     = "rial.test"
	hostDisabled = "disabled.test"
	hostBroken   = "broken.test"
	hostPrivate  = "private.test"
	hostNoDNS    = "nodns.test"
	hostClosed   = "closed.test"
	hostTLS      = "tls.test"
	hostSlow     = "slow.test"
	hostSpaced   = "spaced.test"
	hostStrict   = "strict.test"
	hostHang     = "hang.test"
)

// NewShop starts the fake site.
func NewShop(t *testing.T) *Shop {
	s := &Shop{t: t, hits: map[string]int{}, hitTimes: map[string][]time.Time{},
		Started: make(chan string, 16), Cancelled: make(chan string, 16)}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.tlsSrv = httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(func() {
		s.srv.CloseClientConnections()
		s.srv.Close()
		s.tlsSrv.Close()
	})
	_, s.Port, _ = net.SplitHostPort(s.srv.Listener.Addr().String())
	_, s.TLSPort, _ = net.SplitHostPort(s.tlsSrv.Listener.Addr().String())
	s.ClosedPort = strconv.Itoa(freePort(t))
	return s
}

// URL builds http://host:port/path on the plain server.
func (s *Shop) URL(host, path string) string { return "http://" + host + ":" + s.Port + path }

// Resolve maps test host names; anything else does not exist.
func (s *Shop) Resolve(_ context.Context, host string) ([]net.IP, error) {
	switch host {
	case hostPrivate:
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	case hostNoDNS, "unknown.test":
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	if strings.HasSuffix(host, ".test") {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// AllowedNets exempts loopback so the fake shop is reachable; every other
// private range stays blocked.
func (s *Shop) AllowedNets() []*net.IPNet {
	_, n, _ := net.ParseCIDR("127.0.0.0/8")
	return []*net.IPNet{n}
}

// Hits returns how many times path was requested.
func (s *Shop) Hits(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

// HitTimes returns when path was requested.
func (s *Shop) HitTimes(path string) []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.hitTimes[path]...)
}

func (s *Shop) record(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[path]++
	s.hitTimes[path] = append(s.hitTimes[path], time.Now())
	return s.hits[path]
}

func (s *Shop) serve(w http.ResponseWriter, r *http.Request) {
	n := s.record(r.URL.Path)
	html := func(status int, body string) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
	switch p := r.URL.Path; p {
	case "/robots.txt":
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "User-agent: *\nDisallow: /product/robots-blocked/\n")

	case "/product/ok/", "/product/robots-blocked/":
		html(200, productPage(page{}))
	case "/product/canonical/":
		html(200, productPage(page{canonical: "http://" + hostWWW + ":" + s.Port + "/product/canonical/"}))
	case "/product/foreign-canonical/":
		html(200, productPage(page{canonical: "https://elsewhere.example/product/x/"}))
	case "/product/out-of-stock/":
		html(200, productPage(page{price: `<span class="amount">۱۲۰,۰۰۰ تومان</span>`, outOfStock: true, noButton: true}))
	case "/product/no-button/":
		html(200, productPage(page{noButton: true}))
	case "/product/call/":
		html(200, productPage(page{price: `قیمت: تماس بگیرید`, noButton: true}))
	case "/product/range/":
		html(200, productPage(page{price: `<span class="amount">۲۰۰٬۰۰۰</span><span class="amount">۱۰۰٬۰۰۰ تومان</span>`}))
	case "/product/no-price/":
		html(200, productPage(page{price: "-", noDesc: true}))
	case "/product/no-image/":
		html(200, productPage(page{noImage: true}))
	case "/product/missing-title/":
		html(200, productPage(page{noTitle: true}))
	case "/product/bad-price/":
		html(200, productPage(page{price: `<span class="amount">به زودی</span>`}))
	case "/product/rial/":
		html(200, productPage(page{price: `<del><span class="amount">۵,۰۰۰,۰۰۰ ریال</span></del><ins><span class="amount">4,000,000 ریال</span></ins>`}))
	case "/product/windows-1256/":
		enc, err := encoding.ReplaceUnsupported(charmap.Windows1256.NewEncoder()).String(productPage(page{title: "بادام ممتاز"}))
		if err != nil {
			s.t.Errorf("encode windows-1256: %v", err)
		}
		w.Header().Set("Content-Type", "text/html; charset=windows-1256")
		fmt.Fprint(w, enc)

	case "/product/gone/":
		html(404, "<html><body><h1>Not found</h1></body></html>")
	case "/product/gone410/":
		html(410, "<html><body><h1>Gone</h1></body></html>")
	case "/product/forbidden/":
		html(403, "<html><head><title>403 Forbidden</title></head><body><h1>Forbidden</h1></body></html>")
	case "/product/cf/":
		w.Header().Set("cf-mitigated", "challenge")
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("Set-Cookie", "__cf_bm=secret; Path=/")
		html(403, "<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><div id=\"challenge-platform\"></div></body></html>")
	case "/product/captcha/":
		html(200, `<html><head><title>Verify</title></head><body><form><div class="g-recaptcha" data-sitekey="x"></div></form><p>Please verify you are human before continuing to the store.</p></body></html>`)
	case "/product/rate/":
		w.Header().Set("Retry-After", "0")
		html(429, "<html><body>slow down</body></html>")
	case "/product/rate-once/":
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			html(429, "slow down")
			return
		}
		html(200, productPage(page{}))
	case "/product/rate-long/":
		w.Header().Set("Retry-After", "3600")
		html(429, "slow down")
	case "/product/error/":
		html(500, "<html><body>Internal Server Error</body></html>")
	case "/product/flaky/":
		if n == 1 {
			html(503, "<html><body>maintenance</body></html>")
			return
		}
		html(200, productPage(page{}))
	case "/product/teapot/":
		html(418, "teapot")

	case "/product/redirect-ok/":
		http.Redirect(w, r, "http://"+hostWWW+":"+s.Port+"/product/ok/", http.StatusMovedPermanently)
	case "/product/redirect-out/":
		http.Redirect(w, r, "http://"+hostOther+":"+s.Port+"/product/ok/", http.StatusFound)
	case "/product/loop/":
		k, _ := strconv.Atoi(r.URL.Query().Get("n"))
		http.Redirect(w, r, "/product/loop/?n="+strconv.Itoa(k+1), http.StatusFound)
	case "/product/redirect-category/":
		http.Redirect(w, r, "/category/nuts/", http.StatusMovedPermanently)
	case "/category/nuts/":
		html(200, `<html><body><div class="products"><div class="product-grid-item">x</div></div></body></html>`)

	case "/product/json/":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"title":"not html"}`)
	case "/product/huge/":
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(300*1024))
		w.Write([]byte(strings.Repeat("a", 300*1024)))
	case "/product/huge-chunked/":
		w.Header().Set("Content-Type", "text/html")
		chunk := []byte(strings.Repeat("<p>long</p>", 1000))
		for i := 0; i < 40; i++ {
			w.Write(chunk)
			w.(http.Flusher).Flush()
		}
	case "/product/empty/":
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
	case "/product/blank/":
		html(200, "<html><head><title>x</title></head><body><p>hi</p></body></html>")
	case "/product/spa/":
		html(200, `<!doctype html><html><head><title>Shop</title><script src="/static/app.4f2c.js"></script></head><body><div id="root"></div><script>`+strings.Repeat("window.__STATE__=1;", 400)+`</script></body></html>`)
	case "/product/changed/":
		html(200, changedLayoutPage())

	case "/product/slow/":
		select {
		case <-time.After(3 * time.Second):
			html(200, productPage(page{}))
		case <-r.Context().Done():
		}
	case "/product/slow-ok/":
		s.Started <- p
		time.Sleep(700 * time.Millisecond)
		html(200, productPage(page{}))
	case "/product/hang/":
		s.Started <- p
		select {
		case <-r.Context().Done():
			s.Cancelled <- p
		case <-time.After(30 * time.Second):
			html(200, productPage(page{}))
		}
	default:
		html(404, "<html><body>no route "+p+"</body></html>")
	}
}

type page struct {
	title      string
	noTitle    bool
	noImage    bool
	noDesc     bool
	noButton   bool
	outOfStock bool
	price      string
	canonical  string
}

// productPage renders a WooCommerce-style product page wrapped in the
// layout used by shop.test's root selector.
func productPage(p page) string {
	title := p.title
	if title == "" {
		title = "بادام درختی"
	}
	price := p.price
	if price == "" {
		price = `<del><span class="amount">500,000 تومان</span></del> <ins><span class="amount">400,000 تومان</span></ins>`
	}
	if price == "-" {
		price = ""
	}
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>` + title + ` - Shop</title>`)
	if p.canonical != "" {
		b.WriteString(`<link rel="canonical" href="` + p.canonical + `">`)
	}
	b.WriteString(`</head><body><div class="main-page-wrapper"><div class="site-content">`)
	b.WriteString(`<div class="product type-product">`)
	if !p.noImage {
		b.WriteString(`<div class="woocommerce-product-gallery"><img src="data:image/gif;base64,R0" data-src="/uploads/almond.jpg"></div>`)
	}
	if !p.noTitle {
		b.WriteString(`<h1 class="product_title">` + title + `</h1>`)
	}
	if price != "" {
		b.WriteString(`<p class="price">` + price + `</p>`)
	}
	if p.outOfStock {
		b.WriteString(`<p class="stock out-of-stock">ناموجود</p>`)
	}
	if !p.noDesc {
		b.WriteString(`<div class="woocommerce-product-details__short-description"><p>First paragraph.</p><p>Second paragraph.</p></div>`)
	}
	if !p.noButton {
		b.WriteString(`<button class="single_add_to_cart_button">Add</button>`)
	}
	b.WriteString(`</div></div></div></body></html>`)
	return b.String()
}

func changedLayoutPage() string {
	text := strings.Repeat("This product page has a new layout with plenty of readable text. ", 20)
	return `<!DOCTYPE html><html><head><title>Example product - Shop</title></head><body>
<div class="main-page-wrapper"><div class="site-content"><article class="new-product"><h1>Example product</h1><p>` + text + `</p></article></div></div></body></html>`
}

// writeConfigs writes the WooCommerce template from the repository and
// one site file per test host.
func writeConfigs(t *testing.T, dir string, s *Shop, extra map[string]map[string]any) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Join(dir, "templates"), 0o755))
	must(t, os.MkdirAll(filepath.Join(dir, "sites"), 0o755))
	tpl, err := os.ReadFile(filepath.Join("..", "..", "configs", "templates", "woocommerce.json"))
	must(t, err)
	must(t, os.WriteFile(filepath.Join(dir, "templates", "woocommerce.json"), tpl, 0o644))

	site := func(id string, domains []string, overrides map[string]any) map[string]any {
		m := map[string]any{
			"schema_version": 1,
			"site_id":        id,
			"version":        "2026-11-11.1",
			"extends":        "woocommerce",
			"domains":        domains,
			"fetch":          map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true},
			"pages": map[string]any{"product": map[string]any{
				"root": "div.main-page-wrapper div.site-content div.product.type-product",
			}},
		}
		for k, v := range overrides {
			m[k] = v
		}
		return m
	}
	sites := map[string]map[string]any{
		hostShop:     site("shop", []string{hostShop, hostWWW}, nil),
		hostRial:     site("rial", []string{hostRial}, map[string]any{"locale": map[string]any{"price_unit": "rial"}}),
		hostDisabled: site("disabled", []string{hostDisabled}, map[string]any{"enabled": false}),
		hostPrivate:  site("private", []string{hostPrivate}, nil),
		hostNoDNS:    site("nodns", []string{hostNoDNS}, nil),
		hostClosed:   site("closed", []string{hostClosed}, nil),
		hostTLS:      site("tls", []string{hostTLS}, nil),
		hostSlow:     site("slow", []string{hostSlow}, map[string]any{"fetch": map[string]any{"delay_ms": 0, "timeout_seconds": 1, "retries": 0}}),
		hostSpaced:   site("spaced", []string{hostSpaced}, map[string]any{"fetch": map[string]any{"delay_ms": 400, "timeout_seconds": 5, "retries": 0, "respect_robots_txt": false}}),
		hostStrict:   site("strict", []string{hostStrict}, map[string]any{"validation": map[string]any{"required_fields": []string{"title", "image_url"}}}),
		hostHang:     site("hang", []string{hostHang}, map[string]any{"fetch": map[string]any{"delay_ms": 0, "timeout_seconds": 60, "retries": 0}}),
	}
	for name, cfg := range extra {
		sites[name] = cfg
	}
	for name, cfg := range sites {
		b, err := json.MarshalIndent(cfg, "", "  ")
		must(t, err)
		must(t, os.WriteFile(filepath.Join(dir, "sites", name+".json"), b, 0o644))
	}
	// An invalid file: bad selector and missing version.
	must(t, os.WriteFile(filepath.Join(dir, "sites", hostBroken+".json"), []byte(`{
  "schema_version": 1, "site_id": "broken", "extends": "woocommerce", "domains": ["broken.test"],
  "pages": {"product": {"root": "div[[["}}
}`), 0o644))
}

// WriteSite adds or replaces a site file at runtime.
func (e *Env) WriteSite(name string, cfg map[string]any) {
	b, err := json.MarshalIndent(cfg, "", "  ")
	must(e.t, err)
	path := filepath.Join(e.ConfigDir, "sites", name+".json")
	must(e.t, os.WriteFile(path+".tmp", b, 0o644))
	must(e.t, os.Rename(path+".tmp", path))
}

// selfSignedCert writes a certificate for 127.0.0.1 and returns a pool
// that trusts it.
func selfSignedCert(t *testing.T, dir string) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "e2e"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	must(t, err)
	kb, err := x509.MarshalECPrivateKey(key)
	must(t, err)
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	must(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	must(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600))
	cert, err := x509.ParseCertificate(der)
	must(t, err)
	pool = x509.NewCertPool()
	pool.AddCert(cert)
	return certFile, keyFile, pool
}
