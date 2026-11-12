package fetch

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/siteconfig"
)

// fakeEngine is a test strategy that records the sites it fetched.
type fakeEngine struct {
	name  string
	sites []string
}

func (f *fakeEngine) Name() string { return f.name }

func (f *fakeEngine) Fetch(_ context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure) {
	f.sites = append(f.sites, site.SiteID)
	return &Page{RequestedURL: target.String(), Status: 200}, nil
}

func siteWithEngine(id, engine string) *siteconfig.Site {
	s := testSite()
	s.SiteID = id
	s.Fetch.Engine = engine
	return s
}

func TestRouterSelectsEngineFromSiteConfig(t *testing.T) {
	httpEngine := &fakeEngine{name: EngineHTTP}
	browser := &fakeEngine{name: EngineBrowser}
	r := NewRouter(httpEngine, browser)
	target, _ := url.Parse("http://shop.example/product/1")

	for _, s := range []*siteconfig.Site{
		siteWithEngine("plain", EngineHTTP),
		siteWithEngine("default", ""),
		siteWithEngine("rendered", EngineBrowser),
	} {
		if _, fail := r.Fetch(context.Background(), s, target); fail != nil {
			t.Fatalf("%s: %+v", s.SiteID, fail)
		}
	}
	if !reflect.DeepEqual(httpEngine.sites, []string{"plain", "default"}) {
		t.Errorf("http engine fetched %v", httpEngine.sites)
	}
	if !reflect.DeepEqual(browser.sites, []string{"rendered"}) {
		t.Errorf("browser engine fetched %v", browser.sites)
	}
}

func TestRouterUnavailableEngine(t *testing.T) {
	var disabled *BrowserFetcher // typed nil, as fx provides it when the browser is off
	r := NewRouter(&fakeEngine{name: EngineHTTP}, disabled)
	if got := r.Engines(); !reflect.DeepEqual(got, []string{EngineHTTP}) {
		t.Fatalf("engines = %v, want only http", got)
	}
	target, _ := url.Parse("http://shop.example/product/1")
	page, fail := r.Fetch(context.Background(), siteWithEngine("rendered", EngineBrowser), target)
	if fail == nil || fail.Code != diagnose.EngineUnavailable || fail.Stage != diagnose.StageFetch {
		t.Fatalf("want ENGINE_UNAVAILABLE at fetch, got %+v", fail)
	}
	if page == nil || page.RequestedURL != target.String() {
		t.Fatalf("page evidence missing: %+v", page)
	}
}

func TestCollyFetcherIsTheHTTPStrategy(t *testing.T) {
	if got := testFetcher(nil, publicResolver).Name(); got != EngineHTTP {
		t.Fatalf("Fetcher.Name() = %q, want %q", got, EngineHTTP)
	}
}
