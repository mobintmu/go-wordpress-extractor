# Websites

## Supported

Each site has a config in [`configs/sites/`](../configs/sites/). All are enabled. The engine column is the site's `fetch.engine`. See README "Fetch engines" and [SEARCH_ENGINE_PROPOSAL.md](SEARCH_ENGINE_PROPOSAL.md).

| # | Website | Platform / theme | Price unit | Engine |
|---|---------|------------------|------------|--------|
| 1 | 3-form.com | 3form's own CMS | USD | `http` |
| 2 | badomjip.com | WooCommerce, WoodMart | toman | `http` |
| 3 | emtek.com | Next.js storefront | USD | `http` |
| 4 | fergusonhome.com | Own React storefront, behind Akamai | USD | `duckduckgo`, alternate `brave`. The site is not contacted, so there is no price, image or specs |
| 5 | flooranddecor.com | Salesforce Commerce Cloud | USD | `http` |
| 6 | hafele.com.de | Intershop, behind Cloudflare | EUR | `duckduckgo`, alternate `brave`. The site is not contacted. Not every product is indexed (`SEARCH_NO_RESULT`) |
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
