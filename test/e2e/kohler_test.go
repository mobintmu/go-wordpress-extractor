package e2e

import "testing"

// TestKohler runs the real kohler.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/kohler/kohler.ai-web_search.json).
func TestKohler(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "kohler", Domain: "kohler.com", Recorded: "kohler.ai-web_search",
		Path: "/en/products/kitchen-faucets/shop-kitchen-faucets/crue-pull-down-kitchen-sink-faucet-w-three-function-sprayhead-22972", CategoryPath: "/en/products/kitchen-faucets", GonePath: "/en/products/kitchen-faucets/shop-kitchen-faucets/gone-faucet",
		Title: "Crue Pull-Down Single-Handle Kitchen Faucet", Price: 43125, State: "available",
		Specs: 6, Features: 9,
	})
}
