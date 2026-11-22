package e2e

import "testing"

// TestABCWorldwideStone runs the real abcworldwidestone.com config (http
// engine, nothing to mock) against its saved live page. ABC Stone is a slab
// supplier with no price, cart or stock text, so
// there is no sale, out-of-stock or call-for-price subtest.
func TestABCWorldwideStone(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "abcworldwidestone", Domain: "abcworldwidestone.com", Fixture: "gris-macael-dusted",
		Path:         "/portfolio/gris-macael-dusted/",
		CategoryPath: "/portfolio/",
		GonePath:     "/portfolio/gone-stone/",
		Engine:       "http",
		Title:        "Gris Macael Dusted",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		Canonical:    "https://abcworldwidestone.com/portfolio/gris-macael-dusted/",
		SpecCount:    3,
		Specs: map[string][2]string{
			"Dimensions": {"", "As each stone is unique, sizes will vary upon availability. Average slab size is 80” x 40” – 120” x 70”. Tiles or special sizes may be available upon request."},
		},
		NoSpecs: []edit{{Old: `<div class="portfolio_single_outer">`, New: `<div class="portfolio_single_removed">`}},
		Layout:  []edit{{Old: `<div class="portfolio_content">`, New: `<div class="portfolio_renamed">`}},
	})
}
