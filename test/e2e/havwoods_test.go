package e2e

import "testing"

// TestHavwoods runs the real havwoods.com config (http engine, nothing to
// mock) against its saved live page and variants of it. Havwoods is a trade
// supplier with no price on the page ('Request pricing'), so the sample
// itself is the call-for-price case, and there is no sale or out-of-stock
// subtest: the page has no price, cart or stock markup to vary.
func TestHavwoods(t *testing.T) {
	runSiteCase(t, siteCase{
		SiteID: "havwoods", Domain: "havwoods.com", Fixture: "amazon-13-prime-1-strip",
		Path:         "/us/products/hw3699/",
		Variant:      "/us/products/hw900000{n}/",
		CategoryPath: "/us/selection/engineered-wood-flooring/",
		GonePath:     "/us/products/hw999999/",
		Engine:       "http",
		Title:        "Amazon 13 Prime 260mm 1-Strip",
		Price:        -1,
		State:        "call_for_price",
		Canonical:    "https://www.havwoods.com/us/products/hw3699/",
		SpecCount:    14,
		Specs: map[string][2]string{
			"Species":    {"", "European Oak"},
			"Wear Layer": {"", "4mm"},
			"Thickness":  {"", `1/2" (13mm)`},
			"Width":      {"", `10 1/4" (260mm)`},
		},
		NoSpecs: []edit{{Old: `<table class="PdpTopSectionstyles__Specifications`, New: `<table class="PdpTopSectionstyles__Removed`}},
		Layout:  []edit{{Old: `<h1 class="PdpTopSectionstyles__Name`, New: `<h1 class="PdpTopSectionstyles__Renamed`}},
	})
}
