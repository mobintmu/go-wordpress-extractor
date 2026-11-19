# Websites

## Supported

Each site has a config in [`configs/sites/`](../configs/sites/). All are enabled. The engine column is the site's `fetch.engine`. See README "Fetch engines", [ENGINES.md](ENGINES.md) (every engine, including `mojeek`, `yahoo` and `yandex`) and [SEARCH_ENGINE_PROPOSAL.md](SEARCH_ENGINE_PROPOSAL.md).

| # | Website | Platform / theme | Price unit | Engine |
|---|---------|------------------|------------|--------|
| 1 | 3-form.com | 3form's own CMS | USD | `http` |
| 2 | badomjip.com | WooCommerce, WoodMart | toman | `http` |
| 3 | emtek.com | Next.js storefront | USD | `http` |
| 4 | fergusonhome.com | Own React storefront, behind Akamai | USD | `ai-crawler-avalai` with `ai_fetch: web_search` since 2026-11-19 (config 2026-11-19.2): Akamai blocks plain HTTP and Chrome, so the site is never contacted; AvalAI's search model (`sonar`) returns title, price, description and specs, unverified (`AI_UNVERIFIED`). Needs AvalAI on the instance. Until then `duckduckgo`, alternate `brave` (no price, image or specs); those search settings are kept in the config's `$comment` |
| 5 | flooranddecor.com | Salesforce Commerce Cloud | USD | `browser` (config 2026-11-13.1) |
| 6 | hafele.com.de | Intershop, behind Cloudflare | EUR | `browser` (config 2026-11-13.2). On 2026-11-13 it briefly used `duckduckgo`, alternate `brave`, but DuckDuckGo had not indexed the sample product (`SEARCH_NO_RESULT`); the config's `$comment` names the search settings to use if the browser engine stops passing the challenge |
| 7 | hmland.ir | WooCommerce, WoodMart | rial | `http` |
| 8 | hosseinibrothers.ir | PrestaShop, Panda | toman | `http` |
| 9 | msisurfaces.com | ASP.NET Core, custom theme | USD | `http` |
| 10 | sherwin-williams.com | WebSphere Commerce | USD (price not in server HTML) | `http`. Planned: `browser`, after checking it sends Chrome no challenge |
| 11 | shop.interface.com | Salesforce Commerce Cloud | USD | `http` |
| 12 | tilebar.com | Magento 2 | USD | `http` |

## Candidates (no config yet)

1. satrinshop.ir
2. anneshiirley.ir
3. iblaackshop.com
4. elaart.ir
5. farako.com
