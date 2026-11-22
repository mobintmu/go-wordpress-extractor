package e2e

import "testing"

// TestStoneLandUSA runs the real stonelandusa.com config, which selects the
// paid ai-crawler-avalai engine because the slab price is only in the page's
// JSON-LD. AvalAI is mocked: the fake LLM replays the answer recorded on
// 2026-11-19 ($1,821.75, which also guards the fixed parsing of amounts
// over 999 written without thousands separators).
func TestStoneLandUSA(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID: "stonelandusa", Domain: "stonelandusa.com", Fixture: "grigio-carnico-slab",
		Path:         "/product/grigio-carnico-sl-614/",
		Variant:      "/product/grigio-carnico-{v}/",
		CategoryPath: "/product-category/marble/",
		Title:        "Grigio Carnico SL 614",
		Price:        182175,
		State:        "available",
		Canonical:    "https://stonelandusa.com/product/grigio-carnico-sl-614/",
		Specs:        4,
		Layout:       []edit{{Old: `<div class="product-data-table">`, New: `<div class="product-data-renamed">`}},
	})
}
