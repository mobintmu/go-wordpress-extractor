package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go-wordpress/internal/diagnose"

	mws "github.com/jcalvert/metawebsearch"
)

// scrapedServer serves body with status for every request and records the
// requests, so a test sees what the provider sent and how often.
type scrapedServer struct {
	*httptest.Server
	body   []byte
	status atomic.Int32
	hits   atomic.Int32
	last   atomic.Pointer[http.Request]
}

func newScrapedServer(t *testing.T, body []byte) *scrapedServer {
	t.Helper()
	s := &scrapedServer{body: body}
	s.status.Store(http.StatusOK)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.Clone(context.Background()))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(int(s.status.Load()))
		_, _ = w.Write(s.body)
	}))
	t.Cleanup(s.Close)
	return s
}

const scrapedUA = "go-wordpress-extractor/test"

func scrapedProviders(client *http.Client) map[string]*Scraped {
	return map[string]*Scraped{
		ProviderMojeek: NewMojeek(client, scrapedUA, nil),
		ProviderYahoo:  NewYahoo(client, scrapedUA, nil),
		ProviderYandex: NewYandex(client, scrapedUA, nil),
	}
}

// The synthetic pages hold the Ferguson sample product. Each engine's page
// is parsed by the library's selectors; Yahoo's redirect links are unwrapped
// and its Bing ad is dropped.
func TestScrapedParsesFixtures(t *testing.T) {
	const product = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394"
	cases := map[string]struct {
		results int
		rank    int
		query   string // the query parameter the engine uses
	}{
		ProviderMojeek: {3, 2, "q"},
		ProviderYahoo:  {2, 1, "p"},
		ProviderYandex: {2, 1, "text"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newScrapedServer(t, fixture(t, name, "synthetic-fergusonhome.html"))
			p := scrapedProviders(srv.Client())[name].WithEndpoint(srv.URL)
			if p.Name() != name {
				t.Fatalf("Name() = %q", p.Name())
			}
			res, f := p.Search(context.Background(), Query{Text: "site:fergusonhome.com duravit 030549 0hole", Region: "us-en"})
			if f != nil || len(res) != c.results {
				t.Fatalf("%d results, %v", len(res), f)
			}
			r := res[c.rank-1]
			if r.Rank != c.rank || r.URL != product {
				t.Errorf("result = %+v", r)
			}
			if !strings.HasPrefix(r.Title, "Duravit 0305490017 Starck 3") || !strings.HasSuffix(r.Title, "| Ferguson Home") {
				t.Errorf("title = %q", r.Title)
			}
			if !strings.HasPrefix(r.Snippet, "Duravit 030549-0HOLE Features:") {
				t.Errorf("snippet = %q", r.Snippet)
			}
			for i, r := range res {
				if r.Rank != i+1 || strings.Contains(r.URL, "bing.com") || strings.Contains(r.URL, "r.search.yahoo.com") {
					t.Errorf("result %d = %+v", i, r)
				}
			}
			req := srv.last.Load()
			if got := req.URL.Query().Get(c.query); got != "site:fergusonhome.com duravit 030549 0hole" {
				t.Errorf("%s = %q (URL %s)", c.query, got, req.URL)
			}
			if req.Header.Get("User-Agent") != scrapedUA {
				t.Errorf("User-Agent = %q, want the honest one", req.Header.Get("User-Agent"))
			}
			for _, h := range browserOnlyHeaders {
				if v := req.Header.Get(h); v != "" {
					t.Errorf("%s = %q was sent; the service does not claim to be a browser", h, v)
				}
			}
			if req.Header.Get("Accept") == "" {
				t.Error("Accept is missing")
			}
		})
	}
}

// Mojeek takes the region as cookies; with no region none are sent.
func TestScrapedMojeekRegion(t *testing.T) {
	srv := newScrapedServer(t, fixture(t, ProviderMojeek, "synthetic-fergusonhome.html"))
	p := NewMojeek(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL)
	cookies := func() map[string]string {
		out := map[string]string{}
		for _, c := range srv.last.Load().Cookies() {
			out[c.Name] = c.Value
		}
		return out
	}
	if _, f := p.Search(context.Background(), Query{Text: "x", Region: "de-de"}); f != nil {
		t.Fatal(f)
	}
	if got := cookies(); got["arc"] != "DE" || got["lb"] != "de" {
		t.Errorf("de-de cookies = %v", got)
	}
	for _, region := range []string{"wt-wt", ""} {
		if _, f := p.Search(context.Background(), Query{Text: "x", Region: region}); f != nil {
			t.Fatal(f)
		}
		got := cookies()
		if _, ok := got["arc"]; ok || got["lb"] != "" {
			t.Errorf("region %q must send no region cookies, got %v", region, got)
		}
		if got["safesearch"] == "" {
			t.Errorf("region %q dropped the other cookies too: %v", region, got)
		}
	}
}

