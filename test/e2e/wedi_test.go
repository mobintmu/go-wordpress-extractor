package e2e

import "testing"

// TestWedi runs the real wedi.net config against its saved live page (AI engine, mocked: the fake LLM replays the recorded answer).

func TestWedi(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID:       "wedi",
		Domain:       "wedi.net",
		Fixture:      "fundo-plano",
		Path:         "/en/products/flush-to-floor-showers/fundo-shower-element-with-point-drainage/fundo-plano",
		CategoryPath: "/en/products/flush-to-floor-showers",
		Variant:      "/en/products/flush-to-floor-showers/fundo-shower-element-with-point-drainage/fundo-{v}",
		Title:        "Fundo Plano",
		Price:        -1,
		State:        "unknown",
		Canonical:    "",
		Specs:        16,
		Features:     3,
		Warnings:     []string{"PRICE_MISSING"},
		Layout:       []edit{{Old: "class=\"product-header__title", New: "class=\"renamed"}},
	})
}
