package e2e

import "testing"

// TestWilsonart runs the real wilsonart.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestWilsonart(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "wilsonart",
		Domain:       "wilsonart.com",
		Fixture:      "beigewood-7850",
		Path:         "/beigewood-7850",
		CategoryPath: "/laminate/standard",
		Variant:      "/beigewood-{v}-7850",
		Title:        "Beigewood 7850",
		Price:        -1,
		State:        "unknown",
		Canonical:    "https://www.wilsonart.com/beigewood-7850",
		Specs:        2,
		Features:     0,
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "class=\"product-info-main\"", New: "class=\"renamed\""}},
	})
}
