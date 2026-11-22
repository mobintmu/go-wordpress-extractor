package e2e

import "testing"

// TestWoodworkerssource runs the real woodworkerssource.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/woodworkerssource/woodworkerssource.ai-web_search.json).
func TestWoodworkerssource(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "woodworkerssource", Domain: "woodworkerssource.com", Recorded: "woodworkerssource.ai-web_search",
		Path: "/walnut/walnut-flat-cut-paper-back-veneer-sheet-4-x-8-roll.html", CategoryPath: "/walnut", GonePath: "/walnut/gone-sheet",
		Title: "Premium Walnut (Flat Cut) Paper Back Veneer Sheet - 4' x 8' Roll", Price: 23100, State: "available",
		Specs: 10, Features: 0,
	})
}
