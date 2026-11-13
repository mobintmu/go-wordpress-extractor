package fetch

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch/search"
	"go-wordpress/internal/siteconfig"

	"go.uber.org/zap"
)

// Engine names for the search engines, as written in fetch.engine.
const (
	EngineDuckDuckGo = siteconfig.EngineDuckDuckGo
	EngineBrave      = siteconfig.EngineBrave
)

// closestLimit is how many non-matching result URLs are kept as evidence.
const closestLimit = 3

// SearchPage is what a search engine found for one request.
type SearchPage struct {
	Engine string
	// Query is the query whose results held the match.
	Query string
	// Result is the matching result, or nil when nothing matched.
	Result *search.Result
	// Queries lists every query sent, in order.
	Queries []diagnose.SearchQuery
	// Closest lists up to 3 product URLs on the site that came back but are
	// not the requested product. Evidence only; never used as the product.
	Closest []string
	// Cached is true when the answer came from the engine's cache.
	Cached bool
}

// Evidence returns the search part of a failure's evidence.
func (p *SearchPage) Evidence() *diagnose.SearchEvidence {
	if p == nil {
		return nil
	}
	q := p.Queries
	if q == nil {
		q = []diagnose.SearchQuery{}
	}
	return &diagnose.SearchEvidence{Engine: p.Engine, Cached: p.Cached, Queries: q, Closest: p.Closest}
}

// SearchOptions tune a SearchFetcher.
type SearchOptions struct {
	// MinInterval is the minimum time between two queries to the provider
	// from this instance, across all sites.
	MinInterval time.Duration
	// CacheTTL is how long a matched result is kept. 0 turns it off.
	CacheTTL time.Duration
	// NoResultTTL is how long a SEARCH_NO_RESULT answer is kept. 0 turns it off.
	NoResultTTL time.Duration
}

// SearchFetcher is the Engine for one search provider ("duckduckgo" or
// "brave"). It looks the product URL up and returns the matching result in
// Page.Search. It never sends a request to the product's site.
type SearchFetcher struct {
	provider search.Provider
	opts     SearchOptions
	log      *zap.Logger
	now      func() time.Time

	mu       sync.Mutex
	nextSlot time.Time
	cache    map[string]cacheEntry
}

type cacheEntry struct {
	page    SearchPage
	fail    *diagnose.Failure
	expires time.Time
}

// NewSearchFetcher builds the engine for provider. A nil provider gives a
// nil engine, which the Router skips.
func NewSearchFetcher(provider search.Provider, opts SearchOptions, log *zap.Logger) *SearchFetcher {
	if provider == nil {
		return nil
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &SearchFetcher{provider: provider, opts: opts, log: log, now: time.Now, cache: map[string]cacheEntry{}}
}

// Name is the provider name, which is also the fetch.engine value.
func (s *SearchFetcher) Name() string { return s.provider.Name() }

// Fetch looks target up. On a match the Page carries the result in Search;
// otherwise the failure is SEARCH_NO_RESULT or a provider failure, and
// Page.Search holds the queries as evidence. A request uses this one engine
// only: a failure is never retried on another engine.
func (s *SearchFetcher) Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure) {
	page := &Page{RequestedURL: target.String(), Engine: s.Name()}
	cfg := site.SearchSettings()
	key := s.Name() + "|" + search.Normalize(target.String(), cfg.IgnoreParams)

	if hit, ok := s.cached(key); ok {
		sp := hit.page
		sp.Cached = true
		page.Search = &sp
		if hit.fail != nil {
			f := *hit.fail
			return page, &f
		}
		return page, nil
	}

	sp := &SearchPage{Engine: s.Name(), Queries: []diagnose.SearchQuery{}}
	page.Search = sp
	seen := map[string]bool{}
	total := 0
	for _, text := range search.Queries(target, cfg.Queries) {
		if err := s.wait(ctx); err != nil {
			return page, deadline(ctx, "waiting for the search provider's rate limit")
		}
		qctx, cancel := context.WithTimeout(ctx, site.Timeout())
		results, fail := s.provider.Search(qctx, search.Query{Text: text, Region: cfg.Region})
		cancel()
		if ctx.Err() != nil {
			return page, deadline(ctx, "querying "+s.Name())
		}
		if fail != nil {
			s.log.Warn("search provider failed", zap.String("engine", s.Name()), zap.String("code", fail.Code), zap.String("query", text))
			return page, fail
		}
		total += len(results)
		q := diagnose.SearchQuery{Query: text, Results: len(results)}
		for i := range results {
			r := results[i]
			if search.SameURL(r.URL, target.String(), cfg.IgnoreParams) {
				q.Matched = true
				sp.Query, sp.Result = text, &r
				break
			}
			if len(sp.Closest) < closestLimit && closeCandidate(site, r.URL) && !seen[r.URL] {
				seen[r.URL] = true
				sp.Closest = append(sp.Closest, r.URL)
			}
		}
		sp.Queries = append(sp.Queries, q)
		if sp.Result != nil {
			sp.Closest = nil
			s.store(key, *sp, nil, s.opts.CacheTTL)
			return page, nil
		}
	}

	var fail *diagnose.Failure
	if total == 0 {
		fail = diagnose.New(diagnose.SearchNoResult, diagnose.ConfidenceHigh,
			"%s returned no results for the %s tried.", s.Name(), plural(len(sp.Queries), "query", "queries"))
	} else {
		fail = diagnose.New(diagnose.SearchNoResult, diagnose.ConfidenceHigh,
			"%s returned %s for the %s tried, but none is the requested product URL.",
			s.Name(), plural(total, "result", "results"), plural(len(sp.Queries), "query", "queries"))
	}
	s.store(key, *sp, fail, s.opts.NoResultTTL)
	return page, fail
}

