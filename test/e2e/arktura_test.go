package e2e

import "testing"

// TestArktura runs the real arktura.com config (http engine, nothing to mock)
// against its saved product-line page. Arktura quotes panels through its
// architects, so there is no price, stock, sale or spec subtest.
func TestArktura(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "arktura", Domain: "arktura.com", Fixture: "vapor-exterior",
		Path:             "/vapor-exterior/",
		Variant:          "/ruelo-{v}/",
		CategoryPath:     "/Product/Vapor",
		GonePath:         "/gone-line/",
		Engine:           "http",
		Title:            "Vapor® Exterior",
		Price:            -1,
		State:            "unknown",
		Warnings:         []string{"PRICE_MISSING"},
		Canonical:        "https://arktura.com/vapor-exterior/",
		Layout:           []edit{{Old: `<div class="product-header-intro">`, New: `<div class="product-header-renamed">`}},
		MissingTitleCode: true,
	})
}
