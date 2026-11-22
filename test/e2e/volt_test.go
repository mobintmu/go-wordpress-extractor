package e2e

import "testing"

// TestVolt runs the real voltlighting.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestVolt(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "volt",
		Domain:       "voltlighting.com",
		Fixture:      "mr11-decorative-rock-light",
		Path:         "/mr11-decorative-rock-light",
		CategoryPath: "/shop/line-voltage/120v-path-lights",
		Variant:      "/mr11-decorative-rock-light-{v}",
		Title:        "Decorative Rock Light",
		Price:        3399,
		State:        "available",
		Canonical:    "",
		Specs:        17,
		Features:     5,
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
