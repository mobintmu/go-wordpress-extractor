# Websites

## Supported

Each site has a config in [`configs/sites/`](../configs/sites/). All are enabled. The engine column is the site's `fetch.engine`; `default` means the config follows `APP_DEFAULT_ENGINE` (resolved to `ai-crawler-avalai` on this instance). See README "Fetch engines", [ENGINES.md](ENGINES.md) (every engine, including `mojeek`, `yahoo` and `yandex`) and [SEARCH_ENGINE_PROPOSAL.md](SEARCH_ENGINE_PROPOSAL.md).

| # | Website | Platform / theme | Price unit | Engine |
|---|---------|------------------|------------|--------|
| 1 | 1000bulbs.com | Own platform (Foundation CSS) | USD (per unit) | `http` |
| 2 | 3-form.com | 3form's own CMS | USD | `http` |
| 3 | 3m.com | Akamai-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 4 | abcworldwidestone.com | WordPress, custom theme (portfolio posts) | none (no price) | `http` |
| 5 | acehardware.com | Cloudflare-protected retailer | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 6 | annsacks.kohler.com | Next.js (Kohler), behind Akamai | USD | `default` (resolved to `ai-crawler-avalai`): the page is a JavaScript shell (only JSON-LD carries the product) and headless Chrome times out. Needs AvalAI on the instance |
| 7 | arktura.com | WordPress, custom theme | none (no price) | `http` |
| 8 | badomjip.com | WooCommerce, WoodMart | toman | `http` |
| 9 | bentleymills.com | WooCommerce, Restoration (custom product layout) | none (no price) | `http` |
| 10 | bostik.com | Own CMS | none (no price) | `http` |
| 11 | bravarooftile.com | Cloudflare-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 12 | cosmossurfaces.com | Angular SPA | none (no price) | `http` (head metadata only; headless Chrome failed) |
| 13 | daltile.com | Mohawk Industries brand site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 14 | dap.com | Sitecore | none (no price) | `default` (resolved to `ai-crawler-avalai`): variant table and image are not readable by selectors (about 3.4k tokens per request; `http` gives title and description) |
| 15 | deckorators.com | Bot-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 16 | drytreat.com | PHP site | none (no price) | `browser`: plain HTTP is denied from this network |
| 17 | emser.com | Optimizely (Insite) B2B Commerce, React | none (no price) | `browser`: the server HTML is a JavaScript shell |
| 18 | emtek.com | Next.js storefront | USD | `http` |
| 19 | farrow-ball.com | Magento 2, custom theme | USD | `default` (resolved to `ai-crawler-avalai`): specs are prose the selectors cannot pair (about 4k tokens per request; `http` gives title, price, image and description for free) |
| 20 | fergusonhome.com | Own React storefront, behind Akamai | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` since 2026-11-19 (config 2026-11-19.2): Akamai blocks plain HTTP and Chrome, so the site is never contacted; AvalAI's search model (`sonar`) returns title, price, description and specs, unverified (`AI_UNVERIFIED`). Needs AvalAI on the instance. Until then `duckduckgo`, alternate `brave` (no price, image or specs); those search settings are kept in the config's `$comment` |
| 21 | fleetwoodusa.com | Server-rendered site | none (dealer quote) | `http`: the only route that works (browser and AI page fetch are denied) |
| 22 | flooranddecor.com | Salesforce Commerce Cloud | USD | `browser` (config 2026-11-13.1) |
| 23 | flor.com | Magento-style catalogue behind Cloudflare | USD (per tile) | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 24 | gaf.com | Access-controlled manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 25 | hafele.com.de | Intershop, behind Cloudflare | EUR | `browser` (config 2026-11-13.2). On 2026-11-13 it briefly used `duckduckgo`, alternate `brave`, but DuckDuckGo had not indexed the sample product (`SEARCH_NO_RESULT`); the config's `$comment` names the search settings to use if the browser engine stops passing the challenge |
| 26 | havwoods.com | React, styled-components | none (call for price) | `http` |
| 27 | hermanmiller.com | Access-controlled manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 28 | hmland.ir | WooCommerce, WoodMart | rial | `http` |
| 29 | hosseinibrothers.ir | PrestaShop, Panda | toman | `http` |
| 30 | innovationsusa.com | Own PHP site, Bootstrap | none (trade login) | `http` |
| 31 | kichler.com | Headless CMS, Next.js-style app | USD | `default` (resolved to `ai-crawler-avalai`): the price is only in JSON-LD (about 6k tokens per request; `http` gives title, image and description, no price) |
| 32 | kohler.com | Access-controlled manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 33 | laminart.com | Magento 2 | none (quote only) | `http` |
| 34 | lithofin.com | TYPO3 | none (no price) | `http`: the selectors give title and description (an AI page fetch was tried but not used) |
| 35 | llflooring.com | Salesforce Commerce Cloud | USD (per box) | `http` |
| 36 | lowes.com | Akamai-protected retailer | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 37 | marble.com | Own PHP site (pjax, Bootstrap) | none (quoted per project) | `default` (resolved to `ai-crawler-avalai`): specs and applications are bare text the selectors cannot pair (about 10k tokens per request; `http` gives title, image and description for free) |
| 38 | marvin.com | Server-rendered site | none (dealer quote) | `http` |
| 39 | miracle-sealants.com | WordPress | none (no price) | `http` |
| 40 | moistureshield.com | WordPress | none (no price) | `default` (resolved to `ai-crawler-avalai`): the feature list and warranties are not readable by selectors (about 3.9k tokens per request; `http` gives the title only) |
| 41 | msisurfaces.com | ASP.NET Core, custom theme | USD | `http` |
| 42 | nanawall.com | Bot-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 43 | owenscorning.com | Access-controlled manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 44 | pella.com | Bot-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 45 | phillipjeffries.com | Rails + AngularJS | none (trade login) | `http` |
| 46 | qualitybath.com | Bot-protected retailer | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 47 | rehau.com | Access-controlled manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 48 | richelieu.com | Unreachable distributor site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 49 | roomandboard.com | Access-controlled retailer | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 50 | schluter.com | Hybris-style store | none (no price) | `http` |
| 51 | schumacher.com | AWS WAF-protected storefront | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 52 | shawfloors.com | Own ASP.NET + server-rendered React | USD (per sq ft) | `http` |
| 53 | sherwin-williams.com | WebSphere Commerce | USD (price not in server HTML) | `http`. Planned: `browser`, after checking it sends Chrome no challenge |
| 54 | shop.interface.com | Salesforce Commerce Cloud | USD | `http` |
| 55 | soundseal.com | Server-rendered CMS | none (contact sales) | `http` |
| 56 | steelcase.com | Next.js-style storefront | USD | `default` (resolved to `ai-crawler-avalai`) |
| 57 | stonelandusa.com | WooCommerce, Astra (custom product block) | USD | `default` (resolved to `ai-crawler-avalai`): the price is only in the JSON-LD (the visible page says Get Quote); `http` gives call_for_price |
| 58 | techo-bloc.com | Bot-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 59 | thermatru.com | Access-controlled manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 60 | tilebar.com | Magento 2 | USD | `http` |
| 61 | timbertech.com | Server-rendered site | none (no price) | `default` (resolved to `ai-crawler-avalai`): the collection specs are not readable by selectors (about 4.9k tokens per request; `http` gives title, image and description) |
| 62 | tremcosealants.com | Unreachable manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 63 | trex.com | Server-rendered site | USD | `default` (resolved to `ai-crawler-avalai`): the price is only in JSON-LD (about 4.8k tokens per request; `http` gives title and image, no price) |
| 64 | turf.design | Next.js on Sanity | none (no price) | `default` (resolved to `ai-crawler-avalai`): the technical specs are not read by selectors; plain HTTP times out from the server (about 2.6k tokens per request; `browser` also works without specs) |
| 65 | usg.com | Bot-protected manufacturer site | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 66 | veneersupplies.com | Own store platform | USD (per sheet) | `default` (resolved to `ai-crawler-avalai`) |
| 67 | visualcomfort.com | Custom storefront | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: browser`: the page's specs and price need Chrome rendering (about 10.7k tokens per request) |
| 68 | voltlighting.com | Custom storefront | USD | `default` (resolved to `ai-crawler-avalai`): the price is only in JSON-LD and the specs are not readable by selectors (about 7k tokens per request; `http` gives title, image and description) |
| 69 | waclighting.com | WordPress | none (no price) | `default` (resolved to `ai-crawler-avalai`): the SKU and dimension tables are not readable by selectors (about 3.9k tokens per request; `http` gives title and description) |
| 70 | walkerzanger.com | WooCommerce, own walkerzanger theme | none (no price) | `http` |
| 71 | wedi.net | TYPO3 | none (no price) | `default` (resolved to `ai-crawler-avalai`): the dimension and order tables are not readable by selectors (about 8.3k tokens per request; `http` gives title, image and description) |
| 72 | westelm.com | Access-controlled retailer | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |
| 73 | wilsonart.com | Magento 2, custom template | none (quote only) | `default` (resolved to `ai-crawler-avalai`): repeat and finish are text the selectors cannot pair (about 2.6k tokens per request; `http` gives title and description) |
| 74 | woodworkerssource.com | Bot-protected retailer | USD | `default` (resolved to `ai-crawler-avalai`) with `ai_fetch: web_search` (values unverified) |

## Candidates (no config yet)

1. satrinshop.ir
2. anneshiirley.ir
3. iblaackshop.com
4. elaart.ir
5. farako.com
