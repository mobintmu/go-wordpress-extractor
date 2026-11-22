package e2e

import "testing"

// TestWestelm runs the real westelm.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/westelm/westelm.ai-web_search.json).
func TestWestelm(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "westelm", Domain: "westelm.com", Recorded: "westelm.ai-web_search",
		Path: "/products/yamazaki-sofa-back-console-table-w-shelf-d18054/", CategoryPath: "/shop/furniture/console-tables", GonePath: "/products/gone-table",
		Title: "Yamazaki Sofa Back Console Table w/ Shelf (50\")", Price: 19500, State: "available",
		Specs: 6, Features: 3,
	})
}
