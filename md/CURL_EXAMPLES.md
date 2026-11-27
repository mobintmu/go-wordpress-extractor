# curl examples, one per website

One `POST /api/v1/extract` call for each of the 75 supported sites (see [websites.md](websites.md)). The URLs are the sample products from `testdata/sites/`. Some fixtures are hand-made, so if a call returns `HTTP_NOT_FOUND`, swap in a current product URL from that site.

## Setup

```bash
export API=http://127.0.0.1               # APP_HTTP_ADDRESS / APP_HTTP_PORT
export KEY=gwp_..._...                    # a client key, see SETUP_KEYS.md
```

Every example uses the same shape:

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "<product url>", "request_id": "<your id>"}'
```

- `request_id` is optional (max 128 chars). It is echoed back.
- Both outcomes return HTTP 200. Read the `status` field (`success` or `failed`).
- Add `| jq .` to pretty-print, or `| jq '{status, completeness, unavailable_fields, error: .error.code}'` for a short summary.

Check which sites and config versions the server has loaded:

```bash
curl -sS "$API/api/v1/sites" -H "X-API-Key: $KEY" | jq .
```

## Sites using the `http` engine

These fetch the product page directly. `source.site_contacted` is `true`.

### 1. 1000bulbs.com

Prices are in US dollars per unit (`$21.55 ea.` → `2155` cents).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.1000bulbs.com/product/37/STAG-FEP.html", "request_id": "1000bulbs-1"}'
```

### 2. 3-form.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.3-form.com/products/edge-lit-chroma-countertop", "request_id": "3form-1"}'
```

### 4. abcworldwidestone.com

Slab supplier with no price on the page: expect `price.state` `unknown` with a `PRICE_MISSING` warning, and the standard Dimensions/Thickness/Finish notes as specs.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://abcworldwidestone.com/portfolio/gris-macael-dusted/", "request_id": "abcworldwidestone-1"}'
```

### 7. arktura.com

Product-line page with no price (architect-specified): expect title, image and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://arktura.com/vapor-exterior/", "request_id": "arktura-1"}'
```

### 8. badomjip.com

Prices are in toman (`IRT`). The site has call-for-price, out-of-stock and variable products.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://badomjip.com/product/almondbutter400/", "request_id": "badomjip-1"}'
```

### 9. bentleymills.com

Commercial mill with no price on the page: expect `price.state` `unknown` and a `PRICE_MISSING` warning. Specs come from the SPECS panel.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.bentleymills.com/product/4-agnona/", "request_id": "bentleymills-1"}'
```

### 10. bostik.com

Distributor-only products: expect title, image and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.bostik.com/global/en/bostik-academy/library/product-portfolio/decoration-paint/a535/", "request_id": "bostik-1"}'
```

### 12. cosmossurfaces.com

Angular app: the fields come from the page's `<head>` metadata (`og:title`, `og:image`, meta description), so the title carries the SEO prefix and suffix. No price: expect `price.state` `unknown` with a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.cosmossurfaces.com/washington/granite/absolute-black-premium-3cm", "request_id": "cosmossurfaces-1"}'
```

### 16. drytreat.com

Blocks plain HTTP requests from this network; headless Chrome renders the page. Expect title and description, `price.state` `unknown` and a `PRICE_MISSING` warning. Needs `APP_BROWSER_ENABLED=true`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.drytreat.com/sealers/residential/stain-proof-waterborne/", "request_id": "drytreat-1"}'
```

### 18. emtek.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.emtek.com/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/", "request_id": "emtek-1"}'
```

### 21. fleetwoodusa.com

Dealer-quoted products: expect title, description and the Standard Features table as specs, with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.fleetwoodusa.com/products/3-series/series-3070/", "request_id": "fleetwood-1"}'
```

### 26. havwoods.com

Trade supplier with no price on the page (`Request pricing`): expect `price.state` `call_for_price` with `price.current` null, and `availability` in `unavailable_fields`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.havwoods.com/us/products/hw3699/", "request_id": "havwoods-1"}'
```

### 28. hmland.ir

The site shows rial. The response converts to toman (`currency: IRT`). Persian slugs must be sent percent-encoded, as below.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://hmland.ir/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c/", "request_id": "hmland-1"}'
```

### 29. hosseinibrothers.ir