// A block is reported once and never retried; other statuses are provider
// errors. The library would retry 202, 429 and 503 three times with 5-20 s
// backoff: the hit count proves it does not.
func TestScrapedStatusMapping(t *testing.T) {
	srv := newScrapedServer(t, fixture(t, ProviderYahoo, "synthetic-fergusonhome.html"))
	p := NewYahoo(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL)
	for status, code := range map[int]string{
		202: diagnose.SearchProviderBlocked, 403: diagnose.SearchProviderBlocked, 429: diagnose.SearchProviderBlocked,
		500: diagnose.SearchProviderError, 503: diagnose.SearchProviderError, 404: diagnose.SearchProviderError,
	} {
		srv.status.Store(int32(status))
		before := srv.hits.Load()
		start := time.Now()
		_, f := p.Search(context.Background(), Query{Text: "x"})
		if f == nil || f.Code != code {
			t.Errorf("HTTP %d: %v, want %s", status, f, code)
			continue
		}
		if !strings.Contains(f.RootCause, "Yahoo answered HTTP "+strconv.Itoa(status)) {
			t.Errorf("HTTP %d: root cause %q", status, f.RootCause)
		}
		if n := srv.hits.Load() - before; n != 1 {
			t.Errorf("HTTP %d: %d requests, want exactly 1 (no retry)", status, n)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("HTTP %d: took %s; the library's retry backoff must be off", status, d)
		}
	}
}

// A 200 page with no results is "no results", unless it is a bot check.
func TestScrapedChallengeAndEmptyPages(t *testing.T) {
	srv := newScrapedServer(t, []byte(`<html><head><title>Mojeek</title></head><body><p>No results for your query.</p></body></html>`))
	p := NewMojeek(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL)
	res, f := p.Search(context.Background(), Query{Text: "x"})
	if f != nil || res == nil || len(res) != 0 {
		t.Fatalf("empty page: %v %v", res, f)
	}
	for _, page := range []string{
		`<html><head><title>Are you a robot?</title></head><body><form class="captcha"><img src="/captcha.png"></form></body></html>`,
		`<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><div id="challenge-platform"></div></body></html>`,
	} {
		srv.body = []byte(page)
		_, f := p.Search(context.Background(), Query{Text: "x"})
		if f == nil || f.Code != diagnose.SearchProviderBlocked || f.Confidence != diagnose.ConfidenceMedium {
			t.Errorf("challenge page: %+v", f)
		}
	}
	// Yandex redirects a suspected bot to /showcaptcha: the final URL says so.
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/showcaptcha" {
			_, _ = w.Write([]byte(`<html><body><div class="Form"></div></body></html>`))
			return
		}
		http.Redirect(w, r, "/showcaptcha?retpath=x", http.StatusFound)
	}))
	defer redirecting.Close()
	_, f = NewYandex(redirecting.Client(), scrapedUA, nil).WithEndpoint(redirecting.URL).Search(context.Background(), Query{Text: "x"})
	if f == nil || f.Code != diagnose.SearchProviderBlocked {
		t.Errorf("captcha redirect: %+v", f)
	}
}

// The results page is read up to maxResultsPage bytes, so a huge page
// neither fails nor exhausts memory; results before the cut are kept.
func TestScrapedSizeCap(t *testing.T) {
	page := fixture(t, ProviderYandex, "synthetic-fergusonhome.html")
	huge := append(append([]byte{}, page...), []byte(strings.Repeat("<!-- padding -->", maxResultsPage/16+1))...)
	srv := newScrapedServer(t, huge)
	res, f := NewYandex(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL).Search(context.Background(), Query{Text: "x"})
	if f != nil || len(res) != 2 {
		t.Fatalf("%d results, %v", len(res), f)
	}
}

