package e2e

import "testing"

// TestHermanmiller runs the real hermanmiller.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/hermanmiller/hermanmiller.ai-web_search.json).
func TestHermanmiller(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "hermanmiller", Domain: "hermanmiller.com", Recorded: "hermanmiller.ai-web_search",
		Path: "/products/seating/office-chairs/aeron-chair/", CategoryPath: "/", GonePath: "/products/seating/office-chairs/gone-chair",
		Title: "Aeron Chair", Price: -1, State: "unknown",
		Specs: 7, Features: 6,
	})
}
