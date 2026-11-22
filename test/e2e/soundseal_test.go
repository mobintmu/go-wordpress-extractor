package e2e

import "testing"

// TestSoundseal runs the real soundseal.com config against its saved live page (http, nothing mocked).

func TestSoundseal(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "soundseal",
		Domain:       "soundseal.com",
		Fixture:      "s-4200-tuned-absorber",
		Path:         "/architectural-interiors/products/acoustical-wall-panels/s-4200-tuned-absorbers",
		CategoryPath: "/architectural-interiors/products",
		GonePath:     "/architectural-interiors/products/gone/panel",
		Engine:       "http",
		Title:        "S-4200 Tuned Absorber/Diffuser Panel",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    0,
		Variant:      "/architectural-interiors/products/acoustical-wall-panels/s-{v}-panel",
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
