package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// hostMSI stands in for www.msisurfaces.com in the fake shop.
const hostMSI = "msisurfaces.test"

// msiPath is the product path of the saved page.
const msiPath = "/luxury-vinyl-planks/ashton-2/briven/"

// msiSite loads the real msisurfaces.com config and points it at the fake
// shop. The real domains stay, so the page's canonical link is honoured.
func msiSite(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "msisurfaces.com.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, hostMSI)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}

// msiFixture returns the saved live page and its expected product.
func msiFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	dir := filepath.Join(repoRoot, "testdata", "sites", "msisurfaces")
	html, err := os.ReadFile(filepath.Join(dir, "briven-lvt.html"))
	must(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "briven-lvt.json"))
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

// msiReplace replaces every old with new, and fails if old is missing.
func msiReplace(t *testing.T, html, old, new string) string {
	t.Helper()
	if !strings.Contains(html, old) {
		t.Fatalf("%q not found in the saved page", old)
	}
	return strings.ReplaceAll(html, old, new)
}

// msiWarnings returns the warning codes of r.
func msiWarnings(r *Result) []string {
	var codes []string
	ws, _ := r.At("warnings").([]any)
	for _, w := range ws {
		m, _ := w.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, c)
	}
	return codes
}

// msiSpec returns the first spec with name, or fails.
func msiSpec(t *testing.T, specs []any, name string) map[string]any {
	t.Helper()
	for _, s := range specs {
		m, _ := s.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	t.Fatalf("spec %q not found", name)
	return nil
}

// TestMSI runs the real msisurfaces.com config against its saved page and
// variants of it, through the full HTTP service. MSI sells through dealers:
// the saved page has no price element and no stock signal, so price is not
// configured and every product returns state "unknown" with a PRICE_MISSING
// warning.
func TestMSI(t *testing.T) {
	page, want := msiFixture(t)
	e := Start(t, Options{ExtraSites: map[string]map[string]any{hostMSI: msiSite(t)}})
	s := e.Shop
	productURL := func(path string) string {
		return s.URL(hostMSI, (&url.URL{Path: path}).EscapedPath())
	}
	extract := func(path, requestID string) *Result {
		return e.Post("/api/v1/extract", e.Key, map[string]any{"url": productURL(path), "request_id": requestID})
	}

	s.Handle(msiPath, page)

	t.Run("saved page matches the fixture", func(t *testing.T) {
		r := extract(msiPath, "msi-1")
		r.Success(t)
		if r.Str("site_id") != "msisurfaces" || r.Str("request_id") != "msi-1" || r.Str("config_version") != "2026-11-12.1" {
			t.Errorf("site_id/request_id/config_version = %q/%q/%q",
				r.Str("site_id"), r.Str("request_id"), r.Str("config_version"))
		}
		for _, f := range []string{"title", "url", "image_url", "description"} {
			exp, _ := want[f].(string)
			if got := r.Str("product." + f); got != exp {
				t.Errorf("product.%s = %.80q, want %.80q", f, got, exp)
			}
		}
		if got := r.At("product.price"); !reflect.DeepEqual(got, want["price"]) {
			t.Errorf("product.price = %v, want %v", got, want["price"])
		}
		if got := r.At("product.specs"); !reflect.DeepEqual(got, want["specs"]) {
			t.Errorf("product.specs does not match the fixture")
		}
		if got := msiWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want only PRICE_MISSING", got)
		}
	})

	t.Run("specifications", func(t *testing.T) {
		r := extract(msiPath, "msi-2")
		r.Success(t)
		specs, ok := r.At("product.specs").([]any)
		if !ok || len(specs) != 5 {
			t.Fatalf("specs = %v, want 5 pairs", r.At("product.specs"))
		}
		cases := []struct{ name, value string }{
			{"Primary Color(s)", "Brown"},
			{"Style", "Wood"},
			{"Wear Layer", "12MIL"},
			{"Environmental", "Greenguard Gold, FloorScore, USGBC LEED Certified"},
			{"Radiant Heating", "YES - See Installation Instructions for Details"},
		}
		for _, c := range cases {
			got := msiSpec(t, specs, c.name)
			if got["value"] != c.value {
				t.Errorf("spec %q = %v, want %q", c.name, got["value"], c.value)
			}
			// The page has no group headings around the specs.
			if g, ok := got["group"]; ok {
				t.Errorf("spec %q has group %v, want none", c.name, g)
			}
		}
		// "Additional Resources" only holds a disclaimer link; specs.exclude
		// drops it, so the pair must not appear.
		for _, s := range specs {
			if m, _ := s.(map[string]any); m["name"] == "Additional Resources" {
				t.Errorf("the disclaimer row must be excluded, got %v", m)
			}
		}
	})

	t.Run("product URL with a space is accepted", func(t *testing.T) {
		// Real MSI colors can have a space in the slug, for example
		// /luxury-vinyl-planks/ashton-2/benton blonde/. The requested path
		// is sent escaped but matched decoded; the page's canonical link
		// then wins for product.url.
		path := "/luxury-vinyl-planks/ashton-2/benton blonde/"
		s.Handle(path, page)
		r := extract(path, "msi-3")
		r.Success(t)
		if got := r.Str("product.title"); got != "Briven Luxury Vinyl Plank" {
			t.Errorf("title = %q", got)
		}
		if got := r.Str("product.url"); got != "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/" {
			t.Errorf("url = %q, want the page canonical", got)
		}
	})

	t.Run("specifications are optional", func(t *testing.T) {
		path := "/luxury-vinyl-planks/ashton-2/briven-sample/"
		s.Handle(path, msiReplace(t, page, `class="col-md-8 col-sm-12 specs-column"`,
			`class="col-md-8 col-sm-12 specs-list"`))
		r := extract(path, "msi-4")
		r.Success(t)
		if r.Has("product.specs") {
			t.Errorf("product.specs = %v, want absent", r.At("product.specs"))
		}
		if got := r.Str("product.title"); got != "Briven Luxury Vinyl Plank" {
			t.Errorf("title = %q", got)
		}
		if got := msiWarnings(r); len(got) != 1 || got[0] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want only PRICE_MISSING", got)
		}
	})

	t.Run("description is optional", func(t *testing.T) {
		path := "/luxury-vinyl-planks/ashton-2/briven-desc/"
		s.Handle(path, msiReplace(t, page, `class="padding-all product-highlights"`,
			`class="padding-all product-blurb"`))
		r := extract(path, "msi-5")
		r.Success(t)
		if !r.Has("product.description") || r.At("product.description") != nil {
			t.Errorf("description must be present and null, got %v", r.At("product.description"))
		}
		if got := msiWarnings(r); len(got) != 2 || got[0] != "FIELD_MISSING" || got[1] != "PRICE_MISSING" {
			t.Errorf("warnings = %v, want [FIELD_MISSING PRICE_MISSING]", got)
		}
	})

	t.Run("layout change is reported as SELECTOR_NOT_FOUND", func(t *testing.T) {
		path := "/luxury-vinyl-planks/ashton-2/briven-layout/"
		s.Handle(path, msiReplace(t, page, `class="main-wrapper product-page pb-3"`,
			`class="main-wrapper product-view pb-3"`))
		r := extract(path, "msi-6")
		r.Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)
		if r.Str("site_id") != "msisurfaces" {
			t.Errorf("site_id = %q", r.Str("site_id"))
		}
	})

	t.Run("collection page is not fetched", func(t *testing.T) {
		path := "/luxury-vinyl-planks/ashton-2/"
		r := extract(path, "msi-7")
		r.Failed(t, "NOT_PRODUCT_URL", "config", "client", false)
		if s.Hits(path) != 0 {
			t.Errorf("a non-product URL must not be fetched")
		}
	})

	t.Run("deleted product is HTTP_NOT_FOUND", func(t *testing.T) {
		r := extract("/luxury-vinyl-planks/ashton-2/not-a-color/", "msi-8")
		if r.Str("error.code") != "HTTP_NOT_FOUND" {
			t.Errorf("code = %s", r.Str("error.code"))
		}
	})
}
