package e2e

import "testing"

// TestMarble runs the real marble.com config, which selects the paid
// ai-crawler-avalai engine, against its saved live page. AvalAI is mocked:
// the fake LLM replays the answer recorded on 2026-11-19. Marble.com quotes
// countertops per project, so there is no price, sale, stock or
// call-for-price markup to vary.
func TestMarble(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID: "marble", Domain: "marble.com", Fixture: "gran-perla-granite",
		Path:         "/granite-countertops/gran-perla/1179",
		Variant:      "/granite-countertops/gran-perla-{v}/9999",
		CategoryPath: "/granite-countertops",
		Title:        "Gran Perla Granite",
		Price:        -1,
		State:        "unknown",
		Canonical:    "https://marble.com/granite-countertops/gran-perla/1179",
		Specs:        4,
		Features:     15,
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: `<h1 class="page-title material-name-title">`, New: `<h1 class="page-title renamed">`}},
	})
}
