package e2e

import "testing"

// TestGaf runs the real gaf.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/gaf/gaf.ai-web_search.json).
func TestGaf(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "gaf", Domain: "gaf.com", Recorded: "gaf.ai-web_search",
		Path: "/en-us/roofing-materials/residential-roofing-materials/shingles/timberline-hdz", CategoryPath: "/en-us/roofing-materials/residential-roofing-materials/shingles", GonePath: "/en-us/roofing-materials/residential-roofing-materials/shingles/gone-shingle",
		Title: "Timberline HDZ: GAF's #1-selling shingle", Price: -1, State: "unknown",
		Specs: 0, Features: 5,
	})
}
