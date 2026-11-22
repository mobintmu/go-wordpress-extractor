package e2e

import "testing"

// TestLithofin runs the real lithofin.com config against its saved live page (http, nothing mocked).

func TestLithofin(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "lithofin",
		Domain:       "lithofin.com",
		Fixture:      "mn-easy-clean",
		Path:         "/en-gb/products/natural-and-artificial-stone/lithofin-mn-easy-clean",
		CategoryPath: "/en-gb/products/natural-and-artificial-stone",
		GonePath:     "/en-gb/products/natural-and-artificial-stone/gone-cleaner",
		Engine:       "http",
		Title:        "Lithofin MN Easy-Clean",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    0,
		Variant:      "/en-gb/products/natural-and-artificial-stone/lithofin-mn-{v}",
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "class=\"header header--h2 text-left\"", New: "class=\"renamed\""}},
	})
}
