package search

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/diagnose"
)

// Live tests send real queries. They run only when asked for:
//
//	SEARCH_LIVE_TEST=1 go test ./internal/fetch/search/ -run Live -v
//	SEARCH_LIVE_TEST=1 APP_BRAVE_API_KEY=... go test ./internal/fetch/search/ -run Live -v
//	SEARCH_LIVE_TEST=1 SEARCH_LIVE_SAVE=/tmp/pages go test ./internal/fetch/search/ -run Live -v
//
// SEARCH_LIVE_SAVE writes every raw answer to that directory, to refresh
// the fixtures in testdata/search/. They go through the outbound proxy in APP_PROXY_HOST and APP_PROXY_PORT
// when those are set, as the server does (some networks reach DuckDuckGo
// and Yandex only through it).
//
// They look up the Ferguson sample request the way the server would, with
// the settings of configs/sites/fergusonhome.com.json: both queries
// (site_slug, then url), region us-en, uid ignored when matching. Every
// engine is tried and the report says which one found the product, at what
// rank, after how many queries and how fast. Mojeek, Yahoo and Yandex are
// queried with the honest user agent, so a SEARCH_PROVIDER_BLOCKED answer
// from one of them is a finding, not a test bug: only an engine that
// answers and does not find the product fails the test.
const (
	liveRequest = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155"
	liveProduct = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394"
	liveUA      = "go-wordpress-extractor/live-test"
)

// liveIgnore and liveQueries mirror fetch.search in fergusonhome.com.json.
var (
	liveIgnore  = []string{"uid"}
	liveQueries = []string{"site_slug", "url"}
)

func liveClient(t *testing.T) *http.Client {
	t.Helper()
	if os.Getenv("SEARCH_LIVE_TEST") != "1" {
		t.Skip("set SEARCH_LIVE_TEST=1 to query the real providers")
	}
	proxy, err := config.ProxyURLFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
		t.Logf("through the outbound proxy %s", proxy)
	}
	var rt http.RoundTripper = transport
	if dir := os.Getenv("SEARCH_LIVE_SAVE"); dir != "" {
		rt = &savingTransport{t: t, next: transport, dir: dir}
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: rt}
}

// savingTransport writes every response body to dir, named after the host
// and the order of the request, so a live answer can become a fixture
// (SEARCH_LIVE_SAVE=<dir>). Bodies are saved as received; read them before
// committing.
type savingTransport struct {
	t    *testing.T
	next http.RoundTripper
	dir  string
	n    atomic.Int32
}

func (s *savingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := s.next.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	name := fmt.Sprintf("%02d-%s-%d.html", s.n.Add(1), strings.TrimPrefix(req.URL.Hostname(), "www."), resp.StatusCode)
	if err := os.MkdirAll(s.dir, 0o755); err == nil {
		if err := os.WriteFile(filepath.Join(s.dir, name), body, 0o644); err == nil {
			s.t.Logf("saved %s (%d bytes, %s)", name, len(body), resp.Header.Get("Content-Encoding"))
		}
	}
	return resp, nil
}

func liveProviders(t *testing.T) []Provider {
	client := liveClient(t)
	ps := []Provider{NewDuckDuckGo(client, liveUA), NewMojeek(client, liveUA, nil), NewYahoo(client, liveUA, nil), NewYandex(client, liveUA, nil)}
	if key := os.Getenv("APP_BRAVE_API_KEY"); key != "" {
		ps = append(ps, NewBrave(client, key, liveUA, nil))
	} else {
		t.Log("APP_BRAVE_API_KEY is not set: Brave is not tested")
	}
	return ps
}

// liveOutcome is what one engine did for the Ferguson request.
type liveOutcome struct {
	engine  string
	code    string // "" when the product was found
	query   int    // 1-based query that held the match
	rank    int
	results int
	took    time.Duration
}

func (o liveOutcome) String() string {
	took := o.took.Round(time.Millisecond)
	if o.code == "" {
		return fmt.Sprintf("%-10s FOUND  rank %d of query %d, %s", o.engine, o.rank, o.query, took)
	}
	return fmt.Sprintf("%-10s %s after %s", o.engine, o.code, took)
}

// TestLiveEngines runs the Ferguson request through every engine and ranks
// them: found beats not found, then fewer queries, then a better rank, then
// speed.
func TestLiveEngines(t *testing.T) {
	providers := liveProviders(t)
	target, _ := url.Parse(liveRequest)
	queries := Queries(target, liveQueries)

	var outcomes []liveOutcome
	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			o := liveOutcome{engine: p.Name(), code: diagnose.SearchNoResult}
			start := time.Now()
			for i, text := range queries {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				res, f := p.Search(ctx, Query{Text: text, Region: "us-en"})
				cancel()
				if f != nil {
					o.code = f.Code
					t.Logf("%s: %s %s", text, f.Code, f.RootCause)
					break
				}
				o.results += len(res)
				t.Logf("%s: %d results", text, len(res))
				for _, r := range res {
					t.Logf("  %d. %s\n     %s", r.Rank, r.URL, r.Title)
					if o.code != "" && SameURL(r.URL, liveRequest, liveIgnore) {
						o.code, o.query, o.rank = "", i+1, r.Rank
					}
				}
				if o.code == "" {
					break
				}
			}
			o.took = time.Since(start)
			outcomes = append(outcomes, o)
			if o.code == diagnose.SearchNoResult {
				t.Errorf("%s answered %d results but none is %s (index coverage changes over time)", p.Name(), o.results, liveProduct)
			}
		})
	}

	best := -1
	for i, o := range outcomes {
		if o.code != "" {
			continue
		}
		if best < 0 || better(o, outcomes[best]) {
			best = i
		}
	}
	t.Log("summary:")
	for _, o := range outcomes {
		t.Log("  " + o.String())
	}
	if best < 0 {
		t.Log("no engine found the product")
		return
	}
	t.Logf("best engine for this request: %s", outcomes[best].engine)
}

func better(a, b liveOutcome) bool {
	if a.query != b.query {
		return a.query < b.query
	}
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	return a.took < b.took
}
