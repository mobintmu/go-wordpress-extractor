package e2e

import "testing"

// TestQualitybath runs the real qualitybath.com config, which is blocked for every page-fetch engine and so uses
// the default AI engine (fetch.engine "default", resolved from APP_DEFAULT_ENGINE to ai-crawler-avalai
// on this instance) with ai_fetch "web_search". The shop is never contacted; the search model's answer is replayed
// from the recorded run (testdata/sites/qualitybath/qualitybath.ai-web_search.json).
func TestQualitybath(t *testing.T) {
	runWebSite(t, webSiteCase{
		SiteID: "qualitybath", Domain: "qualitybath.com", Recorded: "qualitybath.ai-web_search",
		Path: "/california-faucets-sh-083-showerhead-55-8-dia-product-246185.htm", CategoryPath: "/brands/1619/grohe", GonePath: "/gone-showerhead.htm",
		Title: "California Faucets Showerhead SH-083 5-5/8\" DIA", Price: 29925, State: "available",
		Specs: 10, Features: 5,
	})
}
