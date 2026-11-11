package limiter

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-wordpress/internal/config"
)

func newL(global, perSite int, wait time.Duration) *Limiter {
	return New(&config.Config{MaxConcurrentRequests: global, MaxConcurrentPerSite: perSite, SiteWaitTimeout: wait})
}

func TestGlobalCapacity(t *testing.T) {
	l := newL(1, 1, time.Second)
	rel, ok := l.TryAcquire()
	if !ok || l.Active() != 1 {
		t.Fatal("first acquire failed")
	}
	if _, ok := l.TryAcquire(); ok {
		t.Fatal("second acquire must fail without waiting")
	}
	rel()
	if _, ok := l.TryAcquire(); !ok {
		t.Fatal("slot was not released")
	}
}

func TestSiteSlots(t *testing.T) {
	l := newL(10, 1, 50*time.Millisecond)
	rel, err := l.AcquireSite(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := l.AcquireSite(context.Background(), "a"); !errors.Is(err, ErrSiteBusy) || time.Since(start) < 40*time.Millisecond {
		t.Fatalf("want ErrSiteBusy after waiting, got %v", err)
	}
	if r, err := l.AcquireSite(context.Background(), "b"); err != nil {
		t.Fatal("other sites must not wait")
	} else {
		r()
	}
	go func() { time.Sleep(10 * time.Millisecond); rel() }()
	if _, err := l.AcquireSite(context.Background(), "a"); err != nil {
		t.Fatalf("waiter should get the freed slot: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.AcquireSite(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context error, got %v", err)
	}
}

func TestThrottleSpacing(t *testing.T) {
	l := newL(10, 3, time.Second)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Throttle(context.Background(), "s", 30*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 60*time.Millisecond {
		t.Fatalf("3 requests took %s, want at least 60ms", el)
	}
	if err := l.Throttle(context.Background(), "other", 0); err != nil {
		t.Fatal(err)
	}
}
