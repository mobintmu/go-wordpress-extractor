package e2e

import "testing"

// TestMiracle runs the real miracle-sealants.com config against its saved live page (http, nothing mocked).

func TestMiracle(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "miracle",
		Domain:       "miracle-sealants.com",
		Fixture:      "porcelain-ceramic-tile-cleaner",
		Path:         "/miracle-sealants-porcelain-ceramic-tile-cleaner/",
		CategoryPath: "/product-category/cleaners/",
		GonePath:     "/miracle-sealants-gone-cleaner/",
		Engine:       "http",
		Title:        "Porcelain & Ceramic Tile Scrubbing Cleaner",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    0,
		Variant:      "/miracle-sealants-tile-cleaner-{v}/",
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