Prices are in toman (`IRT`). Percent-encoded Persian path.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://hosseinibrothers.ir/%DA%A9%D8%B4%D9%85%D8%B4/%DA%A9%D8%B4%D9%85%D8%B4-%D9%BE%D9%84%D9%88%DB%8C%DB%8C-%D8%A2%D9%81%D8%AA%D8%A7%D8%A8%DB%8C", "request_id": "hosseinibrothers-1"}'
```

### 30. innovationsusa.com

Trade-only prices: expect title, image, description and 9 specs with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.innovationsusa.com/item/antwerp", "request_id": "innovationsusa-1"}'
```

### 33. laminart.com

Samples and quotes only: expect title, image, description and attribute specs with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.laminart.com/coal-706", "request_id": "laminart-1"}'
```

### 35. llflooring.com

The price is the box price (`$51.75` → `5175` cents, USD); `price.raw` also shows the per-square-foot price.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.llflooring.com/p/coreluxe-5mm-w-pad-daydream-meadow-oak-6-in-wide-x-48-in-long-10055236/", "request_id": "llflooring-1"}'
```

### 39. marvin.com

Dealer-quoted windows and doors: expect title and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.marvin.com/products/windows/double-hung/elevate-double-hung-window", "request_id": "marvin-1"}'
```

### 40. miracle-sealants.com

Retailer-sold cleaners: expect title, image and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://miracle-sealants.com/miracle-sealants-porcelain-ceramic-tile-cleaner/", "request_id": "miracle-1"}'
```

### 42. msisurfaces.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/", "request_id": "msisurfaces-1"}'
```

### 46. phillipjeffries.com

Trade-only prices and stock: expect title, image and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.phillipjeffries.com/shop/CARD-464/46400", "request_id": "phillipjeffries-1"}'
```

### 51. schluter.com

Distributor-sold membranes: expect title, image and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.schluter.com/schluter-us/en_US/Membranes/Uncoupling-(DITRA)/Schluter%C2%AE-DITRA-PS/p/DITRA_PS", "request_id": "schluter-1"}'
```

### 53. shawfloors.com

The price is the per-square-foot list price (`$4.99 SF` → `499` cents, USD). Shaw sells through retailers, so there is no stock signal: `availability` is in `unavailable_fields` and `completeness` is `partial`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://shawfloors.com/en-us/laminate/oak-grove-hazelwood/sl119-02039", "request_id": "shawfloors-1"}'
```

### 54. sherwin-williams.com

The price is loaded by JavaScript, so the `http` engine gets no price. Expect `PRICE_MISSING` and `price` in `unavailable_fields` with reason `NOT_ON_PAGE`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.sherwin-williams.com/homeowners/products/emerald-interior-acrylic-latex-paint?colorPartNumber=SW6617&colorType=PALETTE", "request_id": "sherwin-williams-1"}'
```

### 55. shop.interface.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://shop.interface.com/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043R.html", "request_id": "interface-1"}'
```

### 56. soundseal.com

Contact-for-price panels: expect title, image and description with `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.soundseal.com/architectural-interiors/products/acoustical-wall-panels/s-4200-tuned-absorbers", "request_id": "soundseal-1"}'
```

### 61. tilebar.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.tilebar.com/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html", "request_id": "tilebar-1"}'
```

### 67. veneersupplies.com

Prices are in US dollars per sheet (`$16.54` → `1654` cents), sold from stock.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.veneersupplies.com/products/Flat-Cut-Rosewood-Bolivian-Veneer-Sheet--55-x-71.html", "request_id": "veneersupplies-1"}'
```

### 71. walkerzanger.com

Showroom brand with no price for anonymous visitors: expect `price.state` `unknown` with a `PRICE_MISSING` warning. The description is generic category copy.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.walkerzanger.com/product/18x48x1-1-2-adirondack-grey-coping-natural/", "request_id": "walkerzanger-1"}'
```

## Sites using the `browser` engine

These load the product page in real Chrome through Playwright, so the page's JavaScript runs and Cloudflare challenges that only check for a JavaScript-capable browser are passed. `source.engine` is `browser` and `source.site_contacted` is `true`. The server needs `APP_BROWSER_ENABLED=true`, otherwise the call fails with `ENGINE_UNAVAILABLE`. A browser fetch takes several seconds, so keep `--max-time` high.

### 17. emser.com

The product page is rendered by JavaScript (Optimizely B2B React storefront), so this needs `APP_BROWSER_ENABLED=true`. Emser sells through distributors: expect `price.state` `unknown` with a `PRICE_MISSING` warning, and 41 specs in four groups.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.emser.com/product/anvaya-f44anvapo1212m", "request_id": "emser-1"}'
```

