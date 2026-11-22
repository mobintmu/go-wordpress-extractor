package e2e

import "testing"

// TestDeckorators runs the real deckorators.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/deckorators/deckorators.ai-web_search.json).
func TestDeckorators(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "deckorators", Domain: "deckorators.com", Recorded: "deckorators.ai-web_search",
		Path: "/products/voyage-decking", CategoryPath: "/collections/traditional-composite-products", GonePath: "/products/gone-decking",
		Title: "Voyage Decking", Price: -1, State: "unknown",
		Specs: 3, Features: 7,
	})
}
