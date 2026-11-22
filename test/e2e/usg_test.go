package e2e

import "testing"

// TestUsg runs the real usg.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/usg/usg.ai-web_search.json).
func TestUsg(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "usg", Domain: "usg.com", Recorded: "usg.ai-web_search",
		Path: "/en-US/p/product/sheetrock-brand-lay-in-gypsum-ceiling-panels-3260", CategoryPath: "/en-US/p/products/ceilings-500000000001749", GonePath: "/en-US/p/product/gone-panel",
		Title: "Sheetrock® Brand Lay-in Gypsum Ceiling Panels", Price: -1, State: "unknown",
		Specs: 17, Features: 6,
	})
}
