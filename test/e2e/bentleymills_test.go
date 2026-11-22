package e2e

import "testing"

// TestBentleyMills runs the real bentleymills.com config (http engine,
// nothing to mock) against its saved live page and variants of it. Bentley
// Mills is a commercial mill with no price, cart or stock text on the page,
// so there is no sale, out-of-stock or call-for-price subtest, and no
// canonical link to check.
func TestBentleyMills(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "bentleymills", Domain: "bentleymills.com", Fixture: "agnona-broadloom",
		Path:         "/product/4-agnona/",
		Variant:      "/product/900{n}-agnona-{v}/",
		CategoryPath: "/products/carpet/",
		GonePath:     "/product/9999-gone/",
		Engine:       "http",
		Title:        "Agnona",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		SpecCount:    24,
		Specs: map[string][2]string{
			"Product Type":  {"", "Carpet Broadloom"},
			"Construction":  {"", "Tufted Tip-Sheared"},
			"Machine Gauge": {"", "1/12 in (47.2 ends/10 cm)"},
			"Fiber":         {"", "Bentley Premium™ Type 6,6 Nylon"},
		},
		NoSpecs: []edit{{Old: `id="specifications-info"`, New: `id="specifications-removed"`}},
		Layout:  []edit{{Old: `<div class="product-title-section">`, New: `<div class="product-title-renamed">`}},
	})
}