### 22. flooranddecor.com

Cloudflare blocked the plain `http` engine with "Sorry, you have been blocked" (HTTP 403). If the browser gets the same page, the block is on the server's IP and the call fails with `BOT_PROTECTION`; then switch the site to a search engine or route it through a different IP (`APP_PROXY_*`).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.flooranddecor.com/ceramic-tile/zellige-oat-high-gloss-ceramic-tile-101130060.html", "request_id": "flooranddecor-1"}'
```

### 25. hafele.com.de

Cloudflare answers a plain HTTP client with a "Just a moment..." JavaScript challenge, which the browser runs. Prices are hidden from anonymous visitors, so expect `price.state: unknown` with a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/", "request_id": "hafele-1"}'
```

### 37. lumens.com

The product page is a JavaScript app (the price appears about 11 seconds after load) behind a Cloudflare Turnstile challenge. Headless Chrome does not pass it, so the server needs `APP_BROWSER_HEADLESS=false` and a residential proxy (`APP_PROXY_*`); without them expect `BOT_PROTECTION`. The site config allows 90 seconds per fetch.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.lumens.com/n701-round-corner-sofa-by-ethnicraft-ETH2111288.html", "request_id": "lumens-1"}'
```

Short summary:

```bash
... | jq '{status, engine: .source.engine, title: .product.title, price: .product.price, error: .error.code}'
```

## Sites using a search engine (`duckduckgo`, alternate `brave`)

The site itself is not contacted (`source.site_contacted` is `false`). Only title, URL and description come from the search result. Image, price and specs are listed in `unavailable_fields` with reason `NOT_IN_SEARCH_RESULTS`, and `completeness` is `partial`. At least one of the site's engines (`fetch.engine`, then `fetch.alternate_engines`) must be enabled on the instance (`APP_DUCKDUCKGO_ENABLED`, `APP_BRAVE_ENABLED`, `APP_MOJEEK_ENABLED`, `APP_YAHOO_ENABLED` or `APP_YANDEX_ENABLED`), otherwise the call fails with `ENGINE_UNAVAILABLE`. The response looks the same for every search engine; `source.engine` names the one that answered.

fergusonhome.com was served this way until 2026-11-19; it now uses `default` (`APP_DEFAULT_ENGINE`, resolved to `ai-crawler-avalai` here; example 20 below). `configs/sites/lumens.com.json` sets `duckduckgo` (alternate `brave`) too.

## Sites using the default AI engine in their config (`default`, resolved to `ai-crawler-avalai` on this instance)

The site config picks the engine, or follows `APP_DEFAULT_ENGINE` with `fetch.engine: default`, so the request is the same as for any site. With `ai_fetch` `http` or `browser` the page is fetched, cleaned and read by the LLM, and every value is checked against the page. With `ai_fetch: web_search` the site is never contacted: only the URL goes to the search model (`APP_AI_CRAWLER_SEARCH_MODEL`, `sonar`), and the values come back unverified. `source` adds `model`, `prompt_version`, `fetched_with`, `tokens` and, for web search, `sources`. Needs `APP_AI_CRAWLER_ENABLED=true` with an AvalAI key (`APP_DEFAULT_ENGINE=ai-crawler-avalai`); otherwise `ENGINE_UNAVAILABLE`.

### 3. 3m.com

3M's product pages are behind an Akamai 'Access Denied' block for every page fetch from this network (plain HTTP, headless Chrome, and AvalAI's page fetch in browser mode).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.3m.com/3M/en_US/p/d/b00008068/", "request_id": "3m-1"}'
```

### 5. acehardware.com

Ace Hardware product pages answer 'Attention Required' or bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.acehardware.com/departments/paint-and-supplies/caulk-and-sealants/caulk/1912302", "request_id": "acehardware-1"}'
```

### 6. annsacks.kohler.com

The product page is a Next.js shell whose only product data is a JSON-LD block, so the config sets `fetch.engine` `default` (`APP_DEFAULT_ENGINE`, `ai-crawler-avalai` on this instance; with a Chrome User-Agent, since the default one is tarpitted). Expect `source.engine` `ai-crawler-avalai`, the price in USD and an `AI_EXTRACTED` warning. Needs `APP_AI_CRAWLER_ENABLED=true` with an AvalAI key and `APP_DEFAULT_ENGINE=ai-crawler-avalai`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://annsacks.kohler.com/p/trim/savoy-0-5-x-8-box-liner-trim-as11682-trim-boxliner", "request_id": "annsacks-1"}'
```

