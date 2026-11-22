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
		Key: "1000bulbs", Summary: "1000bulbs.com (http)",
		Description: "Prices are in US dollars per unit (`$21.55 ea.` \u2192 `2155` cents). Specs come from the two spec tables.",
		URL:         "https://www.1000bulbs.com/product/37/STAG-FEP.html", RequestID: "1000bulbs-1",
	},
	{
		Key: "abcworldwidestone", Summary: "abcworldwidestone.com (http, no price)",
		Description: "Slab supplier with no price on the page: expect price.state unknown with a PRICE_MISSING warning, and the standard Dimensions/Thickness/Finish notes as specs.",
		URL:         "https://abcworldwidestone.com/portfolio/gris-macael-dusted/", RequestID: "abcworldwidestone-1",
	},
	{
		Key: "arktura", Summary: "arktura.com (http, no price)",
		Description: "Product-line page with no price (architect-specified): expect title, image and description with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://arktura.com/vapor-exterior/", RequestID: "arktura-1",
	},
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
		Key: "bentleymills", Summary: "bentleymills.com (http, no price)",
		Description: "Commercial mill with no price on the page: expect price.state unknown with a PRICE_MISSING warning. Specs come from the SPECS panel.",
		URL:         "https://www.bentleymills.com/product/4-agnona/", RequestID: "bentleymills-1",
	},
	{
		Key: "bostik", Summary: "bostik.com (http, no price)",
		Description: "Distributor-only products: expect title, image and description with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.bostik.com/global/en/bostik-academy/library/product-portfolio/decoration-paint/a535/", RequestID: "bostik-1",
	},
	{
		Key: "cosmossurfaces", Summary: "cosmossurfaces.com (http, metadata only)",
		Description: "Angular app: the fields come from the page's <head> metadata (og:title, og:image, meta description). No price: expect price.state unknown with a PRICE_MISSING warning.",
		URL:         "https://www.cosmossurfaces.com/washington/granite/absolute-black-premium-3cm", RequestID: "cosmossurfaces-1",
	},
	{
		Key: "drytreat", Summary: "drytreat.com (browser, no price)",
		Description: "Blocks plain HTTP requests from this network; headless Chrome renders the page. Expect title and description, price.state unknown and a PRICE_MISSING warning. Needs APP_BROWSER_ENABLED=true.",
		URL:         "https://www.drytreat.com/sealers/residential/stain-proof-waterborne/", RequestID: "drytreat-1",
	},
	{
		Key: "emtek", Summary: "emtek.com (http)",
		URL:       "https://www.emtek.com/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/",
		RequestID: "emtek-1",
	},
	{
		Key: "fleetwood", Summary: "fleetwoodusa.com (http, no price)",
		Description: "Dealer-quoted products: expect title, description and the Standard Features table as specs, with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.fleetwoodusa.com/products/3-series/series-3070/", RequestID: "fleetwood-1",
	},
	{
		Key: "havwoods", Summary: "havwoods.com (http, call for price)",
		Description: "Trade supplier with no price on the page ('Request pricing'): expect price.state call_for_price with price.current null, and availability in unavailable_fields (NOT_CONFIGURED).",
		URL:         "https://www.havwoods.com/us/products/hw3699/", RequestID: "havwoods-1",
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
		Key: "innovationsusa", Summary: "innovationsusa.com (http, no price)",
		Description: "Trade-only prices: expect title, image, description and 9 specs with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.innovationsusa.com/item/antwerp", RequestID: "innovationsusa-1",
	},
	{
		Key: "laminart", Summary: "laminart.com (http, no price)",
		Description: "Samples and quotes only: expect title, image, description and attribute specs with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.laminart.com/coal-706", RequestID: "laminart-1",
	},
	{
		Key: "llflooring", Summary: "llflooring.com (http)",
		Description: "The price is the box price in cents (USD); price.raw also shows the per-square-foot price.",
		URL:         "https://www.llflooring.com/p/coreluxe-5mm-w-pad-daydream-meadow-oak-6-in-wide-x-48-in-long-10055236/", RequestID: "llflooring-1",
	},
	{
		Key: "marvin", Summary: "marvin.com (http, no price)",
		Description: "Dealer-quoted windows and doors: expect title and description with price.state unknown and a PRICE_MISSING warning. Specifications are rendered by scripts and are not read.",
		URL:         "https://www.marvin.com/products/windows/double-hung/elevate-double-hung-window", RequestID: "marvin-1",
	},
	{
		Key: "miracle", Summary: "miracle-sealants.com (http, no price)",
		Description: "Retailer-sold cleaners: expect title, image and description with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://miracle-sealants.com/miracle-sealants-porcelain-ceramic-tile-cleaner/", RequestID: "miracle-1",
	},
	{
		Key: "msisurfaces", Summary: "msisurfaces.com (http)",
		URL: "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/", RequestID: "msisurfaces-1",
	},
	{
		Key: "phillipjeffries", Summary: "phillipjeffries.com (http, no price)",
		Description: "Trade-only prices and stock: expect title, image and description with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.phillipjeffries.com/shop/CARD-464/46400", RequestID: "phillipjeffries-1",
	},
	{
		Key: "schluter", Summary: "schluter.com (http, no price)",
		Description: "Distributor-sold membranes: expect title, image and description with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.schluter.com/schluter-us/en_US/Membranes/Uncoupling-(DITRA)/Schluter%C2%AE-DITRA-PS/p/DITRA_PS", RequestID: "schluter-1",
	},
	{
		Key: "shawfloors", Summary: "shawfloors.com (http, price per sq ft)",
		Description: "The price is the per-square-foot list price in cents (USD). Shaw sells through retailers, so availability is not configured and completeness is partial.",
		URL:         "https://shawfloors.com/en-us/laminate/oak-grove-hazelwood/sl119-02039", RequestID: "shawfloors-1",
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
		Key: "soundseal", Summary: "soundseal.com (http, no price)",
		Description: "Contact-for-price panels: expect title, image and description with price.state unknown and a PRICE_MISSING warning.",
		URL:         "https://www.soundseal.com/architectural-interiors/products/acoustical-wall-panels/s-4200-tuned-absorbers", RequestID: "soundseal-1",
	},
	{
		Key: "tilebar", Summary: "tilebar.com (http)",
		URL:       "https://www.tilebar.com/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html",
		RequestID: "tilebar-1",
	},
	{
		Key: "veneersupplies", Summary: "veneersupplies.com (default → ai-crawler-avalai)",
		Description: "Prices are in US dollars per sheet (`$16.54` \u2192 `1654` cents), sold from stock.",
		URL:         "https://www.veneersupplies.com/products/Flat-Cut-Rosewood-Bolivian-Veneer-Sheet--55-x-71.html", RequestID: "veneersupplies-1",
	},
	{
		Key: "walkerzanger", Summary: "walkerzanger.com (http, no price)",
		Description: "Showroom brand with no price for anonymous visitors: expect price.state unknown with a PRICE_MISSING warning. The description is generic category copy.",
		URL:         "https://www.walkerzanger.com/product/18x48x1-1-2-adirondack-grey-coping-natural/", RequestID: "walkerzanger-1",
	},

	// browser engine
	{
		Key: "emser", Summary: "emser.com (browser, no price)",
		Description: "The product page is rendered by JavaScript (Optimizely B2B React storefront). Needs APP_BROWSER_ENABLED=true, otherwise ENGINE_UNAVAILABLE. Emser sells through distributors: expect price.state unknown with a PRICE_MISSING warning, and 41 specs in four groups.",
		URL:         "https://www.emser.com/product/anvaya-f44anvapo1212m", RequestID: "emser-1",
	},
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
		Key: "3m", Summary: "3m.com (default → ai-crawler-avalai, web search)",
		Description: "3M's product pages are behind an Akamai 'Access Denied' block for every page fetch from this network (plain HTTP, headless Chrome, and AvalAI's page fetch in browser mode). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.3m.com/3M/en_US/p/d/b00008068/", RequestID: "3m-1",
	},
	{
		Key: "acehardware", Summary: "acehardware.com (default → ai-crawler-avalai, web search)",
		Description: "Ace Hardware product pages answer 'Attention Required' or bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.acehardware.com/departments/paint-and-supplies/caulk-and-sealants/caulk/1912302", RequestID: "acehardware-1",
	},
	{
		Key: "annsacks", Summary: "annsacks.kohler.com (default → ai-crawler-avalai)",
		Description: "The product page is a Next.js shell whose only product data is a JSON-LD block, so the site config sets fetch.engine default (APP_DEFAULT_ENGINE, ai-crawler-avalai on this instance; with a Chrome User-Agent, since the default one is tarpitted). Expect source.engine ai-crawler-avalai, price in USD and an AI_EXTRACTED warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key and APP_DEFAULT_ENGINE=ai-crawler-avalai, otherwise ENGINE_UNAVAILABLE.",
		URL:         "https://annsacks.kohler.com/p/trim/savoy-0-5-x-8-box-liner-trim-as11682-trim-boxliner", RequestID: "annsacks-1",
	},
	{
		Key: "bravarooftile", Summary: "bravarooftile.com (default → ai-crawler-avalai, web search)",
		Description: "Brava composite roof tiles are behind bot protection ('Just a moment') for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.bravarooftile.com/roof-tiles/barrel-tile/", RequestID: "bravarooftile-1",
	},
	{
		Key: "daltile", Summary: "daltile.com (default → ai-crawler-avalai, web search)",
		Description: "Daltile (Mohawk Industries) pages time out or are denied for every page fetch from this network, including the AI engine's page fetch. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.daltile.com/products/concrete-look/portfolio/white", RequestID: "daltile-1",
	},
	{
		Key: "dap", Summary: "dap.com (default → ai-crawler-avalai)",
		Description: "Retail product pages: the variant table (SKU, unit size, colour) and the image are read by the LLM, which the selectors cannot do. Expect price.state unknown with PRICE_MISSING. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.dap.com/products/adhesives/weldwood-landau-top-and-trim-1145-high-heat-resistant-contact-cement", RequestID: "dap-1",
	},
	{
		Key: "deckorators", Summary: "deckorators.com (default → ai-crawler-avalai, web search)",
		Description: "Deckorators product pages answer bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.deckorators.com/products/voyage-decking", RequestID: "deckorators-1",
	},
	{
		Key: "farrowball", Summary: "farrow-ball.com (default → ai-crawler-avalai, US store)",
		Description: "US store (USD). The site config sets fetch.engine default (APP_DEFAULT_ENGINE, ai-crawler-avalai on this instance) because the roll sizes and colours are prose only the LLM can pair. Expect source.engine ai-crawler-avalai, price.current 27000, 5 specs and an AI_EXTRACTED warning. Paint pages have no server-side price (PRICE_MISSING). Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key and APP_DEFAULT_ENGINE=ai-crawler-avalai, otherwise ENGINE_UNAVAILABLE.",
		URL:         "https://www.farrow-ball.com/us/wallpaper/five-over-stripe", RequestID: "farrowball-1",
	},
	{
		Key: "fergusonhome", Summary: "fergusonhome.com (default → ai-crawler-avalai, web search)",
		Description: "Ferguson is behind Akamai and answers 403 to plain HTTP and to Chrome, so the site config sets fetch.engine default (APP_DEFAULT_ENGINE, ai-crawler-avalai on this instance) with ai_fetch web_search: the site is never contacted. " +
			"Only the URL goes to AvalAI's search model (APP_AI_CRAWLER_SEARCH_MODEL, sonar), which looks the product up on the web and returns title, price (USD), description and specs with its sources. " +
			"Expect site_id fergusonhome, source.engine ai-crawler-avalai, source.fetched_with web_search, source.site_contacted false, source.sources, and the warnings AI_EXTRACTED and AI_UNVERIFIED: the values were not checked against the page, so price and stock may be out of date. " +
			"Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key and APP_DEFAULT_ENGINE=ai-crawler-avalai, otherwise ENGINE_UNAVAILABLE. Fails with SEARCH_NO_RESULT when no source is on fergusonhome.com.",
		URL: "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155", RequestID: "ferguson-1",
	},
	{
		Key: "flor", Summary: "flor.com (default → ai-crawler-avalai, web search)",
		Description: "FLOR (modular carpet tiles) is behind a Cloudflare block page for every page fetch from this network, including headless Chrome. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.flor.com/area-rugs_carpet-tiles/solids/made-you-look/21-1361.html", RequestID: "flor-1",
	},
	{
		Key: "gaf", Summary: "gaf.com (default → ai-crawler-avalai, web search)",
		Description: "GAF product pages are denied (403) or time out for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.gaf.com/en-us/roofing-materials/residential-roofing-materials/shingles/timberline-hdz", RequestID: "gaf-1",
	},
	{
		Key: "hermanmiller", Summary: "hermanmiller.com (default → ai-crawler-avalai, web search)",
		Description: "Herman Miller product pages deny every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.hermanmiller.com/products/seating/office-chairs/aeron-chair/", RequestID: "hermanmiller-1",
	},
	{
		Key: "kichler", Summary: "kichler.com (default → ai-crawler-avalai)",
		Description: "The price is only in the page's JSON-LD, which the LLM reads. Expect price 2299 (USD 22.99), the image and 6 attribute specs, with an AI_EXTRACTED warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.kichler.com/2-fixture-mount-in-centennial-brass-15515CBR", RequestID: "kichler-1",
	},
	{
		Key: "kohler", Summary: "kohler.com (default → ai-crawler-avalai, web search)",
		Description: "Kohler product pages are denied (403) or time out for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.kohler.com/en/products/kitchen-faucets/shop-kitchen-faucets/crue-pull-down-kitchen-sink-faucet-w-three-function-sprayhead-22972", RequestID: "kohler-1",
	},
	{
		Key: "lithofin", Summary: "lithofin.com (ai-crawler-avalai)",
		Description: "Retailer-sold products: the technical data table and the image are read by the LLM. Expect 5 specs, the image, price.state unknown and a PRICE_MISSING warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.lithofin.com/en-gb/products/natural-and-artificial-stone/lithofin-mn-easy-clean", RequestID: "lithofin-1",
	},
	{
		Key: "lowes", Summary: "lowes.com (default → ai-crawler-avalai, web search)",
		Description: "Akamai blocks every page fetch from this network, so the site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.lowes.com/pd/allen-roth-Nutmeg-Hickory-3-8-in-T-x-5-in-W-Handsculpted-Engineered-Hardwood-Flooring-32-29-sq-ft-Carton/50223071", RequestID: "lowes-1",
	},
	{
		Key: "marble", Summary: "marble.com (default → ai-crawler-avalai, no price)",
		Description: "The site config sets fetch.engine default (APP_DEFAULT_ENGINE, ai-crawler-avalai on this instance) because the material facts (type, origin, colours) and applications are only readable by the LLM. Expect source.engine ai-crawler-avalai, 4 specs, 15 features, price.state unknown and the warnings AI_EXTRACTED and PRICE_MISSING (countertops are quoted per project). Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key and APP_DEFAULT_ENGINE=ai-crawler-avalai, otherwise ENGINE_UNAVAILABLE.",
		URL:         "https://marble.com/granite-countertops/gran-perla/1179", RequestID: "marble-1",
	},
	{
		Key: "moistureshield", Summary: "moistureshield.com (default → ai-crawler-avalai)",
		Description: "Dealer-sold decking: the feature list and warranties are read by the LLM. Expect 7 features, 3 specs, the image, price.state unknown and a PRICE_MISSING warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.moistureshield.com/products/composite-decking/vision/", RequestID: "moistureshield-1",
	},
	{
		Key: "nanawall", Summary: "nanawall.com (default → ai-crawler-avalai, web search)",
		Description: "NanaWall product pages are protected by bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.nanawall.com/glass-walls/folding/nw-aluminum-840", RequestID: "nanawall-1",
	},
	{
		Key: "owenscorning", Summary: "owenscorning.com (default → ai-crawler-avalai, web search)",
		Description: "Owens Corning product pages return an access-denied page for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.owenscorning.com/en-us/roofing/shingles/oakridge", RequestID: "owenscorning-1",
	},
	{
		Key: "pella", Summary: "pella.com (default → ai-crawler-avalai, web search)",
		Description: "Pella product pages are protected by bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.pella.com/shop/windows/250-series/single-hung-windows/", RequestID: "pella-1",
	},
	{
		Key: "qualitybath", Summary: "qualitybath.com (default → ai-crawler-avalai, web search)",
		Description: "Quality Bath product pages are protected by bot protection (403) for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.qualitybath.com/california-faucets-sh-083-showerhead-55-8-dia-product-246185.htm", RequestID: "qualitybath-1",
	},
	{
		Key: "rehau", Summary: "rehau.com (default → ai-crawler-avalai, web search)",
		Description: "REHAU product pages return access denied (403) for every page fetch from this network; AvalAI's page fetch works but its answer cannot be checked against a saved page. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.rehau.com/us-en/mechanicalplumbing/search-by-product/raupex-pipe", RequestID: "rehau-1",
	},
	{
		Key: "richelieu", Summary: "richelieu.com (default → ai-crawler-avalai, web search)",
		Description: "Richelieu product pages time out for every page fetch from this network (the search route returns partial data: 2 specs). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.richelieu.com/us/en/category/kitchen-and-bathroom-accessories/kitchen/under-sink-cabinets/tip-out-trays/miscellaneous-tip-out-tray-hinges/1249728/sku-1162G", RequestID: "richelieu-1",
	},
	{
		Key: "roomandboard", Summary: "roomandboard.com (default → ai-crawler-avalai, web search)",
		Description: "Room & Board product pages return access denied (403) for every page fetch from this network, and AvalAI's page fetch is denied too. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.roomandboard.com/catalog/living/sofas-and-loveseats/andre-sofas", RequestID: "roomandboard-1",
	},
	{
		Key: "schumacher", Summary: "schumacher.com (default → ai-crawler-avalai, web search)",
		Description: "Schumacher's storefront is protected by an AWS WAF JavaScript challenge; HTTP returns an empty challenge page and headless Chrome renders the app without the product (see the progress log). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://schumacher.com/catalog/products/5005854", RequestID: "schumacher-1",
	},
	{
		Key: "steelcase", Summary: "steelcase.com (default → ai-crawler-avalai, www page)",
		Description: "The www product page carries no JSON-LD price, so expect title, image and description with price.state unknown (the store host, store.steelcase.com, shows the price). Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.steelcase.com/products/office-chairs/gesture/", RequestID: "steelcase-1",
	},
	{
		Key: "stonelandusa", Summary: "stonelandusa.com (default → ai-crawler-avalai)",
		Description: "The visible page says 'Get Quote'; the slab price is only in the page's JSON-LD, so the site config sets fetch.engine default (APP_DEFAULT_ENGINE, ai-crawler-avalai on this instance). Expect source.engine ai-crawler-avalai, price.current 182175 (USD 1,821.75) and an AI_EXTRACTED warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key and APP_DEFAULT_ENGINE=ai-crawler-avalai, otherwise ENGINE_UNAVAILABLE.",
		URL:         "https://stonelandusa.com/product/grigio-carnico-sl-614/", RequestID: "stonelandusa-1",
	},
	{
		Key: "techo-bloc", Summary: "techo-bloc.com (default → ai-crawler-avalai, web search)",
		Description: "Techo-Bloc product pages return bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.techo-bloc.com/shop/walls/borealis", RequestID: "techo-bloc-1",
	},
	{
		Key: "thermatru", Summary: "thermatru.com (default → ai-crawler-avalai, web search)",
		Description: "Therma-Tru door style pages are access-denied (403) for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.thermatru.com/explore-products/door-style/S601", RequestID: "thermatru-1",
	},
	{
		Key: "timbertech", Summary: "timbertech.com (default → ai-crawler-avalai)",
		Description: "Dealer-sold decking: the collection specs (board sizes and finish) are read by the LLM. Expect the specs, the image, price.state unknown and a PRICE_MISSING warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.timbertech.com/product/pro-legacy-collection/", RequestID: "timbertech-1",
	},
	{
		Key: "tremco", Summary: "tremcosealants.com (default → ai-crawler-avalai, web search)",
		Description: "Tremco product pages time out for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch). The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.tremcosealants.com/products/tremsil-200", RequestID: "tremco-1",
	},
	{
		Key: "trex", Summary: "trex.com (default → ai-crawler-avalai)",
		Description: "The sample kit price is only in the page's JSON-LD, which the LLM reads. Expect price 500 (USD 5.00), the SKU spec and 5 features, with an AI_EXTRACTED warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.trex.com/products/decking/the-smart-kit/", RequestID: "trex-1",
	},
	{
		Key: "turf", Summary: "turf.design (default → ai-crawler-avalai)",
		Description: "Designer-sold acoustic panels: the NRC and felt specification are read by the LLM. Expect 2 specs, the image, price.state unknown and a PRICE_MISSING warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://turf.design/products/slice", RequestID: "turf-1",
	},
	{
		Key: "usg", Summary: "usg.com (default → ai-crawler-avalai, web search)",
		Description: "USG product pages are protected by bot protection for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.usg.com/en-US/p/product/sheetrock-brand-lay-in-gypsum-ceiling-panels-3260", RequestID: "usg-1",
	},
	{
		Key: "visualcomfort", Summary: "visualcomfort.com (default → ai-crawler-avalai, browser fetch)",
		Description: "The price is in the page's JSON-LD and the rendered page; the LLM reads them after Chrome renders the page (ai_fetch browser). Expect price 72165 cents (USD 721.65), 29 specs and the image. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.visualcomfort.com/us/p/charleston-medium-lantern-5291004", RequestID: "visualcomfort-1",
	},
	{
		Key: "volt", Summary: "voltlighting.com (default → ai-crawler-avalai)",
		Description: "The price is in the page's JSON-LD, which the LLM reads. Expect price 3399 (USD 33.99), 17 specs, 5 features and the image, with an AI_EXTRACTED warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.voltlighting.com/mr11-decorative-rock-light", RequestID: "volt-1",
	},
	{
		Key: "wac", Summary: "waclighting.com (default → ai-crawler-avalai)",
		Description: "Trade-sold lighting: the SKU and dimension tables are read by the LLM. Expect 12 specs, the image, the call-for-price state with a PRICE_MISSING warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://waclighting.com/product/axel/", RequestID: "wac-1",
	},
	{
		Key: "wedi", Summary: "wedi.net (default → ai-crawler-avalai)",
		Description: "Distributor-sold shower systems: the dimensions and order data are read by the LLM. Expect the specs, the image, price.state unknown and a PRICE_MISSING warning. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.wedi.net/en/products/flush-to-floor-showers/fundo-shower-element-with-point-drainage/fundo-plano", RequestID: "wedi-1",
	},
	{
		Key: "westelm", Summary: "westelm.com (default → ai-crawler-avalai, web search)",
		Description: "West Elm product pages are access-denied (403) for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.westelm.com/products/yamazaki-sofa-back-console-table-w-shelf-d18054/", RequestID: "westelm-1",
	},
	{
		Key: "wilsonart", Summary: "wilsonart.com (default → ai-crawler-avalai)",
		Description: "Samples and quotes only (`$0.00` placeholder): expect title, description and the design repeat and finish as specs, with price.state unknown and warnings AI_EXTRACTED and PRICE_MISSING. Needs APP_AI_CRAWLER_ENABLED=true with an AvalAI key.",
		URL:         "https://www.wilsonart.com/beigewood-7850", RequestID: "wilsonart-1",
	},
	{
		Key: "woodworkerssource", Summary: "woodworkerssource.com (default → ai-crawler-avalai, web search)",
		Description: "Woodworkers Source product pages return bot protection for every page fetch from this network. The site is never contacted: the search model answers from the web (sources in source.sources) and the values come back unverified (AI_UNVERIFIED). Expect source.site_contacted false.",
		URL:         "https://www.woodworkerssource.com/walnut/walnut-flat-cut-paper-back-veneer-sheet-4-x-8-roll.html", RequestID: "woodworkerssource-1",
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
