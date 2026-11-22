package e2e

import "testing"

// TestCosmosSurfaces runs the real cosmossurfaces.com config (http engine,
// nothing to mock) against its saved live page, an Angular shell whose only
// product data is the <head> metadata. There is no price, stock or specs
// markup, so there is no sale, out-of-stock, call-for-price or specs subtest.
func TestCosmosSurfaces(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "cosmossurfaces", Domain: "cosmossurfaces.com", Fixture: "absolute-black-premium-3cm",
		Path:         "/washington/granite/absolute-black-premium-3cm",
		Variant:      "/washington/granite/absolute-black-{v}",
		CategoryPath: "/washington/granite",
		GonePath:     "/washington/granite/gone-3cm",
		Engine:       "http",
		Title:        "Everyday Use Absolute Black Premium 3CM | Granite | Washington, DC",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		Canonical:    "https://www.cosmossurfaces.com/washington/granite/absolute-black-premium-3cm",
		Layout:       []edit{{Old: `type="application/ld+json"`, New: `type="application/json"`, All: true}},
	})
}
