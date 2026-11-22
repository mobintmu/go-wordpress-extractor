package e2e

import "testing"

// TestPella runs the real pella.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/pella/pella.ai-web_search.json).
func TestPella(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "pella", Domain: "pella.com", Recorded: "pella.ai-web_search",
		Path: "/shop/windows/250-series/single-hung-windows/", CategoryPath: "/shop/windows/double-hung", GonePath: "/shop/windows/250-series/gone-window",
		Title: "Pella® 250 Series Vinyl Single-Hung Window", Price: -1, State: "unknown",
		Specs: 13, Features: 4,
	})
}
