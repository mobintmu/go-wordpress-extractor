package e2e

import "testing"

// TestNanawall runs the real nanawall.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/nanawall/nanawall.ai-web_search.json).
func TestNanawall(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "nanawall", Domain: "nanawall.com", Recorded: "nanawall.ai-web_search",
		Path: "/glass-walls/folding/nw-aluminum-840", CategoryPath: "/glass-walls/folding", GonePath: "/glass-walls/folding/gone-wall",
		Title: "NW Aluminum 840", Price: -1, State: "unknown",
		Specs: 11, Features: 6,
	})
}
