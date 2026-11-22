package e2e

import "testing"

// TestMarvin runs the real marvin.com config against its saved live page (http, nothing mocked).

func TestMarvin(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "marvin",
		Domain:       "marvin.com",
		Fixture:      "elevate-double-hung",
		Path:         "/products/windows/double-hung/elevate-double-hung-window",
		CategoryPath: "/products/windows/double-hung",
		GonePath:     "/products/windows/double-hung/gone-window",
		Engine:       "http",
		Title:        "Elevate Double Hung Window",
		Price:        -1,
		State:        "unknown",
		Canonical:    "https://www.marvin.com/products/windows/double-hung/elevate-double-hung-window",
		SpecCount:    0,
		Variant:      "/products/windows/double-hung/elevate-{v}-window",
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "<h1", New: "<h2"}},
	})
}
