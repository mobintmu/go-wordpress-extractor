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

The site itself is not contacted (`source.site_contacted` is `false`). Only title, URL and description come from the search result. Image, price and specs are listed in `unavailable_fields` with reason `NOT_IN_SEARCH_RESULTS`, and `completeness` is `partial`. At least one of `APP_DUCKDUCKGO_ENABLED` or `APP_BRAVE_ENABLED` must be on, otherwise the call fails with `ENGINE_UNAVAILABLE`.

### 4. fergusonhome.com

On 2026-11-13 DuckDuckGo had this product at rank 1. A second identical call within 6 hours is served from the cache (`source.cached: true`).

```bash
curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"url": "https://www.fergusonhome.com/duravit-030549-0hole/s1666394?uid=3938155", "request_id": "ferguson-1"}'
```

Short summary:

```bash
... | jq '{status, completeness, title: .product.title, source, unavailable: [.unavailable_fields[].field]}'
```

## Run all 13 at once

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
)
for u in "${urls[@]}"; do
  curl --max-time 190 -sS -X POST "$API/api/v1/extract" \
    -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg url "$u" '{url: $url}')" |
    jq -c --arg url "$u" '{url: $url, status, engine: .source.engine, completeness, error: .error.code}'
done
```
