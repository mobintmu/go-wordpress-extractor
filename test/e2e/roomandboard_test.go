package e2e

import "testing"

// TestRoomandboard runs the real roomandboard.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/roomandboard/roomandboard.ai-web_search.json).
func TestRoomandboard(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "roomandboard", Domain: "roomandboard.com", Recorded: "roomandboard.ai-web_search",
		Path: "/catalog/living/sofas-and-loveseats/andre-sofas", CategoryPath: "/catalog/living/sofas-and-loveseats", GonePath: "/catalog/living/sofas-and-loveseats/gone-sofa",
		Title: "André Sofas", Price: 229900, State: "available",
		Specs: 11, Features: 5,
	})
}
