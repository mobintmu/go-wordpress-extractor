package e2e

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go-wordpress/internal/apikey"
)

func TestRequestValidation(t *testing.T) {
	e := Start(t, Options{})
	cases := []struct {
		name string
		body any
		code string
	}{
		{"not JSON", "{not json", "INVALID_REQUEST"},
		{"JSON array", "[]", "INVALID_REQUEST"},
		{"missing url", map[string]any{"request_id": "x"}, "INVALID_REQUEST"},
		{"empty url", map[string]any{"url": "  "}, "INVALID_REQUEST"},
		{"request_id too long", map[string]any{"url": e.Shop.URL(hostShop, "/product/ok/"), "request_id": strings.Repeat("x", 129)}, "INVALID_REQUEST"},
		{"body over 1 MiB", `{"url":"` + strings.Repeat("a", 1<<20) + `"}`, "INVALID_REQUEST"},
		{"relative url", map[string]any{"url": "/product/ok/"}, "INVALID_URL"},
		{"ftp scheme", map[string]any{"url": "ftp://shop.test/product/ok/"}, "INVALID_URL"},
		{"javascript scheme", map[string]any{"url": "javascript:alert(1)"}, "INVALID_URL"},
		{"malformed", map[string]any{"url": "http://%zz/"}, "INVALID_URL"},
		{"no host", map[string]any{"url": "http:///product/ok/"}, "INVALID_URL"},
		{"credentials in url", map[string]any{"url": "http://user:pass@shop.test/product/ok/"}, "INVALID_URL"},
		{"url too long", map[string]any{"url": "http://shop.test/" + strings.Repeat("a", 2100)}, "INVALID_URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.Post("/api/v1/extract", e.Key, c.body).APIError(t, http.StatusBadRequest, c.code)
		})
	}
}

func TestAuthentication(t *testing.T) {
	e := Start(t, Options{})
	url := e.Shop.URL(hostShop, "/product/ok/")
	body := map[string]any{"url": url}
	id, _ := apikey.Parse(e.Key)
	wrongSecret := apikey.Format(id, strings.Repeat("A", apikey.SecretLength))

	var first []byte
	for name, key := range map[string]string{
		"missing":      "",
		"malformed":    "not-a-key",
		"unknown id":   apikey.Format("zzzzzzzz", strings.Repeat("A", apikey.SecretLength)),
		"wrong secret": wrongSecret,
		"revoked":      e.Revoked,
		"expired":      e.Expired,
	} {
		t.Run(name, func(t *testing.T) {
			r := e.Post("/api/v1/extract", key, body)
			r.APIError(t, http.StatusUnauthorized, "UNAUTHORIZED")
			if first == nil {
				first = r.Raw
			} else if string(first) != string(r.Raw) {
				t.Errorf("every 401 must be identical: %s vs %s", first, r.Raw)
			}
		})
	}
	t.Run("auth is checked before the body", func(t *testing.T) {
		e.Post("/api/v1/extract", "", "{bad json").APIError(t, http.StatusUnauthorized, "UNAUTHORIZED")
	})
	t.Run("no fetch happens for rejected keys", func(t *testing.T) {
		if e.Shop.Hits("/product/ok/") != 0 {
			t.Errorf("the target was fetched without a valid key")
		}
	})
	t.Run("valid key", func(t *testing.T) {
		e.Post("/api/v1/extract", e.Key, body).Success(t)
	})
}

