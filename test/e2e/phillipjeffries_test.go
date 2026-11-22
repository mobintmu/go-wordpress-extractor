package e2e

import "testing"

// TestPhillipJeffries runs the real phillipjeffries.com config (http engine,
// nothing to mock) against its saved live page. Prices, stock and spec
// values are shown only to signed-in trade accounts, so there is no sale,
// out-of-stock, call-for-price or specs subtest, and the page has no
// canonical link.
func TestPhillipJeffries(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "phillipjeffries", Domain: "phillipjeffries.com", Fixture: "meta-metallic-gold-46400",
		Path:         "/shop/CARD-464/46400",
		Variant:      "/shop/CARD-464/9{n}",
		CategoryPath: "/shop/categories",
		GonePath:     "/shop/CARD-464/99999",
		Engine:       "http",
		Title:        "Meta Metallic in Gold Wallcovering - 46400",
		Price:        -1,
		State:        "unknown",
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: `<h1 hidden="true">`, New: `<h2 hidden="true">`}},
	})
}
