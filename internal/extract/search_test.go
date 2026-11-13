package extract

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/fetch/search"
	"go-wordpress/internal/siteconfig"
)

// pageFetcher returns a fixed page and failure, as an engine would.
type pageFetcher struct {
	page *fetch.Page
	fail *diagnose.Failure
}

func (f pageFetcher) Fetch(_ context.Context, _ *siteconfig.Site, u *url.URL) (*fetch.Page, *diagnose.Failure) {
	p := *f.page
	p.RequestedURL = u.String()
	return &p, f.fail
}

func reasons(uf []UnavailableField) map[string]string {
	out := map[string]string{}
	for _, f := range uf {
		out[f.Field] = f.Reason
	}
	return out
}

func warningCodes(ws []Warning) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

// The real DuckDuckGo result for the Ferguson product (2026-11-13).
func TestSearchSuccessFerguson(t *testing.T) {
	result := search.Result{
		Title:   `Duravit 0305490017 Starck 3 20-7/8" Rectangular Ceramic Undermount ...`,
		URL:     "https://www.fergusonhome.com/duravit-030549-0hole/s1666394",
		Snippet: `Duravit 030549-0HOLE Features: Covered under Duravit's 5 year warranty Specifications: Overall Length: 20-7/8" (left to right of sink)`,
		Rank:    1,
	}
	page := &fetch.Page{Engine: fetch.EngineDuckDuckGo, Search: &fetch.SearchPage{
		Engine: fetch.EngineDuckDuckGo, Query: "site:fergusonhome.com duravit 030549 0hole", Result: &result,
		Queries: []diagnose.SearchQuery{{Query: "site:fergusonhome.com duravit 030549 0hole", Results: 10, Matched: true}},
	}}
	u, _ := url.Parse("https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155")
	resp, err := repoService(t, pageFetcher{page: page}).Run(context.Background(), "r1", u)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusSuccess {
		t.Fatalf("status %s: %+v", resp.Status, resp.Error)
	}
	p := resp.Product
	if p.Title != `Duravit 0305490017 Starck 3 20-7/8" Rectangular Ceramic Undermount` || p.URL != result.URL {
		t.Errorf("title/url = %q / %q", p.Title, p.URL)
	}
	if p.Description == nil || !strings.HasPrefix(*p.Description, "Duravit 030549-0HOLE Features") || p.ImageURL != nil {
		t.Errorf("description/image = %v / %v", p.Description, p.ImageURL)
	}
	if p.Price.Current != nil || p.Price.State != PriceUnknown || p.Price.Currency != CurrencyUSD {
		t.Errorf("price = %+v", p.Price)
	}
	want := map[string]string{
		"image_url": ReasonNotInSearchResults, "price": ReasonNotInSearchResults,
		"specs": ReasonNotInSearchResults, "availability": ReasonNotConfigured,
	}
	if got := reasons(resp.UnavailableFields); !reflect.DeepEqual(got, want) {
		t.Errorf("unavailable_fields = %v, want %v", got, want)
	}
	if resp.Completeness != CompletenessPartial {
		t.Errorf("completeness = %q", resp.Completeness)
	}
	if got := warningCodes(resp.Warnings); !reflect.DeepEqual(got, []string{WarnSearchTitleTruncated, WarnSearchSnippet, WarnPriceMissing}) {
		t.Errorf("warnings = %v", got)
	}
	src := resp.Source
	if src == nil || src.Engine != "duckduckgo" || src.SiteContacted || src.Query != "site:fergusonhome.com duravit 030549 0hole" || src.ResultRank != 1 {
		t.Errorf("source = %+v", src)
	}
	b, _ := json.Marshal(resp)
	for _, s := range []string{`"completeness":"partial"`, `"site_contacted":false`, `"unavailable_fields":[{`} {
		if !strings.Contains(string(b), s) {
			t.Errorf("JSON lacks %s: %s", s, b)
		}
	}
}

