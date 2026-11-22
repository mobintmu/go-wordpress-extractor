package e2e

import "testing"

// Test1000Bulbs runs the real 1000bulbs.com config against its saved live page (http, nothing mocked).

func Test1000Bulbs(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "1000bulbs",
		Domain:       "1000bulbs.com",
		Fixture:      "stage-studio-t6",
		Path:         "/product/37/STAG-FEP.html",
		CategoryPath: "/lighting/stage-studio.html",
		GonePath:     "/product/99999/GONE.html",
		Engine:       "http",
		Title:        "Stage and Studio - T6",
		Price:        2155,
		State:        "available",
		Canonical:    "https://www.1000bulbs.com/product/37/STAG-FEP.html",
		SpecCount:    16,
		Variant:      "/product/900{n}/STAG-{v}.html",
		Specs:        map[string][2]string{"Brand": {"", "PLT"}, "MPN (Part No.)": {"", "FEP"}, "Wattage": {"", "1,000 Watt"}, "Lumens (Mean)": {"", "26500"}},
		Layout:       []edit{{Old: "class='pdp-v3 pdp-v3--single-price'", New: "class='renamed'"}},
	})
}
