package e2e

import "testing"

// TestAcehardware runs the real acehardware.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/acehardware/acehardware.ai-web_search.json).
func TestAcehardware(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "acehardware", Domain: "acehardware.com", Recorded: "acehardware.ai-web_search",
		Path: "/departments/paint-and-supplies/caulk-and-sealants/caulk/1912302", CategoryPath: "/", GonePath: "/departments/paint-and-supplies/caulk-and-sealants/caulk/gone-0000",
		Title: "Sashco Lexel Clear Synthetic Rubber All Purpose Caulk 5 oz", Price: 899, State: "available",
		Specs: 0, Features: 0,
	})
}