// The real Hafele case: DuckDuckGo has not indexed the sample product.
func TestSearchNoResultHafele(t *testing.T) {
	page := &fetch.Page{Engine: fetch.EngineDuckDuckGo, Search: &fetch.SearchPage{
		Engine: fetch.EngineDuckDuckGo,
		Queries: []diagnose.SearchQuery{
			{Query: "site:hafele.com.de universalverbinder haefele ixconnect uc 16 64", Results: 10},
			{Query: `"hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571"`, Results: 0},
		},
		Closest: []string{"https://www.hafele.com.de/de/product/korpusverbinder-haefele-ixconnect-rc-40-22-t-mit-rastfunktion-oberteil/P-00861403/"},
	}}
	fail := diagnose.New(diagnose.SearchNoResult, diagnose.ConfidenceHigh, "duckduckgo returned 10 results for the 2 queries tried, but none is the requested product URL.")
	u, _ := url.Parse("https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/")
	resp, _ := repoService(t, pageFetcher{page: page, fail: fail}).Run(context.Background(), "r2", u)
	if resp.Status != StatusFailed || resp.Error.Code != diagnose.SearchNoResult || resp.Error.Stage != diagnose.StageFetch {
		t.Fatalf("got %s %+v", resp.Status, resp.Error)
	}
	ev := resp.Evidence.Search
	if ev == nil || ev.Engine != "duckduckgo" || len(ev.Queries) != 2 || len(ev.Closest) != 1 {
		t.Fatalf("evidence.search = %+v", ev)
	}
	if resp.Evidence.HTTPStatus != 0 || resp.Evidence.HTMLSample != nil {
		t.Errorf("the site was not contacted, so there is no page evidence: %+v", resp.Evidence)
	}
	if resp.Source == nil || resp.Source.SiteContacted || resp.Product != nil || resp.UnavailableFields != nil {
		t.Errorf("source %+v product %v unavailable %v", resp.Source, resp.Product, resp.UnavailableFields)
	}
	b, _ := json.Marshal(resp)
	if strings.Contains(string(b), `"unavailable_fields"`) || !strings.Contains(string(b), `"search":{"engine":"duckduckgo","cached":false`) {
		t.Errorf("JSON: %s", b)
	}
}

// http engine: whatever was found is returned, the rest is listed.
func TestUnavailableFieldsForFetchedPage(t *testing.T) {
	html, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sites", "fergusonhome", "duravit-starck-3-undermount-sink.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := &fetch.Page{Engine: fetch.EngineHTTP, Status: 200, Body: html}
	u, _ := url.Parse("https://www.fergusonhome.com/duravit-030549-0hole/s1666394")
	resp, _ := repoService(t, pageFetcher{page: page}).Run(context.Background(), "r3", u)
	if resp.Status != StatusSuccess {
		t.Fatalf("%+v", resp.Error)
	}
	// Everything the config extracts was found; only availability is not configured.
	if got := reasons(resp.UnavailableFields); !reflect.DeepEqual(got, map[string]string{"availability": ReasonNotConfigured}) {
		t.Errorf("unavailable_fields = %v", got)
	}
	if resp.Completeness != CompletenessPartial || resp.Source == nil || resp.Source.Engine != "http" || !resp.Source.SiteContacted {
		t.Errorf("completeness %q source %+v", resp.Completeness, resp.Source)
	}

	site := &siteconfig.Site{Pages: siteconfig.Pages{Product: siteconfig.ProductPage{
		Fields:       map[string]siteconfig.Field{"title": {}, "image_url": {}, "description": {}, "price_current": {}},
		Availability: siteconfig.Availability{OrderButton: "button"},
		Specs:        &siteconfig.Specs{},
	}}}
	price := int64(100)
	desc := "d"
	full := &Product{Title: "t", ImageURL: &desc, Description: &desc, Price: Price{Current: &price, State: PriceAvailable}, Specs: []Spec{{Name: "a", Value: "b"}}}
	if uf := unavailableFields(site, full, false); len(uf) != 0 || completeness(uf) != CompletenessFull {
		t.Errorf("full product: %v", uf)
	}
	missing := &Product{Price: Price{State: PriceUnknown}}
	got := reasons(unavailableFields(site, missing, false))
	want := map[string]string{"title": ReasonNotOnPage, "image_url": ReasonNotOnPage, "description": ReasonNotOnPage, "price": ReasonNotOnPage, "specs": ReasonNotOnPage}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missing on page = %v, want %v", got, want)
	}
	callForPrice := &Product{Title: "t", ImageURL: &desc, Description: &desc, Price: Price{State: PriceCallForPrice}, Specs: full.Specs}
	if uf := unavailableFields(site, callForPrice, false); len(uf) != 0 {
		t.Errorf("call for price is an answer, not a missing price: %v", uf)
	}
}

func TestSearchTitle(t *testing.T) {
	cases := []struct {
		in        string
		strip     []string
		want      string
		truncated bool
	}{
		{"Cabinet connector, Tab 18, without locking facility - Häfele", []string{" - Häfele"}, "Cabinet connector, Tab 18, without locking facility", false},
		{"Rear panel connector | online at HÄFELE", []string{" | online at HÄFELE", " | HÄFELE"}, "Rear panel connector", false},
		{"Universal-Eckverbinder, Kunststoff | häfele", []string{" | HÄFELE"}, "Universal-Eckverbinder, Kunststoff", false},
		{"Korpusverbinder, Häfele Ixconnect RC 40/22 T, mit Rastfunktion ...", nil, "Korpusverbinder, Häfele Ixconnect RC 40/22 T, mit Rastfunktion", true},
		{"Sink | Ferguson Home", []string{" | Ferguson Home"}, "Sink", false},
		{" | Ferguson Home", []string{" | Ferguson Home"}, "", false},
	}
	for _, c := range cases {
		got, tr := searchTitle(c.in, c.strip)
		if got != c.want || tr != c.truncated {
			t.Errorf("searchTitle(%q) = %q, %v; want %q, %v", c.in, got, tr, c.want, c.truncated)
		}
	}
}
