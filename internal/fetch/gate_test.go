package fetch

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func loopbackNets() []*net.IPNet {
	_, n, _ := net.ParseCIDR("127.0.0.0/8")
	return []*net.IPNet{n}
}

func shopResolver(context.Context, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("127.0.0.1")}, nil
}

func newTestGate(t *testing.T, resolve Resolver, allowed []*net.IPNet) *gate {
	t.Helper()
	g, err := startGate(NewSafeDialer(resolve, allowed), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func clientVia(g *gate) *http.Client {
	pu, _ := url.Parse(g.URL())
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
}

func TestGateForwardsAllowedHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello "+r.Host)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	g := newTestGate(t, shopResolver, loopbackNets())
	resp, err := clientVia(g).Get("http://shop.example:" + port + "/p")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "hello shop.example") {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
}

func TestGateRefusesPrivateDestinations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the private server")
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	g := newTestGate(t, shopResolver, nil) // loopback not exempt
	resp, err := clientVia(g).Get("http://shop.example:" + port + "/p")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	var na *NotAllowedError
	if !errors.As(g.lastFailure("shop.example"), &na) {
		t.Fatalf("refusal not recorded: %v", g.lastFailure("shop.example"))
	}
}

func TestGateRefusesConnectToBlockedPort(t *testing.T) {
	g := newTestGate(t, shopResolver, loopbackNets())
	conn, err := net.Dial("tcp", strings.TrimPrefix(g.URL(), "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "CONNECT shop.example:22 HTTP/1.1\r\nHost: shop.example:22\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestGateTunnelsAllowedConnect(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "secure")
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	g := newTestGate(t, shopResolver, loopbackNets())
	pu, _ := url.Parse(g.URL())
	tr := srv.Client().Transport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(pu)
	resp, err := (&http.Client{Transport: tr}).Get("https://127.0.0.1:" + port + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "secure" {
		t.Fatalf("body %q", body)
	}
}
