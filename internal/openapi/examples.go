package openapi

// Example is one sample request for POST /api/v1/extract. Swagger UI lists
// these in the Examples dropdown above the request body.
type Example struct {
	// Key is the example's id in the spec.
	Key string
	// Summary is the label shown in the dropdown.
	Summary string
	// Description is shown under the dropdown when the example is picked.
	Description string
	URL         string
	RequestID   string
	// AIRule marks a URL on a host without a site config: it is served by
	// the default rule (engine APP_DEFAULT_ENGINE: ai-crawler-avalai or
	// ai-crawler-cloudflare).
	AIRule bool
}

// ExtractExamples holds one sample product per supported site. Keep it in
// step with md/CURL_EXAMPLES.md. The first entry is the default body.
var ExtractExamples = []Example{
	// http engine
	{
		Key: "badomjip", Summary: "badomjip.com (http, IRT)",
		Description: "Prices are in toman (IRT). The site has call-for-price, out-of-stock and variable products.",
		URL:         "https://badomjip.com/product/almondbutter400/", RequestID: "badomjip-1",
	},
	{
		Key: "3form", Summary: "3-form.com (http)",
		URL: "https://www.3-form.com/products/edge-lit-chroma-countertop", RequestID: "3form-1",
	},
	{
		Key: "emtek", Summary: "emtek.com (http)",
		URL:       "https://www.emtek.com/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/",
		RequestID: "emtek-1",
	},
	{
		Key: "hmland", Summary: "hmland.ir (http, rial converted to IRT)",
		Description: "The site shows rial; the response converts to toman (currency IRT). Persian slugs must be percent-encoded.",
		URL:         "https://hmland.ir/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c/", RequestID: "hmland-1",
	},
	{
		Key: "hosseinibrothers", Summary: "hosseinibrothers.ir (http, IRT)",
		Description: "Prices are in toman (IRT). Percent-encoded Persian path.",
		URL:         "https://hosseinibrothers.ir/%DA%A9%D8%B4%D9%85%D8%B4/%DA%A9%D8%B4%D9%85%D8%B4-%D9%BE%D9%84%D9%88%DB%8C%DB%8C-%D8%A2%D9%81%D8%AA%D8%A7%D8%A8%DB%8C",
		RequestID:   "hosseinibrothers-1",
	},
	{
		Key: "msisurfaces", Summary: "msisurfaces.com (http)",
		URL: "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/", RequestID: "msisurfaces-1",
	},
	{
		Key: "sherwinwilliams", Summary: "sherwin-williams.com (http, no price)",
		Description: "The price is loaded by JavaScript, so the http engine gets none. Expect PRICE_MISSING and price in unavailable_fields with reason NOT_ON_PAGE.",
		URL:         "https://www.sherwin-williams.com/homeowners/products/emerald-interior-acrylic-latex-paint?colorPartNumber=SW6617&colorType=PALETTE",
		RequestID:   "sherwin-williams-1",
	},
	{
		Key: "interface", Summary: "shop.interface.com (http)",
		URL: "https://shop.interface.com/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043R.html", RequestID: "interface-1",
	},
	{
		Key: "tilebar", Summary: "tilebar.com (http)",
		URL:       "https://www.tilebar.com/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html",
		RequestID: "tilebar-1",
	},

	// browser engine
	{
		Key: "flooranddecor", Summary: "flooranddecor.com (browser)",
		Description: "Needs APP_BROWSER_ENABLED=true, otherwise ENGINE_UNAVAILABLE. If Cloudflare blocks the server IP the call fails with BOT_PROTECTION.",
		URL:         "https://www.flooranddecor.com/ceramic-tile/zellige-oat-high-gloss-ceramic-tile-101130060.html", RequestID: "flooranddecor-1",
	},
	{
		Key: "hafele", Summary: "hafele.com.de (browser, no price)",
		Description: "Needs APP_BROWSER_ENABLED=true. Prices are hidden from anonymous visitors: expect price.state unknown with a PRICE_MISSING warning.",
		URL:         "https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/", RequestID: "hafele-1",
	},
	{
		Key: "lumens", Summary: "lumens.com (browser, headful + proxy)",
		Description: "Behind a Cloudflare Turnstile challenge. Needs APP_BROWSER_ENABLED=true, APP_BROWSER_HEADLESS=false and a residential proxy (APP_PROXY_*); otherwise BOT_PROTECTION.",
		URL:         "https://www.lumens.com/n701-round-corner-sofa-by-ethnicraft-ETH2111288.html", RequestID: "lumens-1",
	},

	// AI engine selected by a site config
	{
		Key: "fergusonhome", Summary: "fergusonhome.com (ai-crawler-avalai, web search)",
		Description: "Ferguson is behind Akamai and answers 403 to plain HTTP and to Chrome, so the site config sets fetch.engine ai-crawler-avalai with ai_fetch web_search: the site is never contacted. " +
			"Only the URL goes to AvalAI's search model (APP_AI_CRAWLER_SEARCH_MODEL, sonar), which looks the product up on the web and returns title, price (USD), description and specs with its sources. " +
			"Expect site_id fergusonhome, source.engine ai-crawler-avalai, source.fetched_with web_search, source.site_contacted false, source.sources, and the warnings AI_EXTRACTED and AI_UNVERIFIED: the values were not checked against the page, so price and stock may be out of date. " +
			"Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key. Fails with SEARCH_NO_RESULT when no source is on fergusonhome.com.",
		URL: "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155", RequestID: "ferguson-1",
	},

	// AI crawler default rule: no site config
	{
		Key: "wayfair-ai", Summary: "wayfair.com (AI crawler, no site config)",
		Description: "No file in configs/sites/ claims wayfair.com, so the default rule serves it with APP_DEFAULT_ENGINE: the page is fetched and an LLM " +
			"(ai-crawler-avalai: APP_AI_CRAWLER_MODEL, deepseek-v4.1-flash on AvalAI; ai-crawler-cloudflare: APP_AI_CRAWLER_CLOUDFLARE_MODEL, @cf/deepseek-ai/deepseek-v4.1-flash-1017 through Cloudflare AI Gateway) " +
			"reads name, price, image, description, specs and features, each checked against the page. Expect site_id ai-crawler-default, config_origin default, source.engine set to the default engine and an AI_EXTRACTED warning. " +
			"Needs APP_AI_CRAWLER_ENABLED=true with that engine configured; with APP_DEFAULT_ENGINE=none, SITE_NOT_CONFIGURED. Wayfair uses bot protection, so the call may fail with BOT_PROTECTION from the server's IP. " +
			"After 3 product pages match in shadow, the host is served by learned selectors (config_origin learned).",
		URL:       "https://www.wayfair.com/furniture/pdp/mercer41-55-w-8-drawer-dresser-w118377980.html?piid=2115584224",
		RequestID: "wayfair-ai-1", AIRule: true,
	},
}
