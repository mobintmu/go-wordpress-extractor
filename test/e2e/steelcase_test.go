package e2e

import "testing"

// TestSteelcase runs the real steelcase.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestSteelcase(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "steelcase",
		Domain:       "steelcase.com",
		Fixture:      "gesture-chair",
		Path:         "/seating/ergonomic-chairs/gesture",
		CategoryPath: "/seating/ergonomic-chairs",
		Variant:      "/seating/ergonomic-chairs/gesture-{v}",
		Title:        "Gesture",
		Price:        165900,
		State:        "available",
		Canonical:    "",
		Specs:        0,
		Features:     0,
		Layout:       []edit{{Old: "property=\"og:title\"", New: "property=\"og:renamed\""}},
	})
}
