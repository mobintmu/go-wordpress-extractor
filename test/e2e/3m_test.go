package e2e

import "testing"

// Test3M runs the real 3m.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/3m/3m.ai-web_search.json).
func Test3M(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "3m", Domain: "3m.com", Recorded: "3m.ai-web_search",
		Path: "/3M/en_US/p/d/b00008068/", CategoryPath: "/", GonePath: "/3M/en_US/p/d/gone-0000",
		Title: "3M™ Safety-Walk™ Coarse Tapes and Treads 700 Series", Price: -1, State: "unknown",
		Specs: 0, Features: 10,
	})
}
