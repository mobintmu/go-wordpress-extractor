package e2e

import "testing"

// TestRehau runs the real rehau.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/rehau/rehau.ai-web_search.json).
func TestRehau(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "rehau", Domain: "rehau.com", Recorded: "rehau.ai-web_search",
		Path: "/us-en/mechanicalplumbing/search-by-product/raupex-pipe", CategoryPath: "/us-en/mechanicalplumbing/search-by-product", GonePath: "/us-en/mechanicalplumbing/search-by-product/gone-pipe",
		Title: "RAUPEX Crosslinked Polyethylene (PEXa) Pipe", Price: -1, State: "unknown",
		Specs: 0, Features: 8,
	})
}
