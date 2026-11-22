package e2e

import "testing"

// TestShawFloors runs the real shawfloors.com config (http engine, nothing to
// mock) against its saved live page and variants of it. There is no sale,
// out-of-stock or call-for-price subtest: Shaw sells flooring through
// retailers, so the page has no stock signal, no cart for the flooring and no
// sale or call-for-price markup to base selectors on.
func TestShawFloors(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "shawfloors", Domain: "shawfloors.com", Fixture: "oak-grove-hazelwood-available",
		Path:         "/en-us/laminate/oak-grove-hazelwood/sl119-02039",
		Variant:      "/en-us/laminate/oak-grove-{v}/sl119-02039",
		CategoryPath: "/en-us/laminate",
		GonePath:     "/en-us/laminate/oak-grove-gone/sl119-99999",
		Engine:       "http",
		Title:        "Oak Grove - HAZELWOOD",
		Price:        499, // $4.99 SF
		State:        "available",
		Canonical:    "https://shawfloors.com/en-us/laminate/oak-grove-hazelwood/",
		SpecCount:    12,
		Specs: map[string][2]string{
			"Style Number":    {"Overview", "SL119"},
			"Color Name":      {"Overview", "HAZELWOOD"},
			"Width":           {"Construction", `7.55"`},
			"Area per Carton": {"Construction", "21.24 sq ft"},
		},
		NoSpecs: []edit{{Old: `class="specifications__content__item"`, New: `class="specifications-removed"`, All: true}},
		Layout:  []edit{{Old: `<h1 class="product-gallery__info-name"`, New: `<h1 class="product-name-renamed"`}},
	})
}
