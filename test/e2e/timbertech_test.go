package e2e

import "testing"

// TestTimbertech runs the real timbertech.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestTimbertech(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "timbertech",
		Domain:       "timbertech.com",
		Fixture:      "legacy-collection",
		Path:         "/product/pro-legacy-collection/",
		CategoryPath: "/product",
		Variant:      "/product/pro-legacy-collection-{v}/",
		Title:        "Legacy Collection",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		Specs:        6,
		Features:     0,
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
