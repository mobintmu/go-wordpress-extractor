package e2e

import "testing"

// TestWalkerZanger runs the real walkerzanger.com config (http engine,
// nothing to mock) against its saved live page and variants of it. Walker
// Zanger shows no price, cart or stock text to an anonymous visitor, so there
// is no sale, out-of-stock or call-for-price subtest.
func TestWalkerZanger(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "walkerzanger", Domain: "walkerzanger.com", Fixture: "adirondack-grey-coping",
		Path:         "/product/18x48x1-1-2-adirondack-grey-coping-natural/",
		CategoryPath: "/product-category/tile/collections/limestone-tiles/",
		GonePath:     "/product/gone-coping/",
		Engine:       "http",
		Title:        "Adirondack Grey Coping Natural 18x48x1-1/2",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		Canonical:    "https://www.walkerzanger.com/product/18x48x1-1-2-adirondack-grey-coping-natural/",
		SpecCount:    5,
		Specs: map[string][2]string{
			"Material":  {"", "Limestone"},
			"Color":     {"", "Adirondack Grey"},
			"Size":      {"", `18" x 48"`},
			"Thickness": {"", "1 -1/4 Inch"},
		},
		NoSpecs: []edit{{Old: `class="detail-box"`, New: `class="detail-removed"`, All: true}},
		Layout:  []edit{{Old: `<h1 class="product-detail-name">`, New: `<h1 class="product-renamed">`}},
	})
}
