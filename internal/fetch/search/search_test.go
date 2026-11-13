package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go-wordpress/internal/diagnose"
)

func fixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "..", "testdata", "search"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNormalizeAndSameURL(t *testing.T) {
	req := "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155"
	cases := []struct {
		result string
		ignore []string
		want   bool
	}{
		{"https://www.fergusonhome.com/duravit-030549-0hole/s1666394", []string{"uid"}, true},
		{"http://fergusonhome.com/duravit-030549-0hole/s1666394/", []string{"uid"}, true},
		{"https://WWW.FergusonHome.com/duravit-030549-0hole/s1666394?utm_source=x&gclid=1", []string{"uid"}, true},
		{"https://www.fergusonhome.com/duravit-030549-0hole/s1666394", nil, false}, // uid is a variant unless ignored
		{"https://www.fergusonhome.com/duravit-030543-0hole/s1666393", []string{"uid"}, false},
		{"https://www.fergusonhome.com/duravit-030549-0hole/s1666394#reviews", []string{"uid"}, true},
		{"not a url", nil, false},
	}
	for _, c := range cases {
		if got := SameURL(c.result, req, c.ignore); got != c.want {
			t.Errorf("SameURL(%q, ignore %v) = %v, want %v", c.result, c.ignore, got, c.want)
		}
	}
	if a, b := Normalize("https://x.test/p?b=2&a=1", nil), Normalize("https://x.test/p?a=1&b=2", nil); a != b {
		t.Errorf("query order matters: %q vs %q", a, b)
	}
	// /de/ and /en/ pages are different URLs.
	if SameURL("https://www.hafele.com.de/en/product/x/P-1/", "https://www.hafele.com.de/de/product/x/P-1/", nil) {
		t.Error("/en/ matched /de/")
	}
}

func TestSlugWordsAndQueries(t *testing.T) {
	for p, want := range map[string]string{
		"/duravit-030549-0hole/s1666394":                                        "duravit 030549 0hole",
		"/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/": "universalverbinder haefele ixconnect uc 16 64",
		"/tile/zellige-oat-100123456.html":                                      "zellige oat 100123456",
		"/":                                                                     "",
	} {
		if got := SlugWords(p); got != want {
			t.Errorf("SlugWords(%q) = %q, want %q", p, got, want)
		}
	}
	u, _ := url.Parse("https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155")
	got := Queries(u, []string{"site_slug", "url"})
	want := []string{"site:fergusonhome.com duravit 030549 0hole", `"fergusonhome.com/duravit-030549-0hole/s1666394"`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Queries = %q, want %q", got, want)
	}
	if got := Queries(u, []string{"url"}); len(got) != 1 || !strings.HasPrefix(got[0], `"`) {
		t.Errorf("url only = %q", got)
	}
}

// The saved pages are real DuckDuckGo responses from 2026-11-13.
func TestParseDuckDuckGoRealPages(t *testing.T) {
	res, f := ParseDuckDuckGo(fixture(t, "duckduckgo", "fergusonhome-site-slug.html"))
	if f != nil || len(res) != 10 {
		t.Fatalf("site_slug page: %d results, failure %v", len(res), f)
	}
	r := res[0]
	if r.Rank != 1 || r.URL != "https://www.fergusonhome.com/duravit-030549-0hole/s1666394" {
		t.Errorf("first result = %+v", r)
	}
	if !strings.HasPrefix(r.Title, "Duravit 0305490017 Starck 3") || !strings.HasSuffix(r.Title, "...") {
		t.Errorf("title = %q (DuckDuckGo cuts it and adds ...)", r.Title)
	}
	if !strings.HasPrefix(r.Snippet, "Duravit 030549-0HOLE Features:") || !strings.Contains(r.Snippet, `Overall Length: 20-7/8"`) {
		t.Errorf("snippet = %q", r.Snippet)
	}

	for _, name := range []string{"fergusonhome-quoted-url-no-results.html", "hafele-quoted-url-no-results.html"} {
		res, f := ParseDuckDuckGo(fixture(t, "duckduckgo", name))
		if f != nil || res == nil || len(res) != 0 {
			t.Errorf("%s: want an empty result list, got %v %v", name, res, f)
		}
	}

	res, f = ParseDuckDuckGo(fixture(t, "duckduckgo", "hafele-site-slug-no-match.html"))
	if f != nil || len(res) != 10 {
		t.Fatalf("hafele page: %d results, failure %v", len(res), f)
	}
	for _, r := range res {
		if SameURL(r.URL, "https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/", nil) {
			t.Errorf("hafele sample product unexpectedly in the results: %s", r.URL)
		}
	}
}

