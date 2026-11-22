package e2e

import "testing"

// TestFarrowBall runs the real farrow-ball.com config, which selects the paid
// ai-crawler-avalai engine (roll sizes and colours are prose only the LLM
// can pair), against its saved live US wallpaper page. AvalAI is mocked: the
// fake LLM replays the answer recorded on 2026-11-19.
func TestFarrowBall(t *testing.T) {
	runAISiteCase(t, aiSiteCase{
		SiteID: "farrowball", Domain: "farrow-ball.com", Fixture: "five-over-stripe-wallpaper",
		Path:         "/us/wallpaper/five-over-stripe",
		Variant:      "/us/wallpaper/five-over-stripe-{v}",
		CategoryPath: "/us/",
		Title:        "Five Over Stripe",
		Price:        27000,
		State:        "available",
		Canonical:    "https://www.farrow-ball.com/us/wallpaper/five-over-stripe",
		Specs:        5,
		Layout:       []edit{{Old: `<h1 class="page-title"`, New: `<h2 class="page-title"`}},
	})
}
