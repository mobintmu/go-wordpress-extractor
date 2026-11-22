package e2e

import "testing"

// TestTurf runs the real turf.design config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestTurf(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "turf",
		Domain:       "turf.design",
		Fixture:      "slice",
		Path:         "/products/slice",
		CategoryPath: "/products",
		Variant:      "/products/slice-{v}",
		Title:        "Slice",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		Specs:        2,
		Features:     0,
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
