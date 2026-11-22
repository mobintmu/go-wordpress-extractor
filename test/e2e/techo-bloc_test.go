package e2e

import "testing"

// TestTechoBloc runs the real techo-bloc.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/techo-bloc/techo-bloc.ai-web_search.json).
func TestTechoBloc(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "techo-bloc", Domain: "techo-bloc.com", Recorded: "techo-bloc.ai-web_search",
		Path: "/shop/walls/borealis", CategoryPath: "/shop/walls", GonePath: "/shop/walls/gone-wall",
		Title: "Borealis Wall", Price: -1, State: "unknown",
		Specs: 9, Features: 6,
	})
}
