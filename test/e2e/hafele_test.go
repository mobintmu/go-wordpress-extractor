package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-wordpress/internal/fetch"
)

// hostHafele stands in for www.hafele.com.de in the fake shop.
const hostHafele = "hafele.test"

// hafelePath is the product path of the saved page. The live URL was copied
// from a search for "ixconnect" and carried @QueryTerm, Category, PageSize and
// PDP parameters; they only describe the search, so the bare path is used here.
const hafelePath = "/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/"

// hafeleCanonical is the link[rel=canonical] of that page. It becomes
// product.url because its host is one of the site's domains, even when the
// request goes to the fake shop's host.
const hafeleCanonical = "https://www.hafele.com.de" + hafelePath

// hafeleSite loads the real hafele.com.de config and points it at the fake
// shop. The real domains stay, so the canonical link is still accepted.
func hafeleSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "hafele.com.de.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostHafele)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// hafeleFixture returns the saved page and its expected product.
func hafeleFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "hafele")
	html, err := os.ReadFile(filepath.Join(dir, "ixconnect-uc-16-64.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "ixconnect-uc-16-64.json"))
	must(t, err)
	var fx struct {
		Expect struct {
			Product map[string]any `json:"product"`
		} `json:"expect"`
	}
	must(t, json.Unmarshal(raw, &fx))
	if fx.Expect.Product == nil {
		t.Fatal("fixture has no expected product")
	}
	return string(html), fx.Expect.Product
}

// hafeleReplace replaces every old, and fails if it is missing, so a variant can
// never silently test the unmodified page.
func hafeleReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// hafeleWarnings returns the warning codes of r, in response order.
func hafeleWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// hafeleSpecs returns the name/value pairs of r, in response order.
func hafeleSpecs(r *Result) [][2]string {
	items, _ := r.At("product.specs").([]any)
	out := make([][2]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		name, _ := m["name"].(string)
		value, _ := m["value"].(string)
		out = append(out, [2]string{name, value})
	}
	return out
}

// hafeleSpecsOf converts the fixture's specs to the same shape.
func hafeleSpecsOf(want map[string]any) [][2]string {
	items, _ := want["specs"].([]any)
	out := make([][2]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		name, _ := m["name"].(string)
		value, _ := m["value"].(string)
		out = append(out, [2]string{name, value})
	}
	return out
}

// TestHafele runs the real hafele.com.de config against its saved page and
// variants of it, through the full HTTP service. Häfele Germany is an
// Intershop store whose prices are hidden from anonymous visitors: the price
// block is in the markup but styled away, so the product comes back whole with
// price.state unknown, price.raw empty, currency EUR and one PRICE_MISSING
// warning. The specifications are section blocks rather than a table, and the
// page only has a description when its intro block is present.
func TestHafele(t *testing.T) {
	page, want := hafeleFixture(t)
	e := Start(t, Options{
		ExtraSites: map[string]map[string]any{hostHafele: hafeleSite(t)},
		// The saved page is about 370 KB; the suite's default limit is 256 KB.
		Fetch: func(o *fetch.Options) { o.MaxBodyBytes = 2 << 20 },
	})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostHafele, (&url.URL{Path: path}).EscapedPath())
	}

	s.Handle(hafelePath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(hafelePath), "request_id": "hafele-1"})
		r.Success(t)
		if r.Str("site_id") != "hafele" || r.Str("request_id") != "hafele-1" || r.Str("config_version") != "2026-11-12.1" {
			t.Errorf("site_id/request_id/config_version = %q/%q/%q",
				r.Str("site_id"), r.Str("request_id"), r.Str("config_version"))
		}
		for _, f := range []string{"title", "url", "description"} {
			exp, _ := want[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("%s = %.140q, want %.140q", f, got, exp)
			}
		}
		// The gallery path in the saved page is relative (/prod-live/...), so it
		// resolves against the host that was asked; only its path is fixed.
		wantImage, _ := want["image_url"].(string)
		gotImage := r.Str("product.image_url")
		gotURL, err := url.Parse(gotImage)
		if err != nil || !gotURL.IsAbs() {
			t.Errorf("image_url = %q, want an absolute URL", gotImage)
		} else if wantURL, _ := url.Parse(wantImage); gotURL.Path != wantURL.Path {
			t.Errorf("image_url path = %q, want %q", gotURL.Path, wantURL.Path)
		}
		wantPrice, _ := want["price"].(map[string]any)
		for _, f := range []string{"raw", "state", "currency"} {
			exp, _ := wantPrice[f].(string)
			if got := r.Str("product.price." + f); got != exp {
				t.Errorf("price.%s = %q, want %q", f, got, exp)
			}
		}
		if r.Has("product.price.current") && r.At("product.price.current") != nil {
			t.Errorf("price.current = %v, want null", r.At("product.price.current"))
		}
		if got := hafeleWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want one PRICE_MISSING", got)
		}
		gotSpecs, wantSpecs := hafeleSpecs(r), hafeleSpecsOf(want)
		if len(gotSpecs) != len(wantSpecs) {
			t.Fatalf("specs = %d pairs, want %d: %v", len(gotSpecs), len(wantSpecs), gotSpecs)
		}
		for i := range wantSpecs {
			if gotSpecs[i] != wantSpecs[i] {
				t.Errorf("specs[%d] = %v, want %v", i, gotSpecs[i], wantSpecs[i])
			}
		}
	})

	t.Run("the canonical link of the site is the product URL", func(t *testing.T) {
		if !strings.Contains(page, `rel="canonical" href="`+hafeleCanonical+`"`) {
			t.Fatal("the saved page no longer carries that canonical link")
		}
		r := e.Extract(productURL(hafelePath))
		r.Success(t)
		if got := r.Str("product.url"); got != hafeleCanonical {
			t.Errorf("product.url = %q, want the canonical %q", got, hafeleCanonical)
		}
	})

	t.Run("specifications are the named section blocks", func(t *testing.T) {
		r := e.Extract(productURL(hafelePath))
		r.Success(t)
		specs := hafeleSpecs(r)
		if len(specs) != 7 {
			t.Fatalf("specs = %d pairs, want 7: %v", len(specs), specs)
		}
		if specs[0] != [2]string{"Werkstoff", "Polycarbonat"} {
			t.Errorf("specs[0] = %v", specs[0])
		}
		if specs[6] != [2]string{"Maße (L x B x H)", "64 x 16 x 16 mm"} {
			t.Errorf("specs[6] = %v", specs[6])
		}
		for _, sp := range specs {
			if sp[0] == "" {
				t.Errorf("a section without a heading was reported: %v", sp)
			}
		}
	})

	// The page hides its prices from anonymous visitors. The words are in the
	// markup, so this pins the decision not to report them as a price: only a
	// page that really shows a price should ever produce one.
	t.Run("the hidden price block is not reported", func(t *testing.T) {
		if !strings.Contains(page, "Preis auf Anfrage") {
			t.Fatal("the saved page no longer contains 'Preis auf Anfrage'; revisit the price decision")
		}
		r := e.Extract(productURL(hafelePath))
		r.Success(t)
		if got := r.Str("product.price.raw"); got != "" {
			t.Errorf("price.raw = %q, want it empty", got)
		}
		if got := r.Str("product.price.state"); got != "unknown" {
			t.Errorf("price.state = %q, want unknown", got)
		}
		if got := r.Str("product.price.currency"); got != "EUR" {
			t.Errorf("price.currency = %q, want EUR", got)
		}
	})

	t.Run("a page without the intro block keeps the image and the specs", func(t *testing.T) {
		path := "/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01999991/"
		s.Handle(path, hafeleReplace(t, page, `class="productPropertiesIntro"`, `class="productPropertiesGuess"`))
		r := e.Extract(productURL(path))
		r.Success(t)
		if got := hafeleWarnings(r); len(got) != 2 || got[0] != "FIELD_MISSING" || got[1] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want FIELD_MISSING and PRICE_MISSING", got)
		}
		if r.Str("product.title") == "" || r.Str("product.image_url") == "" || len(hafeleSpecs(r)) != 7 {
			t.Errorf("title/image_url/specs = %q/%q/%d, want all three",
				r.Str("product.title"), r.Str("product.image_url"), len(hafeleSpecs(r)))
		}
	})

	t.Run("a page without the section blocks returns no specs", func(t *testing.T) {
		path := "/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01999992/"
		html := hafeleReplace(t, page,
			`class="productPropertiesSection details-product-body is-split-view"`,
			`class="productPropertiesDetail details-product-body is-split-view"`)
		s.Handle(path, html)
		r := e.Extract(productURL(path))
		r.Success(t)
		if got := hafeleSpecs(r); len(got) != 0 {
			t.Errorf("specs = %v, want none", got)
		}
		if got := hafeleWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want one PRICE_MISSING", got)
		}
		if r.Str("product.title") == "" {
			t.Error("title is empty")
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01999993/"
		s.Handle(path, hafeleReplace(t, page, "productDetailPage", "productHeroPage"))
		r := e.Extract(productURL(path))
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "hafele" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	// Product paths are /de/product/<slug>/P-<id>/; categories are
	// /de/products/... (plural) and info pages /de/info/..., so these never
	// reach the shop.
	t.Run("a category page is not fetched", func(t *testing.T) {
		path := "/de/products/moebelbeschlaege-wohnraumloesungen/verbinder-bodentraeger/verbinder/50/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Error("a non-product URL must not be fetched")
		}
	})

	t.Run("an info page is not fetched", func(t *testing.T) {
		path := "/de/info/service/kontakt/410/"
		r := e.Extract(productURL(path))
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Error("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := e.Extract(productURL("/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-99999999/"))
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})

	// The live host answers a client that runs no JavaScript with a Cloudflare
	// managed challenge, so this is what the service meets in production. The
	// fake shop cannot send the "cf-mitigated: challenge" header, so this pins
	// the markup path of the same check.
	t.Run("a Cloudflare challenge is reported as BOT_PROTECTION", func(t *testing.T) {
		path := "/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01999994/"
		s.Handle(path, `<!DOCTYPE html><html lang="en-US"><head><title>Just a moment...</title>`+
			`<script src="/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page/v1"></script></head>`+
			`<body><div class="main-wrapper"><h1>www.hafele.com.de</h1><p>Verifying you are human.</p></div></body></html>`)
		r := e.Extract(productURL(path))
		r.Failed(t, "BOT_PROTECTION", "page_check", "source_site", false)
		if r.Str("error.confidence") != "medium" {
			t.Errorf("markup detection is a heuristic: %s", r.Str("error.confidence"))
		}
	})
}
