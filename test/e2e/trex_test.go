package e2e

import "testing"

// TestTrex runs the real trex.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestTrex(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "trex",
		Domain:       "trex.com",
		Fixture:      "smart-decking-sample-kit",
		Path:         "/products/decking/the-smart-kit/",
		CategoryPath: "/products/decking/",
		Variant:      "/products/decking/the-smart-kit-{v}/",
		Title:        "The Smart Decking Sample Kit",
		Price:        500,
		State:        "available",
		Canonical:    "",
		Specs:        1,
		Features:     5,
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
