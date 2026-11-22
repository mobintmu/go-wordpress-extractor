package e2e

import "testing"

// TestDrytreat runs the real drytreat.com config against its saved live page (http, nothing mocked).

func TestDrytreat(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "drytreat",
		Domain:       "drytreat.com",
		Fixture:      "stain-proof-waterborne",
		Path:         "/sealers/residential/stain-proof-waterborne/",
		CategoryPath: "/sealers/residential/",
		GonePath:     "/sealers/residential/gone-sealer",
		Engine:       "browser",
		Title:        "Premium Waterborne Impregnating Sealer",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    0,
		Variant:      "/sealers/residential/stain-proof-{v}/",
		Warnings:     []string{"PRICE_MISSING"},
	})
}
