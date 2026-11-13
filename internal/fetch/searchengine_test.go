package fetch

import (
	"context"
	"net"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch/search"
	"go-wordpress/internal/siteconfig"
)

// fakeProvider answers each query from a table and records what it got.
type fakeProvider struct {
	name    string
	answers map[string][]search.Result
	fail    *diagnose.Failure
	got     []search.Query
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Search(_ context.Context, q search.Query) ([]search.Result, *diagnose.Failure) {
	f.got = append(f.got, q)
	if f.fail != nil {
		return nil, f.fail
	}
	return f.answers[q.Text], nil
}

// searchSite is a validated site, so product-path matching works.
func searchSite(t *testing.T, engine string, cfg *siteconfig.Search) *siteconfig.Site {
	t.Helper()
	s := &siteconfig.Site{
		SchemaVersion: 1, SiteID: "ferguson", Version: "1", Domains: []string{"fergusonhome.com", "www.fergusonhome.com"},
		Fetch: siteconfig.Fetch{Engine: engine, Search: cfg, TimeoutSeconds: 5},
		Pages: siteconfig.Pages{Product: siteconfig.ProductPage{
			Match: []string{`^/[a-z0-9-]+/s[0-9]+/?$`}, Root: "main",
			Fields: map[string]siteconfig.Field{"title": {Selector: "h1"}},
		}},
	}
	if err := siteconfig.Validate(s); err != nil {
		t.Fatal(err)
	}
	return s
}

const fergusonURL = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155"

func TestSearchFetcherFindsTheProduct(t *testing.T) {
	p := &fakeProvider{name: EngineDuckDuckGo, answers: map[string][]search.Result{
		"site:fergusonhome.com duravit 030549 0hole": {
			{Title: "Other size", URL: "https://www.fergusonhome.com/duravit-030543-0hole/s1666393", Rank: 1},
			{Title: "Duravit 0305490017 Starck 3 ...", URL: "https://www.fergusonhome.com/duravit-030549-0hole/s1666394", Snippet: "snip", Rank: 2},
		},
	}}
	e := NewSearchFetcher(p, SearchOptions{CacheTTL: time.Hour, NoResultTTL: time.Hour}, nil)
	site := searchSite(t, EngineDuckDuckGo, &siteconfig.Search{Region: "us-en", IgnoreParams: []string{"uid"}})
	target, _ := url.Parse(fergusonURL)

	page, fail := e.Fetch(context.Background(), site, target)
	if fail != nil {
		t.Fatal(fail)
	}
	sp := page.Search
	if sp == nil || sp.Result == nil || sp.Result.Rank != 2 || sp.Query != "site:fergusonhome.com duravit 030549 0hole" {
		t.Fatalf("search page = %+v", sp)
	}
	if page.Engine != EngineDuckDuckGo || page.Body != nil || page.Status != 0 {
		t.Errorf("a search page must not look like a fetched page: %+v", page)
	}
	if len(p.got) != 1 || p.got[0].Region != "us-en" {
		t.Errorf("queries sent = %+v (the first query matched, so only one)", p.got)
	}
	if !reflect.DeepEqual(sp.Queries, []diagnose.SearchQuery{{Query: "site:fergusonhome.com duravit 030549 0hole", Results: 2, Matched: true}}) {
		t.Errorf("queries = %+v", sp.Queries)
	}

	// Second request: answered from the cache, nothing sent.
	page, fail = e.Fetch(context.Background(), site, target)
	if fail != nil || !page.Search.Cached || len(p.got) != 1 {
		t.Fatalf("cache: %v cached=%v sent=%d", fail, page.Search.Cached, len(p.got))
	}
}

func TestSearchFetcherNoResult(t *testing.T) {
	// The real Hafele case: results that are all other products, then none.
	p := &fakeProvider{name: EngineDuckDuckGo, answers: map[string][]search.Result{
		"site:fergusonhome.com duravit 030549 0hole": {
			{URL: "https://www.fergusonhome.com/duravit-030543-0hole/s1666393", Rank: 1},
			{URL: "https://www.fergusonhome.com/category/c123", Rank: 2}, // not a product page: not in closest
			{URL: "https://elsewhere.test/duravit-030549-0hole/s1666394", Rank: 3},
			{URL: "https://www.fergusonhome.com/a/s1", Rank: 4},
			{URL: "https://www.fergusonhome.com/b/s2", Rank: 5},
			{URL: "https://www.fergusonhome.com/c/s3", Rank: 6},
		},
	}}
	e := NewSearchFetcher(p, SearchOptions{NoResultTTL: time.Hour}, nil)
	site := searchSite(t, EngineDuckDuckGo, nil) // defaults: site_slug, then url; no ignore_params
	target, _ := url.Parse(fergusonURL)

	page, fail := e.Fetch(context.Background(), site, target)
	if fail == nil || fail.Code != diagnose.SearchNoResult || fail.Retryable {
		t.Fatalf("want SEARCH_NO_RESULT, got %+v", fail)
	}
	if !strings.Contains(fail.RootCause, "returned 6 results for the 2 queries tried, but none is the requested product URL") {
		t.Errorf("root cause = %q", fail.RootCause)
	}
	ev := page.Search.Evidence()
	if len(ev.Queries) != 2 || ev.Queries[1].Query != `"fergusonhome.com/duravit-030549-0hole/s1666394"` || ev.Queries[1].Results != 0 {
		t.Errorf("queries = %+v", ev.Queries)
	}
	want := []string{"https://www.fergusonhome.com/duravit-030543-0hole/s1666393", "https://www.fergusonhome.com/a/s1", "https://www.fergusonhome.com/b/s2"}
	if !reflect.DeepEqual(ev.Closest, want) {
		t.Errorf("closest = %v, want %v", ev.Closest, want)
	}

	// The no-result answer is cached: same failure, cached, nothing sent.
	sent := len(p.got)
	page, fail = e.Fetch(context.Background(), site, target)
	if fail == nil || fail.Code != diagnose.SearchNoResult || !page.Search.Evidence().Cached || len(p.got) != sent {
		t.Fatalf("no-result cache: %+v cached=%v sent=%d", fail, page.Search.Cached, len(p.got)-sent)
	}

	empty := NewSearchFetcher(&fakeProvider{name: EngineBrave}, SearchOptions{}, nil)
	_, fail = empty.Fetch(context.Background(), searchSite(t, EngineBrave, nil), target)
	if fail == nil || fail.Code != diagnose.SearchNoResult || !strings.Contains(fail.RootCause, "returned no results for the 2 queries tried") {
		t.Fatalf("empty: %+v", fail)
	}
}

func TestSearchFetcherProviderFailureIsNotRetried(t *testing.T) {
	p := &fakeProvider{name: EngineBrave, fail: diagnose.New(diagnose.SearchQuotaExceeded, diagnose.ConfidenceHigh, "quota")}
	e := NewSearchFetcher(p, SearchOptions{NoResultTTL: time.Hour}, nil)
	target, _ := url.Parse(fergusonURL)
	site := searchSite(t, EngineBrave, nil)
	for i := 0; i < 2; i++ {
		_, fail := e.Fetch(context.Background(), site, target)
		if fail == nil || fail.Code != diagnose.SearchQuotaExceeded {
			t.Fatalf("got %+v", fail)
		}
	}
	// One query per request (it stops at the failure), and failures are not cached.
	if len(p.got) != 2 {
		t.Errorf("provider calls = %d, want 2", len(p.got))
	}
}

func TestSearchFetcherRateLimit(t *testing.T) {
	p := &fakeProvider{name: EngineDuckDuckGo}
	e := NewSearchFetcher(p, SearchOptions{MinInterval: 40 * time.Millisecond}, nil)
	target, _ := url.Parse(fergusonURL)
	start := time.Now()
	_, _ = e.Fetch(context.Background(), searchSite(t, EngineDuckDuckGo, nil), target) // 2 queries
	if d := time.Since(start); d < 40*time.Millisecond {
		t.Errorf("two queries took %s; want at least the 40ms interval between them", d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	e2 := NewSearchFetcher(p, SearchOptions{MinInterval: time.Hour}, nil)
	_, _ = e2.Fetch(context.Background(), searchSite(t, EngineDuckDuckGo, &siteconfig.Search{Queries: []string{"url"}}), target)
	if _, fail := e2.Fetch(ctx, searchSite(t, EngineDuckDuckGo, &siteconfig.Search{Queries: []string{"url"}}), target); fail == nil || fail.Code != diagnose.DeadlineExceeded {
		t.Errorf("waiting past the deadline: %+v", fail)
	}
}

func TestRouterAlternateEngines(t *testing.T) {
	brave := &fakeEngine{name: EngineBrave}
	ddg := &fakeEngine{name: EngineDuckDuckGo}
	target, _ := url.Parse("http://shop.example/product/1")
	site := siteWithEngine("s", EngineBrave)
	site.Fetch.AlternateEngines = []string{EngineDuckDuckGo}

	page, fail := NewRouter(&fakeEngine{name: EngineHTTP}, brave, ddg).Fetch(context.Background(), site, target)
	if fail != nil || len(brave.sites) != 1 || len(ddg.sites) != 0 || page.Engine != EngineBrave {
		t.Fatalf("engine enabled: brave=%v ddg=%v page.Engine=%q %v", brave.sites, ddg.sites, page.Engine, fail)
	}
	page, fail = NewRouter(&fakeEngine{name: EngineHTTP}, ddg).Fetch(context.Background(), site, target)
	if fail != nil || len(ddg.sites) != 1 || page.Engine != EngineDuckDuckGo {
		t.Fatalf("alternate: ddg=%v page.Engine=%q %v", ddg.sites, page.Engine, fail)
	}
	_, fail = NewRouter(&fakeEngine{name: EngineHTTP}).Fetch(context.Background(), site, target)
	if fail == nil || fail.Code != diagnose.EngineUnavailable || !strings.Contains(fail.RootCause, `alternates ["duckduckgo"]`) {
		t.Fatalf("none enabled: %+v", fail)
	}
}

func TestNewSearchEngines(t *testing.T) {
	if got := NewSearchEngines(DefaultOptions(), SearchSettings{}, nil); len(got) != 0 {
		t.Fatalf("both off: %d engines", len(got))
	}
	got := NewSearchEngines(DefaultOptions(), SearchSettings{DuckDuckGoEnabled: true, BraveEnabled: true, BraveAPIKey: "k"}, nil)
	if len(got) != 2 || got[0].Name() != EngineDuckDuckGo || got[1].Name() != EngineBrave {
		t.Fatalf("engines = %v", got)
	}
}

// The search client applies the same egress rules as page fetches.
func TestEgressClientRefusesPrivateAddresses(t *testing.T) {
	opts := DefaultOptions()
	opts.Resolver = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }
	_, err := NewEgressClient(opts).Get("http://search.example/")
	if err == nil || !strings.Contains(err.Error(), "not allowed") && !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("loopback was not refused: %v", err)
	}
}
