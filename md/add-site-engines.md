# Engine per website

Generated from `configs/sites/*.json` on 2026-11-22. Engine `http` is the default; `default` follows `APP_DEFAULT_ENGINE` (resolved to `ai-crawler-avalai` on this instance). Descriptions explain the non-default engines.

| Website | Site ID | Engine | Description |
|---|---|---|---|
| 1000bulbs.com | 1000bulbs | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| 3-form.com | 3form | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| 3m.com | 3m | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| abcworldwidestone.com | abcworldwidestone | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| acehardware.com | acehardware | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| annsacks.kohler.com | annsacks | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| arktura.com | arktura | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| badomjip.com | badomjip | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| bentleymills.com | bentleymills | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| bostik.com | bostik | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| bravarooftile.com | bravarooftile | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| cosmossurfaces.com | cosmossurfaces | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| daltile.com | daltile | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| dap.com | dap | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| deckorators.com | deckorators | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| drytreat.com | drytreat | `browser` | Product page is rendered by JavaScript or blocks plain HTTP from this network; headless Chrome renders it (APP_BROWSER_ENABLED). |
| emser.com | emser | `browser` | Product page is rendered by JavaScript or blocks plain HTTP from this network; headless Chrome renders it (APP_BROWSER_ENABLED). |
| emtek.com | emtek | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| farrow-ball.com | farrowball | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| fergusonhome.com | fergusonhome | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| fleetwoodusa.com | fleetwood | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| flooranddecor.com | flooranddecor | `browser` | Product page is rendered by JavaScript or blocks plain HTTP from this network; headless Chrome renders it (APP_BROWSER_ENABLED). |
| flor.com | flor | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| gaf.com | gaf | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| hafele.com.de | hafele | `browser` | Product page is rendered by JavaScript or blocks plain HTTP from this network; headless Chrome renders it (APP_BROWSER_ENABLED). |
| havwoods.com | havwoods | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| hermanmiller.com | hermanmiller | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| hmland.ir | hmland | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| hosseinibrothers.ir | hosseinibrothers | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| innovationsusa.com | innovationsusa | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| kichler.com | kichler | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| kohler.com | kohler | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| laminart.com | laminart | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| lithofin.com | lithofin | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| llflooring.com | llflooring | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| lowes.com | lowes | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| lumens.com | lumens | `duckduckgo` |  |
| marble.com | marble | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| marvin.com | marvin | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| miracle-sealants.com | miracle | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| moistureshield.com | moistureshield | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| msisurfaces.com | msisurfaces | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| nanawall.com | nanawall | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| owenscorning.com | owenscorning | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| pella.com | pella | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| phillipjeffries.com | phillipjeffries | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| qualitybath.com | qualitybath | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| rehau.com | rehau | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| richelieu.com | richelieu | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| roomandboard.com | roomandboard | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| schluter.com | schluter | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| schumacher.com | schumacher | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| shawfloors.com | shawfloors | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| sherwin-williams.com | sherwin-williams | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| shop.interface.com | interface | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| soundseal.com | soundseal | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| steelcase.com | steelcase | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| stonelandusa.com | stonelandusa | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| techo-bloc.com | techo-bloc | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| thermatru.com | thermatru | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| tilebar.com | tilebar | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| timbertech.com | timbertech | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| tremcosealants.com | tremco | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| trex.com | trex | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| turf.design | turf | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| usg.com | usg | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| veneersupplies.com | veneersupplies | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| visualcomfort.com | visualcomfort | `default` (resolved to `ai-crawler-avalai`) + `ai_fetch: browser` | The price or specs need Chrome to render the page before the LLM reads it (ai_fetch browser). Needs AvalAI and APP_BROWSER_ENABLED; costs tokens per request. |
| voltlighting.com | volt | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| waclighting.com | wac | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| walkerzanger.com | walkerzanger | `http` | Page is server-rendered; the free http engine returns the full data (no price, specs or images when the page does not carry them). |
| wedi.net | wedi | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| westelm.com | westelm | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |
| wilsonart.com | wilsonart | `default` (resolved to `ai-crawler-avalai`) | Needs the LLM to read values the selectors cannot pair (JSON-LD-only prices, specs in tables or prose). Needs AvalAI and costs tokens per request. |
| woodworkerssource.com | woodworkerssource | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` | Blocked for every page fetch from this network; the search model answers from the web. Values are unverified (AI_UNVERIFIED) and the shop is never contacted. Needs AvalAI. |

## Parked, skipped and open

| Website | Status / reason |
|---|---|
| homedepot.com | PARKED needs saved page. Akamai: http/browser(headless)/avalai-http ACCESS_DENIED; ddg partial(title+snippet); mojeek/yandex blocked, yahoo error; web_search SEARCH_NO_RESULT (614+117 tok); cloudflare not configured. sample=https://www.homedepot.com/p/Lifeproof-Shenandoah-Oak-1-4-in-T-x-6-5-in-W-Click-Lock-Engineered-Hardwood-Flooring-21-67-sq-ft-case-VTWHDSHEO6-5X48/314745855 |
| lowes.com | PARKED needs saved page. Akamai blocks http/browser/avalai-http; ddg partial; web_search partial+16 specs unverified state=out_of_stock (602+527 tok). sample=https://www.lowes.com/pd/allen-roth-Nutmeg-Hickory-3-8-in-T-x-5-in-W-Handsculpted-Engineered-Hardwood-Flooring-32-29-sq-ft-Carton/50223071 |
| build.com | SKIP: robots.txt 301-redirects to fergusonhome.com (already configured); home 403 Akamai |
| mohawkflooring.com | PARKED all engines fail: connection reset/FETCH_TIMEOUT for http/browser/avalai; ddg+web_search SEARCH_NO_RESULT (585+89 tok). Likely geo/IP block. sample=https://www.mohawkflooring.com/shop/Laminated_Wood/detail/CDL45/Adler_Creek/01/Pale_Oak?productLine=RevWood+Plus |
| flooret.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| theflooringfactory.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| artistictile.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| ziatile.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| yorkwallcoverings.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| burkedecor.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| backdrop.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| portolapaints.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| limewash.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| coloratelierpaint.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| thewoodveneerhub.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| naturewall.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| woodupp.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| woodcraft.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| grohe.us | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| kanopibyarmstrong.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| designwithinreach.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| fiberondecking.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| fx-luminaire.com | PARKED DNS NXDOMAIN on this network (Iran filter 10.10.34.36) |
| contractorsdirect.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| tiletools.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| tools4flooring.com | PARKED DNS FILTERED on this network (Iran filter 10.10.34.36) |
| flor.com | PARKED Cloudflare 'Attention Required' on http/browser/avalai (BOT_PROTECTION); ddg no result; web_search partial $20.00 +5 specs unverified (574+297 tok). sample=https://www.flor.com/area-rugs_carpet-tiles/solids/made-you-look/21-1361.html |
| shawcontract.com | SKIP robots.txt: User-agent * Disallow: / (only named search bots allowed). Service also returns ROBOTS_DISALLOWED. web_search would answer (9 specs unverified, 560+273 tok) but no page may be fetched. |
| milliken.com | PARKED Cloudflare 'Attention Required' 403 even on robots.txt; manufacturer, web search finds only collection pages and PDFs, no product page; engines not run |
| patcraft.com | SKIP robots.txt: User-agent * Disallow: / |
| mohawkgroup.com | PARKED connection reset on robots.txt (same Mohawk infra as mohawkflooring.com, where every engine failed); engines not run |
| daltile.com | PARKED (Mohawk Industries) http/avalai FETCH_TIMEOUT, browser ACCESS_DENIED, search blocked; web_search partial 23 specs unverified (561+540 tok). sample=https://www.daltile.com/products/concrete-look/portfolio/white |
| porcelanosa-usa.com | PARKED redirects to store.porcelanosa.com/us/, Google Cloud 403 'client does not have permission' (geo-block) even on robots.txt; engines not run |
| bedrosians.com | PARKED robots.txt 403 with captcha page; engines not run |
| wallcoverings.com | SKIP domain now redirects to blindsreview.com (not a shop) |
| flavorpaper.com | PARKED robots.txt 403; engines not run |
| wallanddeco.com | PARKED no response (000) on robots.txt; engines not run |
| wallpaperdirect.com | PARKED robots.txt 403 Forbidden; engines not run |
| benjaminmoore.com | PARKED Akamai 'Access Denied' on robots.txt; engines not run |
| behr.com | PARKED Cloudflare JS challenge ('Just a moment') on robots.txt; browser engine might pass; engines not run |
| ekenamillwork.com | PARKED Cloudflare JS challenge ('Just a moment'); engines not run |
| trustile.com | PARKED Cloudflare JS challenge ('Just a moment'); engines not run |
| stone-tile.com | PARKED ScandiPWA/Magento SPA: http JS_RENDERED_PAGE, browser+avalai FETCH_TIMEOUT (responses stall at ~38 KB), web_search SEARCH_NO_RESULT (559+64 tok); US_EN store serves the CA home page. sample=https://stone-tile.com/CA_EN/solo-field.html |
| arizonatile.com | PARKED robots.txt Crawl-delay: 600 s (WordPress). Service ignores Crawl-delay and schema caps delay_ms at 60 s; discovery + trials would take ~80 min of waiting. Needs a decision. |
| wallquest.com | PARKED nopCommerce; robots.txt OK but home page Cloudflare 'Attention Required' 403; engines not run |
| schumacher.com | PARKED AWS WAF JS challenge (HTTP 202) on robots/home/products: http+avalai EMPTY_PAGE; headless Chrome passes the challenge but the MUI app never renders the product (only drawer h1s 'Dimensions/Availability/Features', no product name) so its 'success' is wrong; search blocked. sample=https://schumacher.com/catalog/products/5005854 |
| romabio.com | SKIP WooCommerce shop in 'Coming soon' mode: product pages (3 in wp-sitemap) show only 'Great things are on the horizon' |
| alucobondusa.com | SKIP manufacturer marketing pages (JS shell, title+meta description only; no product name/image/price/specs in markup) |
| 3m.com | AI-ROUTE pending: Akamai 'Access Denied' on product page (3M/en_US/p/d/b00008068) |
| advantagelumber.com | AI-ROUTE pending: buy.advantagelumber.com (Shopify) timed out on direct fetch |
| americannationallighting.com | RECHECK: search found homepage only; no product URLs found yet |
| allmodern.com | RECHECK: 429 rate limit on category pages (retry) |
| ceilume.com | SKIP product URLs are tile-family listings (many tiles, prices in listing), not single-product pages |
| baldwinhardware.com | SKIP style-level portfolio/marketing pages (no per-product price/specs) |
| cambriausa.com | SKIP sitemap has inspiration/design pages only; no product pages found |
| custombuildingproducts.com | SKIP page-sitemap only, marketing/how-to pages |
| belgard.com | RECHECK product pages (taxonomy product_application/categories); no product sitemap found yet |
| ardexamericas.com | RECHECK product pages (only blog/news in post-sitemap) |
| certainteed.com | RECHECK product pages (sitemap mostly inspiration/how-to) |
| caesarstoneus.com | RECHECK (no sitemap; search needed) |
| corian.com | RECHECK (no sitemap; search needed) |
| bostik.com | CANDIDATE http: product-portfolio pages (BOSTIK A535 / R3BOND) with h1 and image; no price |
| soelbergi.com | SKIP Divi marketing/collection pages; product copy only, no price or specs |
| frasch.com | SKIP landing pages (LINYFELT meet page), resources; no product data markup |
| masonite.com | SKIP pro-blog/resource pages in sitemap; no product catalogue pages found |
| simpsondoor.com | RECHECK sitemap is careers/tools; find real door product pages |
| modulararts.com | RECHECK no sitemap found |
| hafele.com | RECHECK no sitemap found (hafele.com.de already configured separately) |
| veneersupplies.com | RECHECK no sitemap found |
| formica.com | RECHECK content sitemap is articles/about; product pages unknown |
| wilsonart.com | RECHECK product pages not yet located (1389 urls) |
| 9wood.com | SKIP blog/post pages only (post-sitemap); no product pages |
| andersenwindows.com | SKIP marketing and selection-guide pages; no online prices or product data in markup |
| abc-supply.com | SKIP no product pages found (no sitemap; search empty) |
| aquamix.com | SKIP no product pages found (search empty) |
| beaconproplus.com | SKIP product information is PDF datasheets only; no product HTML pages found |
| boltdepot.com | SKIP catalogue category and filter pages only; no per-item page found |
| acehardware.com | CANDIDATE caulk product pages with price (search found 9); robots 403 'Attention Required' on direct fetch |
| armstrongceilings.com | CANDIDATE commercial ceiling tile product pages (specs); robots 403 'Access denied' on direct fetch |
| bravarooftile.com | CANDIDATE product pages for composite roof tile lines; robots 403 'Just a moment' |
| cabinetparts.com | CANDIDATE drawer slide product pages with prices; robots 403 'Just a moment' |
| cb2.com | SKIP no product page URL found by search (category pages only) |
| davinciroofscapes.com | SKIP product family pages (slate lines), no per-product pages found |
| dekton.com | SKIP colour and collection pages only |
| dap.com | CANDIDATE product pages (adhesives) with specs table; test http |
| drytreat.com | CANDIDATE product pages (sealers) with data sheets; test http |
| deckorators.com | CANDIDATE product-line pages (Voyage, Vista, Summit...) with specs; test http |
| fastenal.com | CANDIDATE product detail pages with prices; robots 403 earlier |
| faucetdirect.com | CANDIDATE product pages (/p<id>) with prices; test |
| ferguson.com | CANDIDATE product pages (/product/...html) ; test |
| fila.com | SKIP search found policy/FAQ pages only; no sealer product pages |
| hunzalighting.com | SKIP blog posts only (post-sitemap) |
| iko.com | SKIP project posts only (post-sitemap) |
| interface.com | SKIP duplicate: shop.interface.com is already configured |
| jameshardie.com | SKIP marketing, forms and support pages only |
| grainger.com | PARKED (final): direct page 'unable to complete your request' (bot block); AI http BOT_PROTECTION; web_search no result |
| acousticalsurfaces.com | SKIP blog and marketing pages only |
| lacantinadoors.com | SKIP learn, blog and project pages only; no product pages found |
| laticrete.com | SKIP product information is TDS PDFs and catalogue pages; no product pages with structured data found |
| fiberon.com | SKIP search returned a fibre-optics company, not the decking brand (decking is fiberondecking.com) |
| lightology.com | SKIP category and listing pages with price ranges only; no per-product page found |
| mapei.com | SKIP documents-led (TDS PDFs); product pages not surfaced |
| knoll.com | PARKED (final): direct, browser and AI page fetch denied; web_search no result |
| lampsplus.com | PARKED (final): direct, browser and AI page fetch denied; web_search no result |
| neolith.com | SKIP collection and store-locator pages only |
| newtechwood.com | SKIP blog posts only; no product pages in sitemap |
| nicocorp.com | SKIP posts and FAQ pages only |
| nicolock.com | SKIP blog posts only |
| owenscorning.com | AI-ROUTE pending: 403 ERROR block page; http/browser ACCESS_DENIED; web_search ok |
| moistureshield.com | AI-ROUTE pending: live AI http answer verified (3 specs, 7 features); http title-only |
| mcmaster.com | SKIP product page timed out on direct fetch; 289k catalogue, dynamic item pages not verified |
| ppgpaints.com | SKIP colour reference pages (RGB/LRV), not purchasable products |
| marvin.com | CANDIDATE http (direct 200; large page) |
| robertsconsolidated.com | PARKED (final): product page redirects to a host the app refuses (REDIRECT_NOT_ALLOWED); web_search no result |
| perigold.com | SKIP listing and category pages only (product URLs not found) |
| sika.com | SKIP knowledge-hub articles only |
| systempavers.com | SKIP blog posts only |
| royalbuildingproducts.com | PARKED (final): direct, browser and AI page fetch denied; web_search no result |
| steelcase.com | AI-ROUTE pending: AI page fetch returns JSON-LD price 1659, image, desc |
| signaturehardware.com | PARKED (final): direct and AI page fetch time out; browser times out; web_search no result |
| supplyhouse.com | PARKED (final): bot protection on direct, browser and AI page fetch; web_search no result |
| silestoneusa.com | PARKED (final): product page redirect refused (REDIRECT_NOT_ALLOWED) or connection failed; web_search no result |
| toto.com | SKIP search returned EU and Vietnam pages only; no US product page found |
| trespa.com | SKIP family and inspiration pages only (no per-product pages found) |
| unilock.com | SKIP blog and project posts only |
| turf.design | CANDIDATE product pages (Slice, Petal) with specs; test |
| thermoryusa.com | PARKED: the server stalls the page body at about 57 KB for curl (and for the AI page fetch: AI answer was complete but no complete saved page can be made here); need a full saved page to build the fixture (AI route would then return 15 specs) |
| wayfair.com | PARKED: product pages rate-limited (429) and no product URL found by search (listings only) |
| zoro.com | SKIP listings and category pages only; no product URL found |
| westernwindowsystems.com | PARKED (final): page fetch redirect refused / connection failed on every route; web_search no result |
| ylighting.com | PARKED (final): product pages render as JavaScript shells (JS_RENDERED_PAGE) on HTTP and browser; AI browser also JS_RENDERED_PAGE; web_search no result |
| DNS-filtered group (17 domains) | Not configured yet: reachable through the proxy, but the app's SSRF guard now allows proxied hostnames; Shopify product pages are rate-limited (429) from this IP. See md/OUTBOUND_PROXY.md |
