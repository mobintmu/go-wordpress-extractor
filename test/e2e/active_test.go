package e2e

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"go-wordpress/internal/apikey"
)

const activePath = "/api/v1/active"

// hold starts an extraction of url that stays open until the returned
// func is called. The func cancels the request and waits for it to end.
func hold(t *testing.T, e *Env, url string) (release func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ExtractCtx(ctx, url) //nolint:errcheck
	}()
	var once sync.Once
	release = func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(release)
	return release
}

func (e *Env) active(t *testing.T) int64 {
	t.Helper()
	r := e.Get(activePath, map[string]string{apikey.HeaderName: e.Key})
	if r.Status != http.StatusOK {
		t.Fatalf("%s: HTTP %d %s", activePath, r.Status, r.Raw)
	}
	return r.Int("extract_active_requests")
}

func TestActiveRequestsAuthentication(t *testing.T) {
	e := Start(t, Options{Env: opsEnv(t)})
	for name, h := range map[string]map[string]string{
		"missing key":    nil,
		"empty key":      {apikey.HeaderName: ""},
		"unknown key":    {apikey.HeaderName: "gwp_unknown_secret"},
		"revoked key":    {apikey.HeaderName: e.Revoked},
		"expired key":    {apikey.HeaderName: e.Expired},
		"operator login": {"Authorization": opsAuthHeader()},
	} {
		t.Run(name, func(t *testing.T) {
			e.Get(activePath, h).APIError(t, http.StatusUnauthorized, "UNAUTHORIZED")
		})
	}
	t.Run("rejections are counted", func(t *testing.T) {
		r := e.Get("/metrics", map[string]string{apikey.HeaderName: e.Key})
		if got := r.Int("rejected.unauthorized"); got != 6 {
			t.Errorf("rejected.unauthorized = %d, want 6", got)
		}
	})
	t.Run("only GET", func(t *testing.T) {
		if r := e.Post(activePath, e.Key, map[string]any{}); r.Status == http.StatusOK {
			t.Errorf("POST %s: got 200", activePath)
		}
	})
}

func TestActiveRequests(t *testing.T) {
	e := Start(t, Options{Env: map[string]string{
		"APP_MAX_CONCURRENT_REQUESTS": "3",
		"APP_SITE_WAIT_TIMEOUT":       "10s",
	}})

	t.Run("idle", func(t *testing.T) {
		r := e.Get(activePath, map[string]string{apikey.HeaderName: e.Key})
		if r.Status != http.StatusOK {
			t.Fatalf("HTTP %d %s", r.Status, r.Raw)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", r.Header.Get("Cache-Control"))
		}
		if len(r.JSON) != 1 || r.Int("extract_active_requests") != 0 {
			t.Errorf(`want exactly {"extract_active_requests":0}, got %s`, r.Raw)
		}
	})

	t.Run("finished requests are not active", func(t *testing.T) {
		e.Extract(e.Shop.URL(hostShop, "/product/ok/")).Success(t)
		e.Extract(e.Shop.URL(hostShop, "/product/gone/"))
		if n := e.active(t); n != 0 {
			t.Errorf("active = %d after requests finished, want 0", n)
		}
	})

	t.Run("rejected requests are not active", func(t *testing.T) {
		e.Post("/api/v1/extract", "", map[string]any{"url": e.Shop.URL(hostShop, "/product/ok/")})
		e.Post("/api/v1/extract", e.Key, map[string]any{"url": "not a url"})
		if n := e.active(t); n != 0 {
			t.Errorf("active = %d, want 0", n)
		}
	})

	t.Run("counts running extractions across sites", func(t *testing.T) {
		releaseHang := hold(t, e, e.Shop.URL(hostHang, "/product/hang/"))
		waitFor(t, e.Shop.Started, "/product/hang/")
		if n := e.active(t); n != 1 {
			t.Fatalf("active = %d with one running, want 1", n)
		}

		releaseShop := hold(t, e, e.Shop.URL(hostShop, "/product/hang/"))
		waitFor(t, e.Shop.Started, "/product/hang/")
		if n := e.active(t); n != 2 {
			t.Fatalf("active = %d with two running, want 2", n)
		}

		// A third request for a busy site waits for that site's slot. It
		// already holds an instance slot, so it counts as active.
		releaseWaiting := hold(t, e, e.Shop.URL(hostWWW, "/product/ok/"))
		eventually(t, func() bool { return e.active(t) == 3 })

		t.Run("at capacity, a 429 does not add to the count", func(t *testing.T) {
			e.Extract(e.Shop.URL(hostSlow, "/product/ok/")).APIError(t, http.StatusTooManyRequests, "TOO_MANY_REQUESTS")
			if n := e.active(t); n != 3 {
				t.Errorf("active = %d after a 429, want 3", n)
			}
		})

		t.Run("matches /metrics", func(t *testing.T) {
			m := e.Get("/metrics", map[string]string{apikey.HeaderName: e.Key})
			if m.Int("load.active") != 3 || m.Int("load.capacity") != 3 {
				t.Errorf("/metrics load = %v, want active 3 of 3", m.At("load"))
			}
		})

		releaseWaiting()
		eventually(t, func() bool { return e.active(t) == 2 })
		releaseShop()
		eventually(t, func() bool { return e.active(t) == 1 })
		releaseHang()
		eventually(t, func() bool { return e.active(t) == 0 })
	})

	t.Run("slots are reusable after release", func(t *testing.T) {
		e.Extract(e.Shop.URL(hostShop, "/product/ok/")).Success(t)
		if n := e.active(t); n != 0 {
			t.Errorf("active = %d, want 0", n)
		}
	})
}

// Many clients polling while extractions start and end must always see a
// value between 0 and the capacity.
func TestActiveRequestsUnderLoad(t *testing.T) {
	e := Start(t, Options{Env: map[string]string{"APP_MAX_CONCURRENT_REQUESTS": "4"}})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Extract(e.Shop.URL(hostShop, "/product/ok/")) // 200 or 429, both fine
		}()
	}
	stop := make(chan struct{})
	var pollers sync.WaitGroup
	for range 4 {
		pollers.Add(1)
		go func() {
			defer pollers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				r := e.Get(activePath, map[string]string{apikey.HeaderName: e.Key})
				if n := r.Int("extract_active_requests"); r.Status != http.StatusOK || n < 0 || n > 4 {
					t.Errorf("HTTP %d, active = %d, want 200 and 0..4", r.Status, n)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	pollers.Wait()
	eventually(t, func() bool { return e.active(t) == 0 })
}
