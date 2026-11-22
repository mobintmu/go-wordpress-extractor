package e2e

import "testing"

// TestFleetwood runs the real fleetwoodusa.com config against its saved live page (http, nothing mocked).

func TestFleetwood(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "fleetwood",
		Domain:       "fleetwoodusa.com",
		Fixture:      "series-3070",
		Path:         "/products/3-series/series-3070/",
		CategoryPath: "/",
		GonePath:     "/products/gone-series",
		Engine:       "http",
		Title:        "Series 3070",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    25,
		Variant:      "/products/3-series/series-{v}/",
		Warnings:     []string{"PRICE_MISSING"},
		Specs:        map[string][2]string{"Glass": {"", "1″ Insulated Glass with Dark Bronze Spacer"}, "Frame": {"", "6063 T-6 Extruded Aluminum 1-15/16″ (per track) Deep Equal Leg (Block) Perimeter Frame"}},
	})
}
