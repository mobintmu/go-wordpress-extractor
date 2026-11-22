package e2e

import "testing"

// TestSchumacher runs the real schumacher.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/schumacher/schumacher.ai-web_search.json).
func TestSchumacher(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "schumacher", Domain: "schumacher.com", Recorded: "schumacher.ai-web_search",
		Path: "/catalog/products/5005854", CategoryPath: "/", GonePath: "/catalog/products/99999",
		Title: "Shagreen - Carbon", Price: -1, State: "unknown",
		Specs: 3, Features: 0,
	})
}
