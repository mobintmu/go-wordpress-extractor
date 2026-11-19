# curl examples, one per website

One `POST /api/v1/extract` call for each of the 13 supported sites (see [websites.md](websites.md)). The URLs are the sample products from `testdata/sites/`. Some fixtures are hand-made, so if a call returns `HTTP_NOT_FOUND`, swap in a current product URL from that site.

## Setup

```bash
export API=http://127.0.0.1:4000          # APP_HTTP_ADDRESS / APP_HTTP_PORT
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

### 1. 3-form.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.3-form.com/products/edge-lit-chroma-countertop", "request_id": "3form-1"}'
```

### 2. badomjip.com

Prices are in toman (`IRT`). The site has call-for-price, out-of-stock and variable products.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://badomjip.com/product/almondbutter400/", "request_id": "badomjip-1"}'
```

### 3. emtek.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.emtek.com/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/", "request_id": "emtek-1"}'
```

### 7. hmland.ir

The site shows rial. The response converts to toman (`currency: IRT`). Persian slugs must be sent percent-encoded, as below.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://hmland.ir/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c/", "request_id": "hmland-1"}'
```

### 8. hosseinibrothers.ir

Prices are in toman (`IRT`). Percent-encoded Persian path.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://hosseinibrothers.ir/%DA%A9%D8%B4%D9%85%D8%B4/%DA%A9%D8%B4%D9%85%D8%B4-%D9%BE%D9%84%D9%88%DB%8C%DB%8C-%D8%A2%D9%81%D8%AA%D8%A7%D8%A8%DB%8C", "request_id": "hosseinibrothers-1"}'
```

### 10. msisurfaces.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/", "request_id": "msisurfaces-1"}'
```

### 11. sherwin-williams.com

The price is loaded by JavaScript, so the `http` engine gets no price. Expect `PRICE_MISSING` and `price` in `unavailable_fields` with reason `NOT_ON_PAGE`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.sherwin-williams.com/homeowners/products/emerald-interior-acrylic-latex-paint?colorPartNumber=SW6617&colorType=PALETTE", "request_id": "sherwin-williams-1"}'
```

### 12. shop.interface.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://shop.interface.com/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043R.html", "request_id": "interface-1"}'
```

### 13. tilebar.com

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.tilebar.com/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html", "request_id": "tilebar-1"}'
```

## Sites using the `browser` engine

These load the product page in real Chrome through Playwright, so the page's JavaScript runs and Cloudflare challenges that only check for a JavaScript-capable browser are passed. `source.engine` is `browser` and `source.site_contacted` is `true`. The server needs `APP_BROWSER_ENABLED=true`, otherwise the call fails with `ENGINE_UNAVAILABLE`. A browser fetch takes several seconds, so keep `--max-time` high.

### 5. flooranddecor.com

Cloudflare blocked the plain `http` engine with "Sorry, you have been blocked" (HTTP 403). If the browser gets the same page, the block is on the server's IP and the call fails with `BOT_PROTECTION`; then switch the site to a search engine or route it through a different IP (`APP_PROXY_*`).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.flooranddecor.com/ceramic-tile/zellige-oat-high-gloss-ceramic-tile-101130060.html", "request_id": "flooranddecor-1"}'
```

### 6. hafele.com.de

Cloudflare answers a plain HTTP client with a "Just a moment..." JavaScript challenge, which the browser runs. Prices are hidden from anonymous visitors, so expect `price.state: unknown` with a `PRICE_MISSING` warning.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/", "request_id": "hafele-1"}'
```

### 9. lumens.com

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

fergusonhome.com was served this way until 2026-11-19; it now uses `ai-crawler-avalai` (example 4 below). `configs/sites/lumens.com.json` sets `duckduckgo` (alternate `brave`) too.

## A site using an AI engine in its config (`ai-crawler-avalai`)

The site config picks the engine, so the request is the same as for any site. With `ai_fetch` `http` or `browser` the page is fetched, cleaned and read by the LLM, and every value is checked against the page. With `ai_fetch: web_search` the site is never contacted: only the URL goes to the search model (`APP_AI_CRAWLER_SEARCH_MODEL`, `sonar`), and the values come back unverified. `source` adds `model`, `prompt_version`, `fetched_with`, `tokens` and, for web search, `sources`. Needs `APP_AI_CRAWLER_ENABLED=true` with an AvalAI key; otherwise `ENGINE_UNAVAILABLE`.

### 4. fergusonhome.com

