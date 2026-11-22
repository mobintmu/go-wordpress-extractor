package e2e

import "testing"

// TestLowes runs the real lowes.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/lowes/pd-nutmeg-hickory.ai-web_search.json).
func TestLowes(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "lowes", Domain: "lowes.com", Recorded: "pd-nutmeg-hickory.ai-web_search",
		Path: "/pd/allen-roth-Nutmeg-Hickory-3-8-in-T-x-5-in-W-Handsculpted-Engineered-Hardwood-Flooring-32-29-sq-ft-Carton/50223071", CategoryPath: "/b/flooring/engineered-hardwood", GonePath: "/pd/gone-product/0000",
		Title: "allen + roth Nutmeg Hickory 3/8-in T x 5-in W Handscraped Engineered Hardwood Flooring (32.29-sq ft / Carton)", Price: -1, State: "out_of_stock",
		Specs: 16, Features: 4,
	})
}
