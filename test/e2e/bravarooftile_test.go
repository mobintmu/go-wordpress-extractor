package e2e

import "testing"

// TestBravarooftile runs the real bravarooftile.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/bravarooftile/bravarooftile.ai-web_search.json).
func TestBravarooftile(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "bravarooftile", Domain: "bravarooftile.com", Recorded: "bravarooftile.ai-web_search",
		Path: "/roof-tiles/barrel-tile/", CategoryPath: "/", GonePath: "/roof-tiles/gone-tile",
		Title: "Brava Barrel Tile", Price: -1, State: "unknown",
		Specs: 12, Features: 7,
	})
}
