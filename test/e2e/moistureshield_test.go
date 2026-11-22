package e2e

import "testing"

// TestMoistureshield runs the real moistureshield.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestMoistureshield(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "moistureshield",
		Domain:       "moistureshield.com",
		Fixture:      "vision-decking",
		Path:         "/products/composite-decking/vision/",
		CategoryPath: "/products/composite-decking/",
		Variant:      "/products/composite-decking/vision-{v}/",
		Title:        "Vision®",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		Specs:        3,
		Features:     7,
		Warnings:     []string{"PRICE_MISSING"},
	})
}
