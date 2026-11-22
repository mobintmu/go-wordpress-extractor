package e2e

import "testing"

// TestFlor runs the real flor.com config, which is blocked for every page-fetch engine and so uses
// ai-crawler-avalai with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/flor/flor.ai-web_search.json).
func TestFlor(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "flor", Domain: "flor.com", Recorded: "flor.ai-web_search",
		Path: "/area-rugs_carpet-tiles/solids/made-you-look/21-1361.html", CategoryPath: "/", GonePath: "/area-rugs_carpet-tiles/solids/gone-0-0",
		Title: "Made You Look - Pearl", Price: 2000, State: "available",
		Specs: 5, Features: 4,
	})
}
