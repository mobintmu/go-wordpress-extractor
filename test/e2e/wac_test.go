package e2e

import "testing"

// TestWac runs the real waclighting.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestWac(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "wac",
		Domain:       "waclighting.com",
		Fixture:      "axel-wall-light",
		Path:         "/product/axel/",
		CategoryPath: "/category-landing",
		Variant:      "/product/axel-{v}/",
		Title:        "Axel Wall Light 3000K",
		Price:        -1,
		State:        "call_for_price",
		Canonical:    "",
		Specs:        12,
		Features:     12,
		Layout:       []edit{{Old: "class=\"bt\"", New: "class=\"renamed\""}},
	})
}
