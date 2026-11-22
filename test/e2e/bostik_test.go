package e2e

import "testing"

// TestBostik runs the real bostik.com config against its saved live page (http, nothing mocked).

func TestBostik(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID:       "bostik",
		Domain:       "bostik.com",
		Fixture:      "a535-acryl-parquet",
		Path:         "/global/en/bostik-academy/library/product-portfolio/decoration-paint/a535/",
		CategoryPath: "/global/en/bostik-academy/library/product-portfolio/",
		GonePath:     "/global/en/bostik-academy/library/product-portfolio/decoration-paint/gone/",
		Engine:       "http",
		Title:        "BOSTIK A535 ACRYL PARQUET",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		SpecCount:    0,
		Variant:      "/global/en/bostik-academy/library/product-portfolio/decoration-paint/v{n}-{v}/",
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "class=\"banner-title\"", New: "class=\"renamed\""}},
	})
}