func TestParseDuckDuckGoEdgeCases(t *testing.T) {
	page := `<html><body>
<div class="result results_links web-result result--ad"><a class="result__a" href="https://ad.example/">Ad</a></div>
<div class="result results_links web-result"><a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fshop.test%2Fp%2F1&amp;rut=x">Old &amp; <b>redirect</b></a><a class="result__snippet">snip</a></div>
</body></html>`
	res, f := ParseDuckDuckGo([]byte(page))
	if f != nil || len(res) != 1 || res[0].URL != "https://shop.test/p/1" || res[0].Title != "Old & redirect" || res[0].Rank != 1 {
		t.Fatalf("got %+v %v", res, f)
	}
	_, f = ParseDuckDuckGo([]byte(`<html><title>DuckDuckGo</title><body><form id="challenge-form"></form></body></html>`))
	if f == nil || f.Code != diagnose.SearchProviderBlocked {
		t.Fatalf("anomaly page: %v", f)
	}
}

func TestDuckDuckGoHTTP(t *testing.T) {
	page := fixture(t, "duckduckgo", "fergusonhome-site-slug.html")
	var gotForm url.Values
	var gotUA string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm, gotUA = r.PostForm, r.UserAgent()
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		w.WriteHeader(status)
		_, _ = w.Write(page)
	}))
	defer srv.Close()
	d := NewDuckDuckGo(srv.Client(), "go-wordpress-extractor/test").WithEndpoint(srv.URL)
	res, f := d.Search(context.Background(), Query{Text: "site:fergusonhome.com duravit", Region: "us-en"})
	if f != nil || len(res) != 10 {
		t.Fatalf("%d results, %v", len(res), f)
	}
	if gotForm.Get("q") != "site:fergusonhome.com duravit" || gotForm.Get("kl") != "us-en" || gotUA != "go-wordpress-extractor/test" {
		t.Errorf("form %v, user agent %q", gotForm, gotUA)
	}
	for st, code := range map[int]string{202: diagnose.SearchProviderBlocked, 429: diagnose.SearchProviderBlocked, 503: diagnose.SearchProviderError, 404: diagnose.SearchProviderError} {
		status = st
		if _, f := d.Search(context.Background(), Query{Text: "x"}); f == nil || f.Code != code {
			t.Errorf("HTTP %d: %v, want %s", st, f, code)
		}
	}
}

func TestParseBrave(t *testing.T) {
	res, f := ParseBrave(fixture(t, "brave", "synthetic-fergusonhome.json"))
	if f != nil || len(res) != 2 {
		t.Fatalf("%d results, %v", len(res), f)
	}
	r := res[1]
	if r.Rank != 2 || r.URL != "https://www.fergusonhome.com/duravit-030549-0hole/s1666394" {
		t.Errorf("result = %+v", r)
	}
	if strings.Contains(r.Title, "<strong>") || !strings.Contains(r.Title, "Undermount Bathroom Sink | Ferguson Home") {
		t.Errorf("title = %q", r.Title)
	}
	if r.Snippet != "Duravit 030549-0HOLE Features: Covered under Duravit's 5 year warranty and 5 year commercial warranty Constructed of ceramic Undermount installation" {
		t.Errorf("snippet = %q", r.Snippet)
	}
	if res, f := ParseBrave([]byte(`{"type":"search"}`)); f != nil || res == nil || len(res) != 0 {
		t.Errorf("no web key: %v %v", res, f)
	}
	if _, f := ParseBrave([]byte(`<html>`)); f == nil || f.Code != diagnose.SearchProviderError {
		t.Errorf("not JSON: %v", f)
	}
}

func TestBraveHTTP(t *testing.T) {
	body := fixture(t, "brave", "synthetic-fergusonhome.json")
	var got *http.Request
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("X-RateLimit-Remaining", "1999")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	b := NewBrave(srv.Client(), "secret-key", "go-wordpress-extractor/test", nil).WithEndpoint(srv.URL)
	res, f := b.Search(context.Background(), Query{Text: "site:fergusonhome.com duravit", Region: "us-en"})
	if f != nil || len(res) != 2 {
		t.Fatalf("%d results, %v", len(res), f)
	}
	q := got.URL.Query()
	if got.Header.Get("X-Subscription-Token") != "secret-key" || q.Get("q") != "site:fergusonhome.com duravit" ||
		q.Get("country") != "US" || q.Get("search_lang") != "en" || q.Get("count") != "10" {
		t.Errorf("request %s headers %v", got.URL, got.Header)
	}
	if _, f := b.Search(context.Background(), Query{Text: "x", Region: "wt-wt"}); f != nil || got.URL.Query().Has("country") {
		t.Errorf("wt-wt must send no country: %s", got.URL)
	}
	for st, code := range map[int]string{
		401: diagnose.SearchProviderAuthFailed, 403: diagnose.SearchProviderAuthFailed, 429: diagnose.SearchQuotaExceeded,
		422: diagnose.SearchProviderError, 500: diagnose.SearchProviderError,
	} {
		status = st
		_, f := b.Search(context.Background(), Query{Text: "x"})
		if f == nil || f.Code != code {
			t.Errorf("HTTP %d: %v, want %s", st, f, code)
		}
		if f != nil && strings.Contains(f.RootCause, "secret-key") {
			t.Errorf("HTTP %d: the key leaked into the failure", st)
		}
	}
}
