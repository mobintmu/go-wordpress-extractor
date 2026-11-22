package e2e

import "testing"

// TestEmser runs the real emser.com config against the page Chrome rendered.
// The config selects the browser engine; the suite has no Chrome, so the
// runner serves the rendered page through the http engine and checks that
// the config still says browser. Emser sells through distributors (the only
// price is the sample chip's), so there is no sale, out-of-stock or
// call-for-price subtest.
func TestEmser(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "emser", Domain: "emser.com", Fixture: "anvaya-poppy-12x12",
		Path:         "/product/anvaya-f44anvapo1212m",
		CategoryPath: "/catalog/porcelain",
		GonePath:     "/product/gone-f44gone1212m",
		Engine:       "browser",
		Title:        "ANVAYA - 12X12, POPPY, MATTE",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		Canonical:    "https://www.emser.com/product/anvaya-f44anvapo1212m",
		SpecCount:    41,
		Specs: map[string][2]string{
			"Grout Joint Width (Inches)": {"Product Specifications", `1/8"`},
			"Material":                   {"Technical Specifications", "PORCELAIN"},
			"Nominal Size":               {"Technical Specifications", "12X12"},
			"UV Stable":                  {"Usage", "YES"},
		},
		NoSpecs: []edit{{Old: `data-test-selector="attributesGroupWrapper"`, New: `data-test-selector="attributesRemoved"`, All: true}},
		Layout:  []edit{{Old: `data-test-selector="productDetails_productDescription"`, New: `data-test-selector="productDetails_renamed"`}},
	})
}