### 11. bravarooftile.com

Brava composite roof tiles are behind bot protection ('Just a moment') for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.bravarooftile.com/roof-tiles/barrel-tile/", "request_id": "bravarooftile-1"}'
```

### 13. daltile.com

Daltile (Mohawk Industries) pages time out or are denied for every page fetch from this network, including the AI engine's page fetch.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.daltile.com/products/concrete-look/portfolio/white", "request_id": "daltile-1"}'
```

### 14. dap.com

Retail product pages: the variant table (SKU, unit size, colour) and the image are read by the LLM. Expect `price.state` `unknown` with `PRICE_MISSING`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.dap.com/products/adhesives/weldwood-landau-top-and-trim-1145-high-heat-resistant-contact-cement", "request_id": "dap-1"}'
```

### 15. deckorators.com

Deckorators product pages answer bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.deckorators.com/products/voyage-decking", "request_id": "deckorators-1"}'
```

### 19. farrow-ball.com

US store (USD). The config sets `fetch.engine` `default` (`APP_DEFAULT_ENGINE`, `ai-crawler-avalai` on this instance) because the roll sizes and colours are prose only the LLM can pair. Expect `price.current` `27000`, 5 specs and an `AI_EXTRACTED` warning. Paint pages have no server-side price (`PRICE_MISSING`).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.farrow-ball.com/us/wallpaper/five-over-stripe", "request_id": "farrowball-1"}'
```

### 20. fergusonhome.com

Ferguson is behind Akamai and answers 403 to plain HTTP and to Chrome, so the config sets `fetch.engine: default` (`APP_DEFAULT_ENGINE`, `ai-crawler-avalai` on this instance) and uses `ai_fetch: web_search`; the site is never contacted. Expect `site_id: fergusonhome`, `source.engine: ai-crawler-avalai`, `source.fetched_with: web_search`, `source.site_contacted: false`, `source.sources` (a fergusonhome.com page) and the warnings `AI_EXTRACTED` and `AI_UNVERIFIED`. On 2026-11-19 it returned the title, price 15666 (USD cents, `$156.66`), the description and the specs; the image was not in the answer (`NOT_IN_SEARCH_RESULTS`). The values are not checked against the page, so price and stock may be out of date. With no source on fergusonhome.com it fails with `SEARCH_NO_RESULT`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155", "request_id": "ferguson-1"}'
```

Short summary:

```bash
... | jq '{status, site_id, completeness, source, title: .product.title, price: .product.price, specs: (.product.specs | length), warnings: [.warnings[].code], unavailable: .unavailable_fields, error: .error.code}'
```

### 23. flor.com

FLOR (modular carpet tiles) is behind a Cloudflare block page for every page fetch from this network, including headless Chrome.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.flor.com/area-rugs_carpet-tiles/solids/made-you-look/21-1361.html", "request_id": "flor-1"}'
```

### 24. gaf.com

GAF product pages are denied (403) or time out for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.gaf.com/en-us/roofing-materials/residential-roofing-materials/shingles/timberline-hdz", "request_id": "gaf-1"}'
```

### 27. hermanmiller.com

Herman Miller product pages deny every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.hermanmiller.com/products/seating/office-chairs/aeron-chair/", "request_id": "hermanmiller-1"}'
```

### 31. kichler.com

The price is only in the page's JSON-LD, which the LLM reads. Expect `price.current` `2299` (USD 22.99), the image and 6 attribute specs, with an `AI_EXTRACTED` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.kichler.com/2-fixture-mount-in-centennial-brass-15515CBR", "request_id": "kichler-1"}'
```

### 32. kohler.com

Kohler product pages are denied (403) or time out for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.kohler.com/en/products/kitchen-faucets/shop-kitchen-faucets/crue-pull-down-kitchen-sink-faucet-w-three-function-sprayhead-22972", "request_id": "kohler-1"}'
```

### 34. lithofin.com

Retailer-sold products: the technical data table and the image are read by the LLM. Expect 5 specs, the image, `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.lithofin.com/en-gb/products/natural-and-artificial-stone/lithofin-mn-easy-clean", "request_id": "lithofin-1"}'
```

### 36. lowes.com

Akamai blocks every page fetch, so the site is never contacted: the search model answers from the web, and the values come back unverified (`AI_UNVERIFIED`).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.lowes.com/pd/allen-roth-Nutmeg-Hickory-3-8-in-T-x-5-in-W-Handsculpted-Engineered-Hardwood-Flooring-32-29-sq-ft-Carton/50223071", "request_id": "lowes-1"}'
```

