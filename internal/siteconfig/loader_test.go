package siteconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMerge(t *testing.T) {
	base := map[string]any{"a": map[string]any{"x": 1, "y": 2}, "list": []any{1, 2}, "keep": true}
	over := map[string]any{"a": map[string]any{"y": 3}, "list": []any{9}}
	got := Merge(base, over)
	want := map[string]any{"a": map[string]any{"x": 1, "y": 3}, "list": []any{9}, "keep": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge = %v", got)
	}
	if base["a"].(map[string]any)["y"] != 2 {
		t.Fatal("Merge modified its input")
	}
}

func TestRepositoryConfigsAreValid(t *testing.T) {
	entries, err := LoadDir(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("want at least 2 sites, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Err != nil {
			t.Errorf("%s: %v", e.File, e.Err)
		}
	}
	snap := NewSnapshot(entries)
	b := snap.Lookup("WWW.BADOMJIP.COM.")
	if b == nil || b.SiteID != "badomjip" {
		t.Fatalf("www domain lookup failed")
	}
	s := b.Site
	if s.Pages.Product.Root != "div.product.type-product" || s.Fetch.TimeoutSeconds != 30 || !s.RespectRobots() {
		t.Errorf("template not merged: %+v", s.Pages.Product)
	}
	if !strings.Contains(s.Pages.Product.Fields["image_url"].Selector, "figure.woocommerce-product-gallery__image") {
		t.Errorf("woodmart override missing")
	}
	if !s.MatchesProductPath("/product/x/") || s.MatchesProductPath("/product-category/nuts/") {
		t.Errorf("match patterns wrong")
	}
	h := snap.Lookup("hosseinibrothers.ir").Site
	for path, want := range map[string]bool{
		"/کشمش/کشمش-پلویی-آفتابی":   true,  // product: /<category>/<product>
		"/آجیل/بادام/بادام-درختی/":  false, // category: trailing slash
		"/content/about-us.html":    false, // info page
		"/1059-large_default/x.jpg": false, // image
		"/contact-us":               false, // single segment
	} {
		if got := h.MatchesProductPath(path); got != want {
			t.Errorf("hosseinibrothers MatchesProductPath(%q) = %v, want %v", path, got, want)
		}
	}
}

type fixture map[string]string

