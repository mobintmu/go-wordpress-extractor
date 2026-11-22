package e2e

import "testing"

// TestKichler runs the real kichler.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestKichler(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "kichler",
		Domain:       "kichler.com",
		Fixture:      "2-fixture-mount-centennial",
		Path:         "/2-fixture-mount-in-centennial-brass-15515CBR",
		CategoryPath: "/",
		Variant:      "/2-fixture-mount-in-{v}-15515CBR",
		Title:        "2 Fixture Mount in Centennial Brass",
		Price:        2299,
		State:        "available",
		Canonical:    "",
		Specs:        6,
		Features:     1,
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
