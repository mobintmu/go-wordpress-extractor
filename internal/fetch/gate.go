package fetch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// gate is a forward proxy on 127.0.0.1 that the browser engine sends all
// of Chrome's traffic through. Chrome resolves nothing itself: every
// connection is dialed here, with the same SafeDialer (or configured
// outbound proxy) as the HTTP engine. That keeps the SSRF rules — no
// private, loopback or metadata addresses, no blocked ports, checks after
// DNS resolution — identical for both engines.
type gate struct {
	ln        net.Listener
	srv       *http.Server
	dialer    *SafeDialer
	upstream  *url.URL
	transport *http.Transport
	log       *zap.Logger

	// failures remembers why the last connection to a host was refused, so
	// the browser engine can report URL_NOT_ALLOWED or DNS_FAILED instead of
	// Chrome's generic net::ERR_TUNNEL_CONNECTION_FAILED.
	mu       sync.Mutex
	failures map[string]gateFailure
}

type gateFailure struct {
	err error
	at  time.Time
}

// gateRefusedHeader marks a response the gate made itself because it
// refused or could not reach the destination; for plain http:// Chrome
// shows it as the page, so the browser engine must not treat it as the
// site's answer.
const gateRefusedHeader = "X-Extractor-Egress-Refused"

// gateFailureTTL bounds how long a refusal is attributed to a host.
const gateFailureTTL = time.Minute

func startGate(dialer *SafeDialer, upstream *url.URL, log *zap.Logger) (*gate, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for the browser egress gate: %w", err)
	}
	g := &gate{ln: ln, dialer: dialer, upstream: upstream, log: log, failures: make(map[string]gateFailure)}
	g.transport = &http.Transport{
		Proxy:                 nil,
		DialContext:           g.dialDirect,
		TLSHandshakeTimeout:   15 * time.Second,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	if upstream != nil {
		g.transport.Proxy = g.proxyFor
		g.transport.DialContext = g.dialUpstream
	}
	g.srv = &http.Server{Handler: g, ReadHeaderTimeout: 15 * time.Second}
	go func() {
		if err := g.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("browser egress gate stopped", zap.Error(err))
		}
	}()
	return g, nil
}

// URL is the proxy address handed to Chrome.
func (g *gate) URL() string { return "http://" + g.ln.Addr().String() }

func (g *gate) Close() error {
	g.transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := g.srv.Shutdown(ctx); err != nil {
		return g.srv.Close()
	}
	return nil
}

// lastFailure returns why the latest connection to host was refused, if
// that happened recently.
func (g *gate) lastFailure(host string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	f, ok := g.failures[strings.ToLower(host)]
	if !ok || time.Since(f.at) > gateFailureTTL {
		return nil
	}
	return f.err
}

func (g *gate) record(host string, err error) {
	host = strings.ToLower(host)
	g.mu.Lock()
	defer g.mu.Unlock()
	if err == nil {
		delete(g.failures, host)
		return
	}
	g.failures[host] = gateFailure{err: err, at: time.Now()}
	for h, f := range g.failures { // keep the map small
		if time.Since(f.at) > gateFailureTTL {
			delete(g.failures, h)
		}
	}
}

func (g *gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		g.tunnel(w, r)
		return
	}
	if !r.URL.IsAbs() || r.URL.Scheme != "http" {
		http.Error(w, "only absolute http:// requests and CONNECT are proxied", http.StatusBadRequest)
		return
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	removeHopHeaders(out.Header)
	resp, err := g.transport.RoundTrip(out)
	g.record(r.URL.Hostname(), err)
	if err != nil {
		g.refuse(w, r.URL.Host, err)
		return
	}
	defer resp.Body.Close()
	removeHopHeaders(resp.Header)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// tunnel serves CONNECT (HTTPS and WebSocket over TLS).
func (g *gate) tunnel(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		http.Error(w, "CONNECT needs host:port", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var upstream net.Conn
	if g.upstream == nil {
		upstream, err = g.dialer.DialContext(ctx, "tcp", r.Host)
	} else {
		upstream, err = g.connectViaUpstream(ctx, r.Host)
	}
	g.record(host, err)
	if err != nil {
		g.refuse(w, r.Host, err)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	client, buf, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	if n := buf.Reader.Buffered(); n > 0 {
		pending, _ := buf.Reader.Peek(n)
		if _, err := upstream.Write(pending); err != nil {
			client.Close()
			upstream.Close()
			return
		}
	}
	pipe(client, upstream)
}

func (g *gate) refuse(w http.ResponseWriter, target string, err error) {
	status := http.StatusBadGateway
	var notAllowed *NotAllowedError
	if errors.As(err, &notAllowed) {
		status = http.StatusForbidden
	}
	g.log.Debug("browser egress refused", zap.String("target", target), zap.Error(err))
	w.Header().Set(gateRefusedHeader, "1")
	http.Error(w, err.Error(), status)
}

// dialDirect is used for plain http:// requests without an outbound proxy.
func (g *gate) dialDirect(ctx context.Context, network, addr string) (net.Conn, error) {
	return g.dialer.DialContext(ctx, network, addr)
}

// proxyFor checks the destination before handing a plain http:// request
// to the outbound proxy, as the HTTP engine does.
func (g *gate) proxyFor(req *http.Request) (*url.URL, error) {
	if err := g.dialer.CheckDestination(req.Context(), req.URL.Hostname(), portOf(req.URL)); err != nil {
		return nil, err
	}
	return g.upstream, nil
}

// dialUpstream connects to the configured outbound proxy and nowhere else.
func (g *gate) dialUpstream(ctx context.Context, network, addr string) (net.Conn, error) {
	if addr != g.upstream.Host {
		return nil, &NotAllowedError{Host: addr, Reason: "not the configured proxy"}
	}
	conn, err := g.dialer.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, &proxyError{addr: addr, err: err}
	}
	return conn, nil
}

// connectViaUpstream opens a CONNECT tunnel to hostport through the
// outbound proxy, after checking the destination.
func (g *gate) connectViaUpstream(ctx context.Context, hostport string) (net.Conn, error) {
	host, portStr, _ := net.SplitHostPort(hostport)
	port, _ := strconv.Atoi(portStr)
	if err := g.dialer.CheckDestination(ctx, host, port); err != nil {
		return nil, err
	}
	conn, err := g.dialUpstream(ctx, "tcp", g.upstream.Host)
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: hostport}, Host: hostport, Header: make(http.Header)}
	if u := g.upstream.User; u != nil {
		req.SetBasicAuth(u.Username(), passwordOf(u))
		req.Header.Set("Proxy-Authorization", req.Header.Get("Authorization"))
		req.Header.Del("Authorization")
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, &proxyError{addr: g.upstream.Host, err: err}
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, &proxyError{addr: g.upstream.Host, err: err}
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, &proxyError{addr: g.upstream.Host, err: fmt.Errorf("CONNECT %s: %s", hostport, resp.Status)}
	}
	_ = conn.SetDeadline(time.Time{})
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

func passwordOf(u *url.Userinfo) string {
	p, _ := u.Password()
	return p
}

// bufferedConn returns bytes the proxy sent right after its CONNECT reply.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// pipe copies both ways until either side closes, then closes both.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	a.Close()
	b.Close()
	<-done
}

var hopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func removeHopHeaders(h http.Header) {
	for _, f := range strings.Split(h.Get("Connection"), ",") {
		if f = strings.TrimSpace(f); f != "" {
			h.Del(f)
		}
	}
	for _, k := range hopHeaders {
		h.Del(k)
	}
}