// closeCandidate reports whether a non-matching result is worth showing as
// evidence: a product page on the site's own domains.
func closeCandidate(site *siteconfig.Site, raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && site.HasDomain(u.Hostname()) && site.MatchesProductPath(u.Path)
}

// wait reserves the next query slot for this provider and sleeps until it.
func (s *SearchFetcher) wait(ctx context.Context) error {
	s.mu.Lock()
	now := s.now()
	slot := s.nextSlot
	if slot.Before(now) {
		slot = now
	}
	s.nextSlot = slot.Add(s.opts.MinInterval)
	s.mu.Unlock()
	d := slot.Sub(now)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *SearchFetcher) cached(key string) (cacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	if !ok {
		return cacheEntry{}, false
	}
	if !s.now().Before(e.expires) {
		delete(s.cache, key)
		return cacheEntry{}, false
	}
	return e, true
}

// maxCacheEntries bounds the in-memory cache; expired entries are dropped
// first, then the whole cache is cleared if it is still full.
const maxCacheEntries = 10000

func (s *SearchFetcher) store(key string, sp SearchPage, fail *diagnose.Failure, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.cache) >= maxCacheEntries {
		for k, e := range s.cache {
			if !now.Before(e.expires) {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= maxCacheEntries {
			s.cache = map[string]cacheEntry{}
		}
	}
	s.cache[key] = cacheEntry{page: sp, fail: fail, expires: now.Add(ttl)}
}

func deadline(ctx context.Context, while string) *diagnose.Failure {
	return diagnose.New(diagnose.DeadlineExceeded, diagnose.ConfidenceHigh,
		"The request's time limit was reached while %s (%v).", while, ctx.Err()).At(diagnose.StageFetch)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// SearchSettings are the instance settings for the search engines, read
// from the APP_SEARCH_*, APP_DUCKDUCKGO_* and APP_BRAVE_* variables.
type SearchSettings struct {
	UserAgent   string
	CacheTTL    time.Duration
	NoResultTTL time.Duration

	DuckDuckGoEnabled     bool
	DuckDuckGoMinInterval time.Duration

	BraveEnabled     bool
	BraveAPIKey      string
	BraveMinInterval time.Duration
}

// NewSearchEngines builds the enabled search engines. Their HTTP client
// follows the same egress rules as page fetches (blocked addresses and
// ports, the outbound proxy).
func NewSearchEngines(opts Options, st SearchSettings, log *zap.Logger) []Engine {
	if log == nil {
		log = zap.NewNop()
	}
	var out []Engine
	if !st.DuckDuckGoEnabled && !st.BraveEnabled {
		return out
	}
	client := NewEgressClient(opts)
	if st.DuckDuckGoEnabled {
		out = append(out, NewSearchFetcher(search.NewDuckDuckGo(client, st.UserAgent), SearchOptions{
			MinInterval: st.DuckDuckGoMinInterval, CacheTTL: st.CacheTTL, NoResultTTL: st.NoResultTTL,
		}, log.Named("duckduckgo")))
	}
	if st.BraveEnabled {
		out = append(out, NewSearchFetcher(search.NewBrave(client, st.BraveAPIKey, st.UserAgent, log.Named("brave")), SearchOptions{
			MinInterval: st.BraveMinInterval, CacheTTL: st.CacheTTL, NoResultTTL: st.NoResultTTL,
		}, log.Named("brave")))
	}
	return out
}

// NewEgressClient returns an HTTP client for calls the service makes on its
// own behalf (search providers). Like page fetches, it refuses private,
// loopback and metadata addresses and blocked ports, ignores environment
// proxies, and uses the configured outbound proxy when there is one.
func NewEgressClient(opts Options) *http.Client {
	e := &Fetcher{opts: opts, dialer: NewSafeDialer(opts.Resolver, opts.AllowedNets)}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           e.dialer.DialContext,
		TLSClientConfig:       opts.TLSConfig,
		TLSHandshakeTimeout:   15 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
		ExpectContinueTimeout: time.Second,
	}
	if opts.Proxy != nil {
		transport.Proxy = e.proxyFor
		transport.DialContext = e.dialProxy
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errTooManyRedirects
			}
			return nil
		},
	}
}
