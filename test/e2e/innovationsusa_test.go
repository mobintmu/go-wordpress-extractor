package e2e

import "testing"

// TestInnovationsUSA runs the real innovationsusa.com config (http engine,
// nothing to mock) against its saved live page. Prices and the cart are
// behind the trade sign-in, so there is no sale, out-of-stock or
// call-for-price subtest.
func TestInnovationsUSA(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "innovationsusa", Domain: "innovationsusa.com", Fixture: "antwerp-linen",
		Path:         "/item/antwerp",
		CategoryPath: "/product/all-wallcovering",
		GonePath:     "/item/gone",
		Engine:       "http",
		Title:        "Antwerp",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		Canonical:    "https://www.innovationsusa.com/item/antwerp",
		SpecCount:    9,
		Specs: map[string][2]string{
			"Composition":  {"", "97% Linen/3% Polyester"},
			"Total Weight": {"", "13 oz/lineal yd (369 gsm)"},
			"Width":        {"", `43" (109 cm)`},
			"Fire Rating":  {"", "ASTM E84 - Class A"},
		},
		NoSpecs: []edit{{Old: `<ul class="skuSpec m-0">`, New: `<ul class="skuRemoved m-0">`}},
		Layout:  []edit{{Old: `<h1 class="product-title text-uppercase m-0">`, New: `<h1 class="renamed text-uppercase m-0">`}},
	})
}
