package e2e

import "testing"

// TestVeneersupplies runs the real veneersupplies.com config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestVeneersupplies(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "veneersupplies",
		Domain:       "veneersupplies.com",
		Fixture:      "flat-cut-rosewood-bolivian",
		Path:         "/products/Flat-Cut-Rosewood-Bolivian-Veneer-Sheet--55-x-71.html",
		CategoryPath: "/categories/Raw__Wood__Veneer/Wood__Veneer__H__-__M/Koa__Wood__Veneer/",
		Variant:      "/products/Flat-Cut-Rosewood-{v}.html",
		Title:        "Flat Cut Rosewood (Bolivian) Veneer Sheet 5.5\" x 71\"",
		Price:        1654,
		State:        "available",
		Canonical:    "",
		Specs:        11,
		Features:     3,
		Layout:       []edit{{Old: "id=\"Container\"", New: "id=\"Renamed\""}},
	})
}
