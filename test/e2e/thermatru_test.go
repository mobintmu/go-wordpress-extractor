package e2e

import "testing"

// TestThermatru runs the real thermatru.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/thermatru/thermatru.ai-web_search.json).
func TestThermatru(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "thermatru", Domain: "thermatru.com", Recorded: "thermatru.ai-web_search",
		Path: "/explore-products/door-style/S601", CategoryPath: "/explore-products/new-products", GonePath: "/explore-products/door-style/gone-door",
		Title: "S601 - Smooth-Star®", Price: -1, State: "unknown",
		Specs: 8, Features: 6,
	})
}