func writeDir(t *testing.T, files fixture) string {
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func site(t *testing.T, m map[string]any) string {
	base := map[string]any{
		"schema_version": 1, "site_id": "s", "version": "1", "domains": []string{"s.test"},
		"pages": map[string]any{"product": map[string]any{
			"match": []string{"^/p/"}, "root": "main",
			"fields": map[string]any{"title": map[string]any{"selector": "h1"}},
		}},
	}
	for k, v := range m {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return string(b)
}

func load(t *testing.T, files fixture) []*Entry {
	entries, err := LoadDir(writeDir(t, files))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestDefaults(t *testing.T) {
	e := load(t, fixture{"sites/s.test.json": site(t, nil), "templates/.keep.json": "{}"})[0]
	if e.Err != nil {
		t.Fatal(e.Err)
	}
	s := e.Site
	if s.Fetch.UserAgent != "ProductExtractor/1.0" || *s.Fetch.DelayMS != 1000 || s.Fetch.TimeoutSeconds != 30 ||
		s.Retries() != 2 || !s.RespectRobots() || s.Locale.PriceUnit != "toman" || !s.IsEnabled() ||
		!reflect.DeepEqual(s.Validation.RequiredFields, []string{"title"}) || s.Pages.Product.Fields["title"].Pick != "first" {
		t.Fatalf("defaults not applied: %+v", s)
	}
}

func TestInvalidSites(t *testing.T) {
	cases := map[string]struct {
		file string
		want string
	}{
		"bad json":                  {"{", "invalid JSON"},
		"unknown field":             {site(t, map[string]any{"colour": "red"}), "unknown field"},
		"missing template":          {site(t, map[string]any{"extends": "nope"}), `template "nope" not found`},
		"bad selector":              {site(t, map[string]any{"pages": map[string]any{"product": map[string]any{"match": []string{"^/"}, "root": "div[[", "fields": map[string]any{"title": map[string]any{"selector": "h1"}}}}}), "invalid selector"},
		"bad regex":                 {site(t, map[string]any{"pages": map[string]any{"product": map[string]any{"match": []string{"("}, "root": "main", "fields": map[string]any{"title": map[string]any{"selector": "h1"}}}}}), "match[0]"},
		"unknown field name":        {site(t, map[string]any{"pages": map[string]any{"product": map[string]any{"match": []string{"^/"}, "root": "main", "fields": map[string]any{"sku": map[string]any{"selector": "x"}}}}}), "not a known field"},
		"required without selector": {site(t, map[string]any{"validation": map[string]any{"required_fields": []string{"image_url"}}}), "has no selector"},
		"bad pick":                  {site(t, map[string]any{"pages": map[string]any{"product": map[string]any{"match": []string{"^/"}, "root": "main", "fields": map[string]any{"title": map[string]any{"selector": "h1", "pick": "all"}}}}}), "pick must be"},
		"bad unit":                  {site(t, map[string]any{"locale": map[string]any{"price_unit": "usd"}}), "price_unit"},
		"bad domain":                {site(t, map[string]any{"domains": []string{"not a host"}}), "not a valid hostname"},
		"bad ranges":                {site(t, map[string]any{"fetch": map[string]any{"timeout_seconds": 500, "retries": 9, "delay_ms": -1}}), "timeout_seconds"},
	}
	for name, c := range cases {
		e := load(t, fixture{"sites/broken.test.json": c.file, "templates/t.json": "{}"})[0]
		if e.Err == nil || !strings.Contains(e.Err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, e.Err, c.want)
		}
		if e.Status() != "invalid" || len(e.Domains) == 0 {
			t.Errorf("%s: an invalid file must still claim a domain", name)
		}
	}
}

func TestExtendsChainAndCycle(t *testing.T) {
	entries := load(t, fixture{
		"templates/base.json":  `{"fetch":{"retries":1,"timeout_seconds":7},"locale":{"call_for_price_words":["a","b"]}}`,
		"templates/theme.json": `{"extends":"base","fetch":{"retries":3}}`,
		"templates/loop1.json": `{"extends":"loop2"}`,
		"templates/loop2.json": `{"extends":"loop1"}`,
		"sites/a.test.json":    site(t, map[string]any{"extends": "theme", "locale": map[string]any{"call_for_price_words": []string{"c"}}}),
		"sites/b.test.json":    strings.Replace(site(t, map[string]any{"extends": "loop1"}), "s.test", "b.test", 1),
	})
	a, b := entries[0], entries[1]
	if a.Err != nil || a.Site.Retries() != 3 || a.Site.Fetch.TimeoutSeconds != 7 || !reflect.DeepEqual(a.Site.Locale.CallForPriceWords, []string{"c"}) {
		t.Fatalf("chain merge: %v %+v", a.Err, a.Site)
	}
	if b.Err == nil || !strings.Contains(b.Err.Error(), "cycle") {
		t.Fatalf("cycle not detected: %v", b.Err)
	}
}

func TestDuplicateDomains(t *testing.T) {
	entries := load(t, fixture{"sites/a.json": site(t, nil), "sites/b.json": site(t, nil)})
	for _, e := range entries {
		if e.Err == nil || !strings.Contains(e.Err.Error(), "claimed by both") {
			t.Fatalf("duplicate domain not reported: %v", e.Err)
		}
	}
}

func TestAttrList(t *testing.T) {
	var f Field
	if err := json.Unmarshal([]byte(`{"selector":"img","attr":"src"}`), &f); err != nil || !reflect.DeepEqual([]string(f.Attr), []string{"src"}) {
		t.Fatalf("string attr: %v %v", f.Attr, err)
	}
	if err := json.Unmarshal([]byte(`{"selector":"img","attr":["data-src","src"]}`), &f); err != nil || len(f.Attr) != 2 {
		t.Fatalf("list attr: %v %v", f.Attr, err)
	}
	if err := json.Unmarshal([]byte(`{"selector":"img","attr":5}`), &f); err == nil {
		t.Fatal("number attr accepted")
	}
}
