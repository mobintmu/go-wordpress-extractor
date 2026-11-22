package e2e

import "testing"

// TestLaminart runs the real laminart.com config against its saved live page (http, nothing mocked).

func TestLaminart(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "laminart",
		Domain:       "laminart.com",
		Fixture:      "coal-706",
		Path:         "/coal-706",
		CategoryPath: "/superorganic",
		GonePath:     "/gone-000000",
		Engine:       "http",
		Title:        "Coal Buffalo 706",
		Price:        -1,
		State:        "unknown",
		Canonical:    "https://www.laminart.com/coal-706",
		SpecCount:    2,
		Variant:      "/coal-{v}-{n}",
		Warnings:     []string{"PRICE_MISSING"},
		Specs:        map[string][2]string{"Translucent": {"", "No"}, "Vertical use only": {"", "No"}},
		Layout:       []edit{{Old: "class=\"product-info-main\"", New: "class=\"renamed\""}},
	})
}
