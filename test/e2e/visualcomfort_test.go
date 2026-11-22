package e2e

import "testing"

// TestVisualcomfort runs the real visualcomfort.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestVisualcomfort(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "visualcomfort",
		Domain:       "visualcomfort.com",
		Fixture:      "charleston-medium-lantern",
		Path:         "/us/p/charleston-medium-lantern-5291004",
		CategoryPath: "/us/c/best-sellers",
		Variant:      "/us/p/charleston-medium-lantern-{v}",
		Title:        "Charleston Medium Lantern",
		Price:        72165,
		State:        "available",
		Canonical:    "",
		Specs:        29,
		Features:     0,
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