### 38. marble.com

The config sets `fetch.engine` `default` (`APP_DEFAULT_ENGINE`, `ai-crawler-avalai` on this instance) because the material facts (type, origin, colours) and applications are only readable by the LLM. Expect `source.engine` `ai-crawler-avalai`, 4 specs, 15 features, `price.state` `unknown` and the warnings `AI_EXTRACTED` and `PRICE_MISSING` (countertops are quoted per project).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://marble.com/granite-countertops/gran-perla/1179", "request_id": "marble-1"}'
```

### 41. moistureshield.com

Dealer-sold decking: the feature list and warranties are read by the LLM. Expect 7 features, 3 specs, the image, `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.moistureshield.com/products/composite-decking/vision/", "request_id": "moistureshield-1"}'
```

### 43. nanawall.com

NanaWall product pages are protected by bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.nanawall.com/glass-walls/folding/nw-aluminum-840", "request_id": "nanawall-1"}'
```

### 44. owenscorning.com

Owens Corning product pages return an access-denied page for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.owenscorning.com/en-us/roofing/shingles/oakridge", "request_id": "owenscorning-1"}'
```

### 45. pella.com

Pella product pages are protected by bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.pella.com/shop/windows/250-series/single-hung-windows/", "request_id": "pella-1"}'
```

### 47. qualitybath.com

Quality Bath product pages are protected by bot protection (403) for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.qualitybath.com/california-faucets-sh-083-showerhead-55-8-dia-product-246185.htm", "request_id": "qualitybath-1"}'
```

### 48. rehau.com

REHAU product pages return access denied (403) for every page fetch from this network; AvalAI's page fetch works but its answer cannot be checked against a saved page.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.rehau.com/us-en/mechanicalplumbing/search-by-product/raupex-pipe", "request_id": "rehau-1"}'
```

### 49. richelieu.com

Richelieu product pages time out for every page fetch from this network (the search route returns partial data: 2 specs).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.richelieu.com/us/en/category/kitchen-and-bathroom-accessories/kitchen/under-sink-cabinets/tip-out-trays/miscellaneous-tip-out-tray-hinges/1249728/sku-1162G", "request_id": "richelieu-1"}'
```

### 50. roomandboard.com

Room & Board product pages return access denied (403) for every page fetch from this network, and AvalAI's page fetch is denied too.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.roomandboard.com/catalog/living/sofas-and-loveseats/andre-sofas", "request_id": "roomandboard-1"}'
```

### 52. schumacher.com

Schumacher's storefront is protected by an AWS WAF JavaScript challenge; HTTP returns an empty challenge page and headless Chrome renders the app without the product (see the progress log).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://schumacher.com/catalog/products/5005854", "request_id": "schumacher-1"}'
```


### 57. steelcase.com

The www product page carries no JSON-LD price, so expect title, image and description with `price.state` `unknown`. The store host shows the price.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.steelcase.com/products/office-chairs/gesture/", "request_id": "steelcase-1"}'
```

### 58. stonelandusa.com

The visible page says `Get Quote`; the slab price is only in the page's JSON-LD, so the config sets `fetch.engine` `default` (`APP_DEFAULT_ENGINE`, `ai-crawler-avalai` on this instance). Expect `price.current` `182175` (USD 1,821.75) and an `AI_EXTRACTED` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://stonelandusa.com/product/grigio-carnico-sl-614/", "request_id": "stonelandusa-1"}'
```

### 59. techo-bloc.com

Techo-Bloc product pages return bot protection for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.techo-bloc.com/shop/walls/borealis", "request_id": "techo-bloc-1"}'
```


### 60. thermatru.com

Therma-Tru door style pages are access-denied (403) for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.thermatru.com/explore-products/door-style/S601", "request_id": "thermatru-1"}'
```

### 62. timbertech.com

Dealer-sold decking: the collection specs are read by the LLM. Expect the specs, the image, `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.timbertech.com/product/pro-legacy-collection/", "request_id": "timbertech-1"}'
```

### 63. tremcosealants.com

Tremco product pages time out for every page fetch from this network (HTTP, headless Chrome and AvalAI's page fetch).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.tremcosealants.com/products/tremsil-200", "request_id": "tremco-1"}'
```

