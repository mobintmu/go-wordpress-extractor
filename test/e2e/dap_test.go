package e2e

import "testing"

// TestDap runs the real dap.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestDap(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "dap",
		Domain:       "dap.com",
		Fixture:      "weldwood-landau-1145",
		Path:         "/products/adhesives/weldwood-landau-top-and-trim-1145-high-heat-resistant-contact-cement",
		CategoryPath: "/products/adhesives",
		Variant:      "/products/adhesives/weldwood-{v}-1145-contact-cement",
		Title:        "Weldwood Landau Top & Trim 1145 High Heat Resistant Contact Cement",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		Specs:        8,
		Features:     9,
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "<h1 class=\"sd:sr-only", New: "<h2 class=\"sd:sr-only"}},
	})
}
