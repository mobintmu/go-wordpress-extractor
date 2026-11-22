package e2e

import "testing"

// TestOwenscorning runs the real owenscorning.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/owenscorning/owenscorning.ai-web_search.json).
func TestOwenscorning(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "owenscorning", Domain: "owenscorning.com", Recorded: "owenscorning.ai-web_search",
		Path: "/en-us/roofing/shingles/oakridge", CategoryPath: "/en-us/roofing", GonePath: "/en-us/roofing/shingles/gone-shingle",
		Title: "Oakridge® Shingles", Price: -1, State: "unknown",
		Specs: 5, Features: 4,
	})
}