### 64. trex.com

The sample kit price is only in the page's JSON-LD, which the LLM reads. Expect `price.current` `500` (USD 5.00), the SKU spec and 5 features, with an `AI_EXTRACTED` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.trex.com/products/decking/the-smart-kit/", "request_id": "trex-1"}'
```

### 65. turf.design

Designer-sold acoustic panels: the NRC and felt specification are read by the LLM. Expect 2 specs, the image, `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://turf.design/products/slice", "request_id": "turf-1"}'
```

### 66. usg.com

USG product pages are protected by bot protection for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.usg.com/en-US/p/product/sheetrock-brand-lay-in-gypsum-ceiling-panels-3260", "request_id": "usg-1"}'
```

### 68. visualcomfort.com

The price is in the page's JSON-LD and the rendered page; the LLM reads them after Chrome renders the page. Expect `price.current` `72165` (USD 721.65), 29 specs and the image.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.visualcomfort.com/us/p/charleston-medium-lantern-5291004", "request_id": "visualcomfort-1"}'
```

### 69. voltlighting.com

The price is in the page's JSON-LD, which the LLM reads. Expect `price.current` `3399` (USD 33.99), 17 specs, 5 features and the image.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.voltlighting.com/mr11-decorative-rock-light", "request_id": "volt-1"}'
```

### 70. waclighting.com

Trade-sold lighting: the SKU and dimension tables are read by the LLM. Expect 12 specs, the image and the call-for-price state.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://waclighting.com/product/axel/", "request_id": "wac-1"}'
```

### 72. wedi.net

Distributor-sold shower systems: the dimensions and order data are read by the LLM. Expect the specs, the image, `price.state` `unknown` and a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.wedi.net/en/products/flush-to-floor-showers/fundo-shower-element-with-point-drainage/fundo-plano", "request_id": "wedi-1"}'
```

### 73. westelm.com

West Elm product pages are access-denied (403) for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.westelm.com/products/yamazaki-sofa-back-console-table-w-shelf-d18054/", "request_id": "westelm-1"}'
```

### 74. wilsonart.com

Samples and quotes only (`$0.00` placeholder): expect title, description and the design repeat and finish as specs, with `price.state` `unknown` and warnings `AI_EXTRACTED` and `PRICE_MISSING`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.wilsonart.com/beigewood-7850", "request_id": "wilsonart-1"}'
```

### 75. woodworkerssource.com

Woodworkers Source product pages return bot protection for every page fetch from this network.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.woodworkerssource.com/walnut/walnut-flat-cut-paper-back-veneer-sheet-4-x-8-roll.html", "request_id": "woodworkerssource-1"}'
```

## A site without a config: the AI crawler (`ai-crawler-avalai`, `ai-crawler-cloudflare`)

No file in `configs/sites/` claims the host, so the built-in default rule serves it with `APP_DEFAULT_ENGINE`: the service fetches the page, cleans it and sends it to the LLM, `APP_AI_CRAWLER_MODEL` (`deepseek-v4.1-flash` on AvalAI) for `ai-crawler-avalai`, or `APP_AI_CRAWLER_CLOUDFLARE_MODEL` (`@cf/deepseek-ai/deepseek-v4.1-flash-1017` through Cloudflare AI Gateway) for `ai-crawler-cloudflare`. Name, price, image, description, specs and features come back, each checked against the page; a value that is not on the page is listed in `unavailable_fields` with reason `AI_NOT_GROUNDED`. Expect `site_id: ai-crawler-default`, `config_origin: default`, `source.engine` set to the default engine and the warning `AI_EXTRACTED`. Needs `APP_AI_CRAWLER_ENABLED=true` with that engine configured; with `APP_DEFAULT_ENGINE=none`, `SITE_NOT_CONFIGURED`. Provider problems fail with `AI_*` codes (`AI_RATE_LIMITED`, `AI_AUTH_FAILED`, ...) and `evidence.ai`.

### 76. wayfair.com

Wayfair runs bot protection, so from a data-center IP the fetch may fail with `BOT_PROTECTION` before the model is called. After three product pages of the same host match in shadow, the learned selectors serve it without LLM calls (`config_origin: learned`, warning `LEARNED_CONFIG`).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.wayfair.com/furniture/pdp/mercer41-55-w-8-drawer-dresser-w118377980.html?piid=2115584224", "request_id": "wayfair-ai-1"}'
```

