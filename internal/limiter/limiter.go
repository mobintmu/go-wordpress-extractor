// Package limiter holds the only shared in-memory state of the service:
// per-instance capacity, per-site concurrency, and per-site request spacing.
package limiter

import (
	"context"
	"errors"
	"sync"
	"time"

	"go-wordpress/internal/config"
)

// ErrSiteBusy means the site had no free slot within the wait timeout.
var ErrSiteBusy = errors.New("site busy")

// Limiter limits work per instance and per site.
type Limiter struct {
	global   chan struct{}
	perSite  int
	siteWait time.Duration
	mu       sync.Mutex
	sites    map[string]*site
}

type site struct {
	slots chan struct{}
	mu    sync.Mutex
	next  time.Time
}

// New builds a limiter from the configuration.
func New(cfg *config.Config) *Limiter {
	return &Limiter{
		global:   make(chan struct{}, cfg.MaxConcurrentRequests),
		perSite:  cfg.MaxConcurrentPerSite,
		siteWait: cfg.SiteWaitTimeout,
		sites:    map[string]*site{},
	}
}

// TryAcquire takes an instance slot without waiting. The returned func
// releases it.
func (l *Limiter) TryAcquire() (func(), bool) {
	select {
	case l.global <- struct{}{}:
		return func() { <-l.global }, true
	default:
		return nil, false
	}
}

// Active returns the number of requests holding an instance slot.
func (l *Limiter) Active() int { return len(l.global) }

func (l *Limiter) site(id string) *site {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sites[id]
	if !ok {
		s = &site{slots: make(chan struct{}, l.perSite)}
		l.sites[id] = s
	}
	return s
}

// AcquireSite waits up to the site wait timeout for a slot on siteID. It
// returns ErrSiteBusy on timeout, or the context error if ctx ends first.
func (l *Limiter) AcquireSite(ctx context.Context, siteID string) (func(), error) {
	s := l.site(siteID)
	release := func() { <-s.slots }
	select {
	case s.slots <- struct{}{}:
		return release, nil
	default:
	}
	t := time.NewTimer(l.siteWait)
	defer t.Stop()
	select {
	case s.slots <- struct{}{}:
		return release, nil
	case <-t.C:
		return nil, ErrSiteBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SiteWait returns the site wait timeout.
func (l *Limiter) SiteWait() time.Duration { return l.siteWait }

// Throttle waits until this instance may send the next request to siteID,
// keeping at least delay between requests to the same site.
func (l *Limiter) Throttle(ctx context.Context, siteID string, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	s := l.site(siteID)
	s.mu.Lock()
	now := time.Now()
	at := s.next
	if at.Before(now) {
		at = now
	}
	s.next = at.Add(delay)
	s.mu.Unlock()

	wait := time.Until(at)
	if wait <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