Ferguson is behind Akamai and answers 403 to plain HTTP and to Chrome, so the config uses `ai_fetch: web_search` and the site is never contacted. Expect `site_id: fergusonhome`, `source.engine: ai-crawler-avalai`, `source.fetched_with: web_search`, `source.site_contacted: false`, `source.sources` (a fergusonhome.com page) and the warnings `AI_EXTRACTED` and `AI_UNVERIFIED`. On 2026-11-19 it returned the title, price 15666 (USD cents, `$156.66`), the description and the specs; the image was not in the answer (`NOT_IN_SEARCH_RESULTS`). The values are not checked against the page, so price and stock may be out of date. With no source on fergusonhome.com it fails with `SEARCH_NO_RESULT`.

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155", "request_id": "ferguson-1"}'
```

Short summary:

```bash
... | jq '{status, site_id, completeness, source, title: .product.title, price: .product.price, specs: (.product.specs | length), warnings: [.warnings[].code], unavailable: .unavailable_fields, error: .error.code}'
```

## A site without a config: the AI crawler (`ai-crawler-avalai`, `ai-crawler-cloudflare`)

No file in `configs/sites/` claims the host, so the built-in default rule serves it with `APP_DEFAULT_ENGINE`: the service fetches the page, cleans it and sends it to the LLM, `APP_AI_CRAWLER_MODEL` (`deepseek-v4.1-flash` on AvalAI) for `ai-crawler-avalai`, or `APP_AI_CRAWLER_CLOUDFLARE_MODEL` (`@cf/deepseek-ai/deepseek-v4.1-flash-1017` through Cloudflare AI Gateway) for `ai-crawler-cloudflare`. Name, price, image, description, specs and features come back, each checked against the page; a value that is not on the page is listed in `unavailable_fields` with reason `AI_NOT_GROUNDED`. Expect `site_id: ai-crawler-default`, `config_origin: default`, `source.engine` set to the default engine and the warning `AI_EXTRACTED`. Needs `APP_AI_CRAWLER_ENABLED=true` with that engine configured; with `APP_DEFAULT_ENGINE=none`, `SITE_NOT_CONFIGURED`. Provider problems fail with `AI_*` codes (`AI_RATE_LIMITED`, `AI_AUTH_FAILED`, ...) and `evidence.ai`.

### 14. wayfair.com

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

## Run all 14 at once

```bash
#!/usr/bin/env bash
# Usage: API=http://127.0.0.1:4000 KEY=gwp_... ./curl-all.sh
set -u
urls=(
  "https://www.3-form.com/products/edge-lit-chroma-countertop"
  "https://badomjip.com/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c-%d9%be%d8%b1%d9%88%d8%aa%d8%a6%db%8c%d9%86%db%8c-%d8%ac%db%8c%d9%be-%db%8c%da%a9-%da%a9%db%8c%d9%84%d9%88%da%af%d8%b1/"
  "https://www.emtek.com/all-products/door-hardware/front-door-entry-sets/tubular-entry-sets/select-smooth-grip-full-length-tubular-entry-set/"
  "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155"
  "https://www.flooranddecor.com/ceramic-tile/zellige-oat-high-gloss-ceramic-tile-101130060.html"
  "https://www.hafele.com.de/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/"
  "https://hmland.ir/product/%da%a9%d8%b1%d9%87-%d8%a8%d8%a7%d8%af%d8%a7%d9%85-%d8%b2%d9%85%db%8c%d9%86%db%8c/"
  "https://hosseinibrothers.ir/%DA%A9%D8%B4%D9%85%D8%B4/%DA%A9%D8%B4%D9%85%D8%B4-%D9%BE%D9%84%D9%88%DB%8C%DB%8C-%D8%A2%D9%81%D8%AA%D8%A7%D8%A8%DB%8C"
  "https://www.lumens.com/n701-round-corner-sofa-by-ethnicraft-ETH2111288.html"
  "https://www.msisurfaces.com/luxury-vinyl-planks/ashton-2/briven/"
  "https://www.sherwin-williams.com/homeowners/products/emerald-interior-acrylic-latex-paint?colorPartNumber=SW6617&colorType=PALETTE"
  "https://shop.interface.com/US/en-US/resilient/lvt/cut-and-form-4.5-mm/A043R.html"
  "https://www.tilebar.com/product/avanti-checkerboard-verde-green-and-crema-marfil-beige-12x12-tumbled-marble-tile.html"
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