Short summary:

```bash
... | jq '{status, site_id, config_origin, completeness, source, title: .product.title, price: .product.price, features: .product.features, unavailable: .unavailable_fields, error: .error.code}'
```

With `APP_DEFAULT_ENGINE=ai-crawler-cloudflare`, the same request is answered through Cloudflare AI Gateway. The `source` block then reads (illustrative values):

```json
{
  "config_version": "builtin-1+cloudflare",
  "source": {
    "engine": "ai-crawler-cloudflare",
    "site_contacted": true,
    "model": "@cf/deepseek-ai/deepseek-v4.1-flash-1017",
    "prompt_version": "1",
    "fetched_with": "http",
    "tokens": {"input": 9800, "output": 480}
  }
}
```

A failure through the gateway carries `evidence.ai.provider: "cloudflare"` and `evidence.ai.provider_request_id` set to `cf-aig-log-id`, the id to search for in the Cloudflare dashboard:

```bash
... | jq '{code: .error.code, action: .error.suggested_action, ai: .evidence.ai}'
```

## Run all 76 at once

```bash
#!/usr/bin/env bash
# Usage: API=http://127.0.0.1 KEY=gwp_... ./curl-all.sh
set -u
urls=(
  "https://www.1000bulbs.com/product/37/STAG-FEP.html"
  "https://www.3-form.com/products/edge-lit-chroma-countertop"
  "https://www.3m.com/3M/en_US/p/d/b00008068/"
  "https://abcworldwidestone.com/portfolio/gris-macael-dusted/"
  "https://www.acehardware.com/departments/paint-and-supplies/caulk-and-sealants/caulk/1912302"
  "https://annsacks.kohler.com/p/trim/savoy-0-5-x-8-box-liner-trim-as11682-trim-boxliner"
  "https://arktura.com/vapor-exterior/"
  "https://badomjip.com/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c-%d9%be%d8%b1%d9%88%d8%aa%d8%a6%db%8c%d9%86%db%8c-%d8%ac%db%8c%d9%be-%db%8c%da%a9-%da%a9%db%8c%d9%84%d9%88%da%af%d8%b1/"
  "https://www.bentleymills.com/product/4-agnona/"
  "https://www.bostik.com/global/en/bostik-academy/library/product-portfolio/decoration-paint/a535/"
  "https://www.bravarooftile.com/roof-tiles/barrel-tile/"
  "https://www.cosmossurfaces.com/washington/granite/absolute-black-premium-3cm"
  "https://www.daltile.com/products/concrete-look/portfolio/white"
  "https://www.dap.com/products/adhesives/weldwood-landau-top-and-trim-1145-high-heat-resistant-contact-cement"
  "https://www.deckorators.com/products/voyage-decking"
  "https://www.drytreat.com/sealers/residential/stain-proof-waterborne/"
  "https://www.emser.com/product/anvaya-f44anvapo1212m"
  "https://www.emtek.com/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/"
  "https://www.farrow-ball.com/us/wallpaper/five-over-stripe"
  "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155"
  "https://www.fleetwoodusa.com/products/3-series/series-3070/"
  "https://www.flooranddecor.com/ceramic-tile/zellige-oat-high-gloss-ceramic-tile-101130060.html"
  "https://www.flor.com/area-rugs_carpet-tiles/solids/made-you-look/21-1361.html"
  "https://www.gaf.com/en-us/roofing-materials/residential-roofing-materials/shingles/timberline-hdz"
  "https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/"
  "https://www.havwoods.com/us/products/hw3699/"
  "https://www.hermanmiller.com/products/seating/office-chairs/aeron-chair/"
  "https://hmland.ir/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c/"
  "https://hosseinibrothers.ir/%DA%A9%D8%B4%D9%85%D8%B4/%DA%A9%D8%B4%D9%85%D8%B4-%D9%BE%D9%84%D9%88%DB%8C%DB%8C-%D8%A2%D9%81%D8%AA%D8%A7%D8%A8%DB%8C"
  "https://www.innovationsusa.com/item/antwerp"
  "https://www.kichler.com/2-fixture-mount-in-centennial-brass-15515CBR"
  "https://www.kohler.com/en/products/kitchen-faucets/shop-kitchen-faucets/crue-pull-down-kitchen-sink-faucet-w-three-function-sprayhead-22972"
  "https://www.laminart.com/coal-706"
  "https://www.lithofin.com/en-gb/products/natural-and-artificial-stone/lithofin-mn-easy-clean"
  "https://www.llflooring.com/p/coreluxe-5mm-w-pad-daydream-meadow-oak-6-in-wide-x-48-in-long-10055236/"
  "https://www.lowes.com/pd/allen-roth-Nutmeg-Hickory-3-8-in-T-x-5-in-W-Handsculpted-Engineered-Hardwood-Flooring-32-29-sq-ft-Carton/50223071"
  "https://www.lumens.com/n701-round-corner-sofa-by-ethnicraft-ETH2111288.html"
  "https://marble.com/granite-countertops/gran-perla/1179"
  "https://www.marvin.com/products/windows/double-hung/elevate-double-hung-window"
  "https://miracle-sealants.com/miracle-sealants-porcelain-ceramic-tile-cleaner/"
  "https://www.moistureshield.com/products/composite-decking/vision/"
  "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/"
  "https://www.nanawall.com/glass-walls/folding/nw-aluminum-840"
  "https://www.owenscorning.com/en-us/roofing/shingles/oakridge"
  "https://www.pella.com/shop/windows/250-series/single-hung-windows/"
  "https://www.phillipjeffries.com/shop/CARD-464/46400"
  "https://www.qualitybath.com/california-faucets-sh-083-showerhead-55-8-dia-product-246185.htm"
  "https://www.rehau.com/us-en/mechanicalplumbing/search-by-product/raupex-pipe"
  "https://www.richelieu.com/us/en/category/kitchen-and-bathroom-accessories/kitchen/under-sink-cabinets/tip-out-trays/miscellaneous-tip-out-tray-hinges/1249728/sku-1162G"
  "https://www.roomandboard.com/catalog/living/sofas-and-loveseats/andre-sofas"
  "https://www.schluter.com/schluter-us/en_US/Membranes/Uncoupling-(DITRA)/Schluter%C2%AE-DITRA-PS/p/DITRA_PS"
  "https://schumacher.com/catalog/products/5005854"
  "https://shawfloors.com/en-us/laminate/oak-grove-hazelwood/sl119-02039"
  "https://www.sherwin-williams.com/homeowners/products/emerald-interior-acrylic-latex-paint?colorPartNumber=SW6617&colorType=PALETTE"
  "https://shop.interface.com/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043R.html"
  "https://www.soundseal.com/architectural-interiors/products/acoustical-wall-panels/s-4200-tuned-absorbers"
  "https://www.steelcase.com/products/office-chairs/gesture/"
  "https://stonelandusa.com/product/grigio-carnico-sl-614/"
  "https://www.techo-bloc.com/shop/walls/borealis"
  "https://www.thermatru.com/explore-products/door-style/S601"
  "https://www.tilebar.com/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html"
  "https://www.timbertech.com/product/pro-legacy-collection/"
  "https://www.tremcosealants.com/products/tremsil-200"
  "https://www.trex.com/products/decking/the-smart-kit/"
  "https://turf.design/products/slice"
  "https://www.usg.com/en-US/p/product/sheetrock-brand-lay-in-gypsum-ceiling-panels-3260"
  "https://www.veneersupplies.com/products/Flat-Cut-Rosewood-Bolivian-Veneer-Sheet--55-x-71.html"
  "https://www.visualcomfort.com/us/p/charleston-medium-lantern-5291004"
  "https://www.voltlighting.com/mr11-decorative-rock-light"
  "https://waclighting.com/product/axel/"
  "https://www.walkerzanger.com/product/18x48x1-1-2-adirondack-grey-coping-natural/"
  "https://www.wedi.net/en/products/flush-to-floor-showers/fundo-shower-element-with-point-drainage/fundo-plano"
  "https://www.westelm.com/products/yamazaki-sofa-back-console-table-w-shelf-d18054/"
  "https://www.wilsonart.com/beigewood-7850"
  "https://www.woodworkerssource.com/walnut/walnut-flat-cut-paper-back-veneer-sheet-4-x-8-roll.html"
  # No site config: served by the AI crawler's default rule (needs APP_AI_CRAWLER_ENABLED=true).
  "https://www.wayfair.com/furniture/pdp/mercer41-55-w-8-drawer-dresser-w118377980.html?piid=2115584224"
)
for u in "${urls[@]}"; do
  curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
    -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg url "$u" '{url: $url}')" |
    jq -c --arg url "$u" '{url: $url, status, engine: .source.engine, completeness, error: .error.code}'
done
```
