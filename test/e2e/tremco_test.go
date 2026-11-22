package e2e

import "testing"

// TestTremco runs the real tremcosealants.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/tremco/tremco.ai-web_search.json).
func TestTremco(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "tremco", Domain: "tremcosealants.com", Recorded: "tremco.ai-web_search",
		Path: "/products/tremsil-200", CategoryPath: "/products", GonePath: "/products/gone-sealant",
		Title: "Tremsil® 200", Price: -1, State: "unknown",
		Specs: 6, Features: 7,
	})
}
