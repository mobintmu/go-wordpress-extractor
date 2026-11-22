package e2e

import "testing"

// TestRichelieu runs the real richelieu.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/richelieu/richelieu.ai-web_search.json).
func TestRichelieu(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "richelieu", Domain: "richelieu.com", Recorded: "richelieu.ai-web_search",
		Path: "/us/en/category/kitchen-and-bathroom-accessories/kitchen/under-sink-cabinets/tip-out-trays/miscellaneous-tip-out-tray-hinges/1249728/sku-1162G", CategoryPath: "/us/en/category/hinges-slides-and-opening-systems", GonePath: "/us/en/category/gone/sku-gone",
		Title: "Miscellaneous Tip-Out Tray Hinges", Price: -1, State: "unknown",
		Specs: 2, Features: 0,
	})
}