func TestInstanceCapacity(t *testing.T) {
	e := Start(t, Options{Env: map[string]string{"APP_MAX_CONCURRENT_REQUESTS": "1"}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ExtractCtx(ctx, e.Shop.URL(hostHang, "/product/hang/")) //nolint:errcheck
	}()
	waitFor(t, e.Shop.Started, "/product/hang/")

	r := e.Extract(e.Shop.URL(hostShop, "/product/ok/"))
	r.APIError(t, http.StatusTooManyRequests, "TOO_MANY_REQUESTS")
	if r.Header.Get("Retry-After") == "" {
		t.Errorf("429 must carry Retry-After")
	}
	cancel()
	<-done
	// Capacity is released once the first request ends.
	eventually(t, func() bool { return e.Extract(e.Shop.URL(hostShop, "/product/ok/")).Status == 200 })
}

func TestPerSiteConcurrency(t *testing.T) {
	e := Start(t, Options{Env: map[string]string{"APP_SITE_WAIT_TIMEOUT": "300ms"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.ExtractCtx(ctx, e.Shop.URL(hostHang, "/product/hang/")) //nolint:errcheck
	waitFor(t, e.Shop.Started, "/product/hang/")

	t.Run("same site waits, then 429", func(t *testing.T) {
		r := e.Extract(e.Shop.URL(hostHang, "/product/ok/"))
		r.APIError(t, http.StatusTooManyRequests, "TOO_MANY_REQUESTS")
		if r.Elapsed < 250*time.Millisecond {
			t.Errorf("returned after %s; it should wait for the site's turn first", r.Elapsed)
		}
		if !strings.Contains(r.Str("message"), "site") || r.Header.Get("Retry-After") == "" {
			t.Errorf("got %s", r)
		}
	})
	t.Run("other sites are not blocked", func(t *testing.T) {
		e.Extract(e.Shop.URL(hostShop, "/product/ok/")).Success(t)
	})
	t.Run("waiting request gets the slot when it frees up", func(t *testing.T) {
		e2 := Start(t, Options{Env: map[string]string{"APP_SITE_WAIT_TIMEOUT": "5s"}})
		go func() {
			r, err := e2.ExtractCtx(context.Background(), e2.Shop.URL(hostShop, "/product/slow-ok/"))
			if err != nil || r.Status != 200 {
				t.Errorf("first request: %v %v", err, r)
			}
		}()
		waitFor(t, e2.Shop.Started, "/product/slow-ok/")
		e2.Extract(e2.Shop.URL(hostShop, "/product/ok/")).Success(t)
	})
}

func TestPerSiteDelay(t *testing.T) {
	e := Start(t, Options{})
	e.Extract(e.Shop.URL(hostSpaced, "/product/ok/")).Success(t)
	e.Extract(e.Shop.URL(hostSpaced, "/product/ok/")).Success(t)
	hits := e.Shop.HitTimes("/product/ok/")
	if len(hits) != 2 {
		t.Fatalf("hits = %d", len(hits))
	}
	if gap := hits[1].Sub(hits[0]); gap < 380*time.Millisecond {
		t.Errorf("gap between requests = %s, want at least delay_ms 400", gap)
	}
}

func TestClientDisconnectCancelsFetch(t *testing.T) {
	e := Start(t, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	go e.ExtractCtx(ctx, e.Shop.URL(hostHang, "/product/hang/")) //nolint:errcheck
	waitFor(t, e.Shop.Started, "/product/hang/")
	cancel()
	waitFor(t, e.Shop.Cancelled, "/product/hang/")
}

func TestHealthReadyAndDrain(t *testing.T) {
	e := Start(t, Options{})
	if r := e.Get("/health", nil); r.Status != 200 || r.Str("message") != "OK" {
		t.Fatalf("health: %s", r)
	}
	if r := e.Get("/ready", nil); r.Status != 200 {
		t.Fatalf("ready: %s", r)
	}
	if r := e.Get("/nope", nil); r.Status != 404 || r.Str("code") != "NOT_FOUND" {
		t.Fatalf("unknown route: %s", r)
	}

	t.Run("running requests finish while stopping", func(t *testing.T) {
		e2 := Start(t, Options{})
		result := make(chan *Result, 1)
		go func() {
			r, err := e2.ExtractCtx(context.Background(), e2.Shop.URL(hostShop, "/product/slow-ok/"))
			if err != nil {
				t.Errorf("in-flight request failed: %v", err)
			}
			result <- r
		}()
		waitFor(t, e2.Shop.Started, "/product/slow-ok/")
		e2.Stop()
		r := <-result
		if r == nil || r.Str("status") != "success" {
			t.Fatalf("in-flight request was not completed: %v", r)
		}
		if !e2.Drain.Draining() {
			t.Errorf("drain flag not set on stop")
		}
	})

	t.Run("draining instance returns 503", func(t *testing.T) {
		e.Drain.Start()
		e.Get("/ready", nil).APIError(t, http.StatusServiceUnavailable, "SHUTTING_DOWN")
		e.Extract(e.Shop.URL(hostShop, "/product/ok/")).APIError(t, http.StatusServiceUnavailable, "SHUTTING_DOWN")
		if r := e.Get("/health", nil); r.Status != 200 {
			t.Errorf("health must stay 200 while draining")
		}
	})
}

const (
	opsUser     = "operator"
	opsPassword = "correct horse battery staple"
)

func opsEnv(t *testing.T) map[string]string {
	return map[string]string{"APP_OPS_USER": opsUser, "APP_OPS_PASSWORD_HASH": apikey.Hash(testPepper, opsPassword)}
}

func opsAuthHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(opsUser+":"+opsPassword))
}

func TestOperatorEndpoints(t *testing.T) {
	t.Run("off by default", func(t *testing.T) {
		e := Start(t, Options{})
		for _, p := range []string{"/swagger/doc.json", "/metrics", "/swagger/index.html"} {
			if r := e.Get(p, map[string]string{"Authorization": opsAuthHeader()}); r.Status != 404 {
				t.Errorf("%s: want 404 when operator credentials are unset, got %d", p, r.Status)
			}
		}
	})

	e := Start(t, Options{Env: opsEnv(t)})
	e.Extract(e.Shop.URL(hostShop, "/product/ok/")).Success(t)
	e.Extract(e.Shop.URL(hostShop, "/product/gone/"))
	e.Post("/api/v1/extract", "", map[string]any{"url": "x"})

	bad := map[string]map[string]string{
		"no credentials": nil,
		"wrong password": {"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(opsUser+":nope"))},
		"wrong user":     {"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("root:"+opsPassword))},
		"client api key": {apikey.HeaderName: e.Key},
	}
	for _, p := range []string{"/swagger/doc.json", "/metrics", "/swagger/index.html"} {
		for name, h := range bad {
			t.Run(p+" "+name, func(t *testing.T) {
				r := e.Get(p, h)
				if r.Status != 401 || !strings.HasPrefix(r.Header.Get("WWW-Authenticate"), "Basic") {
					t.Errorf("want 401 with WWW-Authenticate, got %d %q", r.Status, r.Header.Get("WWW-Authenticate"))
				}
			})
		}
	}

	auth := map[string]string{"Authorization": opsAuthHeader()}
	t.Run("swagger ui", func(t *testing.T) {
		r := e.Get("/swagger/index.html", auth)
		if r.Status != 200 || !strings.Contains(string(r.Raw), "swagger") {
			t.Errorf("swagger ui: %d", r.Status)
		}
		init := e.Get("/swagger/swagger-initializer.js", auth)
		if !strings.Contains(string(init.Raw), "doc.json") {
			t.Errorf("swagger ui does not load doc.json: %s", init.Raw)
		}
	})
	t.Run("metrics", func(t *testing.T) {
		r := e.Get("/metrics", auth)
		body := string(r.Raw)
		for _, want := range []string{
			`extract_requests_total{outcome="success",site="shop",code="",price_state="available"} 1`,
			`extract_requests_total{outcome="failed",site="shop",code="HTTP_NOT_FOUND",price_state=""} 1`,
			`extract_rejected_total{reason="unauthorized"} 1`,
			`extract_request_duration_seconds_count{outcome="success"} 1`,
			`extract_active_requests 0`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("metrics missing %q\n%s", want, body)
			}
		}
	})
}

func TestHTTPS(t *testing.T) {
	e := Start(t, Options{TLS: true})
	if !strings.HasPrefix(e.Base, "https://") {
		t.Fatalf("base = %s", e.Base)
	}
	e.Extract(e.Shop.URL(hostShop, "/product/ok/")).Success(t)
	plain := &http.Client{Timeout: 5 * time.Second}
	resp, err := plain.Get(strings.Replace(e.Base, "https://", "http://", 1) + "/health")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Errorf("plain HTTP must not be served when TLS is on")
		}
	}
}

func TestHotReload(t *testing.T) {
	e := Start(t, Options{})
	url := e.Shop.URL(hostShop, "/product/ok/")

	t.Run("new site file is picked up", func(t *testing.T) {
		if r := e.Extract(e.Shop.URL("new.test", "/product/ok/")); r.Str("error.code") != "SITE_NOT_CONFIGURED" {
			t.Fatalf("got %s", r)
		}
		e.WriteSite("new.test", map[string]any{
			"schema_version": 1, "site_id": "newsite", "version": "v2", "extends": "woocommerce",
			"domains": []string{"new.test"},
			"fetch":   map[string]any{"delay_ms": 0, "retries": 0},
		})
		eventually(t, func() bool {
			r := e.Extract(e.Shop.URL("new.test", "/product/ok/"))
			return r.Str("status") == "success" && r.Str("config_version") == "v2"
		})
	})

	t.Run("revoked key stops working without a restart", func(t *testing.T) {
		entries, _, err := apikey.ReadFile(e.KeysFile, false)
		must(t, err)
		added, err := apikey.Generate(testPepper, "added-later", 0, time.Now())
		must(t, err)
		for i := range entries {
			if entries[i].Name == "e2e-client" {
				entries[i].Status = apikey.StatusRevoked
			}
		}
		e.WriteKeyEntries(append(entries, added.Entry)...)
		eventually(t, func() bool { return e.Post("/api/v1/extract", e.Key, map[string]any{"url": url}).Status == 401 })
		e.Post("/api/v1/extract", added.Key, map[string]any{"url": url}).Success(t)
		e.Key = added.Key

		t.Run("a bad file keeps the previous keys", func(t *testing.T) {
			must(t, apikey.WriteAtomic(e.KeysFile, []byte("APIKEY_BROKEN=nonsense\n"), 0o600))
			time.Sleep(400 * time.Millisecond)
			e.Post("/api/v1/extract", added.Key, map[string]any{"url": url}).Success(t)
		})
	})

	t.Run("an in-flight request keeps its snapshot", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		var r *Result
		go func() {
			defer wg.Done()
			r, _ = e.ExtractCtx(context.Background(), e.Shop.URL("new.test", "/product/slow-ok/"))
		}()
		waitFor(t, e.Shop.Started, "/product/slow-ok/")
		e.WriteSite("new.test", map[string]any{
			"schema_version": 1, "site_id": "newsite", "version": "v3", "extends": "woocommerce",
			"domains": []string{"new.test"}, "fetch": map[string]any{"delay_ms": 0, "retries": 0},
		})
		wg.Wait()
		if r == nil || r.Str("config_version") != "v2" {
			t.Fatalf("in-flight request should report the snapshot it started with: %v", r)
		}
	})
}

func waitFor(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case got := <-ch:
			if got == want {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("condition not met within 5s")
}
