package search

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// Live tests send real queries. They run only when asked for:
//
//	SEARCH_LIVE_TEST=1 go test ./internal/fetch/search/ -run Live -v
//	SEARCH_LIVE_TEST=1 APP_BRAVE_API_KEY=... go test ./internal/fetch/search/ -run Live -v
//
// They look up the Ferguson sample product, which DuckDuckGo had at rank 1 on
// 2026-11-13, and report what came back.
const liveProduct = "https://www.fergusonhome.com/duravit-030549-0hole/s1666394"

func liveProviders(t *testing.T) []Provider {
	if os.Getenv("SEARCH_LIVE_TEST") != "1" {
		t.Skip("set SEARCH_LIVE_TEST=1 to query the real providers")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	ps := []Provider{NewDuckDuckGo(client, "go-wordpress-extractor/live-test")}
	if key := os.Getenv("APP_BRAVE_API_KEY"); key != "" {
		ps = append(ps, NewBrave(client, key, "go-wordpress-extractor/live-test", nil))
	} else {
		t.Log("APP_BRAVE_API_KEY is not set: Brave is not tested")
	}
	return ps
}

func TestLiveProviders(t *testing.T) {
	for _, p := range liveProviders(t) {
		t.Run(p.Name(), func(t *testing.T) {
			res, f := p.Search(context.Background(), Query{Text: "site:fergusonhome.com duravit 030549 0hole", Region: "us-en"})
			if f != nil {
				t.Fatalf("%s: %s %s", p.Name(), f.Code, f.RootCause)
			}
			found := false
			for _, r := range res {
				t.Logf("%d. %s\n   %s\n   %.120s", r.Rank, r.URL, r.Title, r.Snippet)
				found = found || SameURL(r.URL, liveProduct, nil)
			}
			if !found {
				t.Errorf("%s did not return %s (index coverage changes over time)", p.Name(), liveProduct)
			}
		})
	}
}