// The library's global limiter would sleep 2 s between two queries to the
// same engine. SearchFetcher spaces queries instead, with the context.
func TestScrapedNoGlobalSleep(t *testing.T) {
	srv := newScrapedServer(t, fixture(t, ProviderMojeek, "synthetic-fergusonhome.html"))
	p := NewMojeek(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, f := p.Search(context.Background(), Query{Text: "x"}); f != nil {
			t.Fatal(f)
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("three queries took %s; the library's rate limiter must be off", d)
	}
}

func TestScrapedTransportAndEndpointErrors(t *testing.T) {
	srv := newScrapedServer(t, nil)
	srv.Close()
	_, f := NewMojeek(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL).Search(context.Background(), Query{Text: "x"})
	if f == nil || f.Code != diagnose.SearchProviderError || !strings.Contains(f.RootCause, "The request to Mojeek failed") {
		t.Errorf("closed server: %+v", f)
	}
	_, f = NewMojeek(http.DefaultClient, scrapedUA, nil).WithEndpoint("not a url").Search(context.Background(), Query{Text: "x"})
	if f == nil || f.Code != diagnose.SearchProviderError || !strings.Contains(f.RootCause, "could not be built") {
		t.Errorf("bad endpoint: %+v", f)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv2 := newScrapedServer(t, fixture(t, ProviderMojeek, "synthetic-fergusonhome.html"))
	if _, f := NewMojeek(srv2.Client(), scrapedUA, nil).WithEndpoint(srv2.URL).Search(ctx, Query{Text: "x"}); f == nil {
		t.Error("a cancelled context must fail")
	}
}

// Google needs a browser TLS profile; the service does not impersonate one.
func TestHardenRefusesClientProfile(t *testing.T) {
	google, ok := mws.EngineByName("google")
	if !ok || google.ClientProfile == "" {
		t.Skip("the library's Google engine no longer needs a TLS profile")
	}
	if _, err := harden(google, scrapedUA, "", false); err == nil || !strings.Contains(err.Error(), "TLS profile") {
		t.Fatalf("err = %v", err)
	}
	for _, e := range []mws.EngineConfig{mws.Mojeek, mws.Yahoo, mws.Yandex} {
		h, err := harden(e, scrapedUA, "", false)
		if err != nil || h.MinDelay != 0 || h.RetryableStatus(429) || h.RetryableStatus(503) || h.RetryableStatus(202) {
			t.Errorf("%s: %+v %v", e.Name, h, err)
		}
	}
}

// harden reviews each EngineConfig field (scraped.go). A library upgrade
// that adds one must be looked at there before this number changes.
func TestHardenCoversEveryEngineConfigField(t *testing.T) {
	typ := reflect.TypeOf(mws.EngineConfig{})
	want := []string{"Name", "BuildRequest", "ParseResponse", "PostProcess", "ClientProfile", "MinDelay", "MaxRetries", "RetryableStatus"}
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metawebsearch.EngineConfig fields = %v, want %v: review harden() for the new field", got, want)
	}
}

// Real answers saved on 2026-11-15 for the Ferguson sample request
// (SEARCH_LIVE_SAVE): Yahoo had the product at rank 1, Mojeek and Yandex
// answered the honest client with a CAPTCHA page, and Yandex answered curl
// with a page that lists other Ferguson products but not the requested one. Yahoo's session crumb and request guids are
// redacted.
func TestScrapedRealPages(t *testing.T) {
	const product = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394"
	query := Query{Text: "site:fergusonhome.com duravit 030549 0hole", Region: "us-en"}

	t.Run("yahoo has the product at rank 1", func(t *testing.T) {
		srv := newScrapedServer(t, fixture(t, ProviderYahoo, "fergusonhome-site-slug.html"))
		res, f := NewYahoo(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL).Search(context.Background(), query)
		if f != nil || len(res) < 5 {
			t.Fatalf("%d results, %v", len(res), f)
		}
		r := res[0]
		if r.Rank != 1 || !SameURL(r.URL, product+"?uid=3938155", []string{"uid"}) {
			t.Errorf("first result = %+v", r)
		}
		if !strings.HasPrefix(r.Title, "Duravit 0305490017 Starck 3") || !strings.Contains(r.Snippet, "Ferguson") {
			t.Errorf("title %q, snippet %q", r.Title, r.Snippet)
		}
		for _, r := range res {
			if u := r.URL; !strings.HasPrefix(u, "https://www.fergusonhome.com/") {
				t.Errorf("result %d is not a direct Ferguson URL (redirect not unwrapped, or an ad): %s", r.Rank, u)
			}
		}
	})

	for name, build := range map[string]func(*http.Client, string) *Scraped{
		ProviderMojeek: func(c *http.Client, ua string) *Scraped { return NewMojeek(c, ua, nil) },
		ProviderYandex: func(c *http.Client, ua string) *Scraped { return NewYandex(c, ua, nil) },
	} {
		t.Run(name+" CAPTCHA page is SEARCH_PROVIDER_BLOCKED", func(t *testing.T) {
			srv := newScrapedServer(t, fixture(t, name, "captcha.html"))
			_, f := build(srv.Client(), scrapedUA).WithEndpoint(srv.URL).Search(context.Background(), query)
			if f == nil || f.Code != diagnose.SearchProviderBlocked || !strings.Contains(f.RootCause, "bot check") {
				t.Fatalf("got %+v", f)
			}
		})
	}

	t.Run("yandex no-match page lists other products, not the requested one", func(t *testing.T) {
		// Titled "There are no search results for ...", the page still lists
		// other Ferguson pages. None of them may be taken for the product.
		srv := newScrapedServer(t, fixture(t, ProviderYandex, "fergusonhome-no-match.html"))
		res, f := NewYandex(srv.Client(), scrapedUA, nil).WithEndpoint(srv.URL).Search(context.Background(), query)
		if f != nil || len(res) == 0 {
			t.Fatalf("got %v %+v; the page is not a challenge and has results", res, f)
		}
		for _, r := range res {
			if SameURL(r.URL, product, nil) {
				t.Errorf("result %d is the product, but Yandex did not have it: %s", r.Rank, r.URL)
			}
		}
	})
}
