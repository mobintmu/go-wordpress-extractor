package e2e

import "testing"

// TestSchluter runs the real schluter.com config against its saved live page (http, nothing mocked).

func TestSchluter(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "schluter",
		Domain:       "schluter.com",
		Fixture:      "ditra-ps",
		Path:         "/schluter-us/en_US/Membranes/Uncoupling-(DITRA)/Schluter%C2%AE-DITRA-PS/p/DITRA_PS",
		CategoryPath: "/schluter-us/en_US/Membranes/Uncoupling-(DITRA)/c/M-U",
		GonePath:     "/schluter-us/en_US/Membranes/gone/p/GONE",
		Engine:       "http",
		Title:        "Schluter®-DITRA-PS",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    0,
		Variant:      "/schluter-us/en_US/Membranes/Uncoupling-(DITRA)/Schluter-DITRA-{v}/p/DITRA_{v}",
		Warnings:     []string{"PRICE_MISSING"},
	})
}
