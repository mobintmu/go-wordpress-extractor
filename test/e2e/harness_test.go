// Package e2e starts the real service (Fx app, HTTP server, files on disk)
// against a fake shop and checks every scenario over HTTP. Every response
// is also validated against the Swagger doc generated into docs/.
package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go-wordpress/internal/apikey"
	"go-wordpress/internal/app"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/server/drain"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const testPepper = "e2e-pepper-0123456789abcdef-0123456789abcdef"

func TestMain(m *testing.M) {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	os.Exit(m.Run())
}

// Env is one running instance of the service.
type Env struct {
	t         *testing.T
	Base      string
	Client    *http.Client
	Key       string // active key
	Revoked   string // revoked key
	Expired   string // expired key
	KeysFile  string
	ConfigDir string
	Shop      *Shop
	Drain     *drain.State
	app       *fx.App
	stopped   bool
}

// Options change how an Env starts.
type Options struct {
	Env        map[string]string // extra APP_* variables
	Fetch      func(*fetch.Options)
	TLS        bool
	ExtraSites map[string]map[string]any
}

// Start boots a service instance and stops it when the test ends.
func Start(t *testing.T, o Options) *Env {
	t.Helper()
	shop := NewShop(t)
	dir := t.TempDir()
	e := &Env{t: t, Shop: shop, ConfigDir: filepath.Join(dir, "configs"), KeysFile: filepath.Join(dir, "secrets", "api_keys.json")}

	writeConfigs(t, e.ConfigDir, shop, o.ExtraSites)
	e.writeKeys()

	port := freePort(t)
	vars := map[string]string{
		"APP_ENV":                     "test",
		"APP_HTTP_PORT":               strconv.Itoa(port),
		"APP_HTTP_ADDRESS":            "127.0.0.1",
		"APP_SITE_CONFIG_DIR":         e.ConfigDir,
		"APP_API_KEYS_FILE":           e.KeysFile,
		"APP_API_KEY_PEPPER":          testPepper,
		"APP_OPS_USER":                "",
		"APP_OPS_PASSWORD_HASH":       "",
		"APP_TLS_CERT_FILE":           "",
		"APP_TLS_KEY_FILE":            "",
		"APP_MAX_REQUEST_DURATION":    "20s",
		"APP_SITE_WAIT_TIMEOUT":       "5s",
		"APP_MAX_CONCURRENT_REQUESTS": "20",
		"APP_MAX_CONCURRENT_PER_SITE": "1",
		"APP_RELOAD_INTERVAL":         "100ms",
	}
	scheme := "http"
	e.Client = &http.Client{Timeout: 60 * time.Second}
	if o.TLS {
		certFile, keyFile, pool := selfSignedCert(t, dir)
		vars["APP_TLS_CERT_FILE"], vars["APP_TLS_KEY_FILE"] = certFile, keyFile
		e.Client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
		scheme = "https"
	}
	for k, v := range o.Env {
		vars[k] = v
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}

	e.app = app.NewApp(
		fx.NopLogger,
		fx.Decorate(func(*zap.Logger) *zap.Logger { return zap.NewNop() }),
		fx.Decorate(func(opts fetch.Options) fetch.Options {
			opts.Resolver = shop.Resolve
			opts.AllowedNets = shop.AllowedNets()
			opts.BackoffBase = 20 * time.Millisecond
			opts.MaxBodyBytes = 256 * 1024
			if o.Fetch != nil {
				o.Fetch(&opts)
			}
			return opts
		}),
		fx.Populate(&e.Drain),
	)
	if err := e.app.Err(); err != nil {
		t.Fatalf("build app: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.app.Start(ctx); err != nil {
		t.Fatalf("start app: %v", err)
	}
	t.Cleanup(e.Stop)
	e.Base = fmt.Sprintf("%s://127.0.0.1:%d", scheme, port)
	e.waitHealthy()
	return e
}

// Stop drains and stops the instance.
func (e *Env) Stop() {
	if e.stopped {
		return
	}
	e.stopped = true
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := e.app.Stop(ctx); err != nil {
		e.t.Errorf("stop app: %v", err)
	}
}

func (e *Env) waitHealthy() {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := e.Client.Get(e.Base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatalf("service did not become healthy at %s", e.Base)
}

func (e *Env) writeKeys() {
	now := time.Now()
	active, err := apikey.Generate(testPepper, "e2e-client", 24*time.Hour, now)
	must(e.t, err)
	revoked, err := apikey.Generate(testPepper, "e2e-revoked", 0, now)
	must(e.t, err)
	revoked.Entry.Status = apikey.StatusRevoked
	expired, err := apikey.Generate(testPepper, "e2e-expired", time.Hour, now.Add(-2*time.Hour))
	must(e.t, err)
	e.Key, e.Revoked, e.Expired = active.Key, revoked.Key, expired.Key
	e.WriteKeyEntries(active.Entry, revoked.Entry, expired.Entry)
}

// WriteKeyEntries replaces the key file.
func (e *Env) WriteKeyEntries(entries ...apikey.Entry) {
	must(e.t, os.MkdirAll(filepath.Dir(e.KeysFile), 0o700))
	data, err := apikey.Encode(entries, apikey.FormatOf(e.KeysFile))
	must(e.t, err)
	must(e.t, apikey.WriteAtomic(e.KeysFile, data, 0o600))
}

// Result is a decoded HTTP response.
type Result struct {
	Status  int
	Header  http.Header
	Raw     []byte
	JSON    map[string]any
	Elapsed time.Duration
}

// Extract posts {"url": url} with the active key.
func (e *Env) Extract(url string) *Result {
	return e.Post("/api/v1/extract", e.Key, map[string]any{"url": url, "request_id": "t-" + strconv.FormatInt(time.Now().UnixNano(), 36)})
}

// ExtractCtx posts with a client context, for cancellation tests.
func (e *Env) ExtractCtx(ctx context.Context, url string) (*Result, error) {
	b, _ := json.Marshal(map[string]any{"url": url})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.Base+"/api/v1/extract", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(apikey.HeaderName, e.Key)
	return e.do(req)
}

// Post sends body (marshalled unless it is []byte or string) with key.
func (e *Env) Post(path, key string, body any) *Result {
	var rd io.Reader
	switch b := body.(type) {
	case []byte:
		rd = bytes.NewReader(b)
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		must(e.t, err)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(http.MethodPost, e.Base+path, rd)
	must(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(apikey.HeaderName, key)
	}
	r, err := e.do(req)
	must(e.t, err)
	return r
}

// Get sends a GET with optional headers.
func (e *Env) Get(path string, headers map[string]string) *Result {
	req, err := http.NewRequest(http.MethodGet, e.Base+path, nil)
	must(e.t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r, err := e.do(req)
	must(e.t, err)
	return r
}

func (e *Env) do(req *http.Request) (*Result, error) {
	start := time.Now()
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	r := &Result{Status: resp.StatusCode, Header: resp.Header, Raw: raw, Elapsed: time.Since(start)}
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&r.JSON); err != nil {
			e.t.Errorf("response is not JSON: %v\n%s", err, raw)
		}
	}
	checkContract(e.t, req.Method, req.URL.Path, r)
	return r, nil
}

// ---- assertions on results ----

// Str reads a string at a dotted path, for example "error.code".
func (r *Result) Str(path string) string {
	v := r.At(path)
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// Int reads an integer at path; -1 when missing or null.
func (r *Result) Int(path string) int64 {
	n, ok := r.At(path).(json.Number)
	if !ok {
		return -1
	}
	i, _ := n.Int64()
	return i
}

// At reads any value at a dotted path. Numeric segments index arrays.
func (r *Result) At(path string) any {
	var cur any = r.JSON
	for _, seg := range strings.Split(path, ".") {
		switch c := cur.(type) {
		case map[string]any:
			cur = c[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i >= len(c) {
				return nil
			}
			cur = c[i]
		default:
			return nil
		}
	}
	return cur
}

// Has reports whether path exists (even with a null value).
func (r *Result) Has(path string) bool {
	parts := strings.Split(path, ".")
	parent := r.JSON
	if len(parts) > 1 {
		m, ok := r.At(strings.Join(parts[:len(parts)-1], ".")).(map[string]any)
		if !ok {
			return false
		}
		parent = m
	}
	_, ok := parent[parts[len(parts)-1]]
	return ok
}

func (r *Result) String() string { return fmt.Sprintf("HTTP %d %s", r.Status, r.Raw) }

// Failed asserts a failed result with code, stage, and owner.
func (r *Result) Failed(t *testing.T, code, stage, owner string, retryable bool) {
	t.Helper()
	if r.Status != http.StatusOK || r.Str("status") != "failed" {
		t.Fatalf("want HTTP 200 failed, got %s", r)
	}
	if got := r.Str("error.code"); got != code {
		t.Fatalf("want code %s, got %s\n%s", code, got, r.Raw)
	}
	if got := r.Str("error.stage"); got != stage {
		t.Errorf("%s: want stage %s, got %s", code, stage, got)
	}
	if got := r.Str("error.owner"); got != owner {
		t.Errorf("%s: want owner %s, got %s", code, owner, got)
	}
	if got := r.At("error.retryable"); got != retryable {
		t.Errorf("%s: want retryable %v, got %v", code, retryable, got)
	}
	for _, f := range []string{"error.message", "error.root_cause", "error.confidence"} {
		if r.Str(f) == "" {
			t.Errorf("%s: %s is empty", code, f)
		}
	}
	if !r.Has("partial.product") {
		t.Errorf("%s: partial.product is missing", code)
	}
	if r.Has("product") || r.Has("warnings") {
		t.Errorf("%s: failed result must not carry product or warnings", code)
	}
}

// Success asserts a success result.
func (r *Result) Success(t *testing.T) {
	t.Helper()
	if r.Status != http.StatusOK || r.Str("status") != "success" {
		t.Fatalf("want success, got %s", r)
	}
	if _, ok := r.At("warnings").([]any); !ok {
		t.Errorf("success must carry a warnings array: %s", r.Raw)
	}
	if r.Has("error") || r.Has("evidence") || r.Has("partial") {
		t.Errorf("success must not carry error, evidence, or partial: %s", r.Raw)
	}
}

// APIError asserts a non-200 error body.
func (r *Result) APIError(t *testing.T, status int, code string) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("want HTTP %d, got %s", status, r)
	}
	if got := r.Str("code"); got != code {
		t.Fatalf("want code %s, got %s", code, r.Raw)
	}
	if r.Str("message") == "" {
		t.Errorf("message is empty: %s", r.Raw)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func newAppForStartupTest() *fx.App {
	return app.NewApp(fx.NopLogger, fx.Decorate(func(*zap.Logger) *zap.Logger { return zap.NewNop() }))
}
