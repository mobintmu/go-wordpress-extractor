package e2e

import "testing"

// TestDaltile runs the real daltile.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/daltile/daltile.ai-web_search.json).
func TestDaltile(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "daltile", Domain: "daltile.com", Recorded: "daltile.ai-web_search",
		Path: "/products/concrete-look/portfolio/white", CategoryPath: "/", GonePath: "/products/stone-look/gone-0000",
		Title: "White", Price: -1, State: "unknown",
		Specs: 16, Features: 5,
	})
}
