---
name: add-site
description: Add a new shop website to the product extractor. Use when the user gives a website URL and a sample product URL and asks to add, support, onboard, or configure that site. Covers detecting the platform and theme, writing configs/sites/<domain>.json, checking it live, saving a fixture, and writing an end-to-end test for the sample product.
---

# Add a new website

The user gives you two things:

1. **Site URL**, for example `https://hmland.ir/`
2. **Sample product URL**, for example `https://hmland.ir/product/%da%a9%d8%b1%d9%87-.../`

If they did not give a sample product URL, ask for one. A home page alone is not enough: every selector is checked against a real product page.

When you finish, these files exist and every test passes:

| File | What it is |
|---|---|
| `configs/sites/<domain>.json` | The site config, usually extending a template |
| `testdata/sites/<site_id>/<fixture>.html` + `.json` | The saved live product page and its expected output |
| `test/e2e/<site_id>_test.go` | End-to-end test that runs the real config against the saved page through the HTTP service |

Do not commit unless the user asks.

Names used below:

- `<domain>`: the host without `www.`, for example `hmland.ir`. Used for the config file name.
- `<site_id>`: lowercase, `^[a-z0-9][a-z0-9_-]{0,63}$`, usually the domain without the TLD, for example `hmland`. Used for the fixture folder, the test file, and the `site_id` field.
- `<fixture>`: a short kebab-case name that describes the sample page, for example `peanut-butter-variable` or `raisin-available`.
- `$SCRATCH`: your scratchpad directory. Keep downloaded HTML there, not in the repo, until `sitectl fixture` saves the real fixture.

## Step 1: Read what already exists

Read these before writing anything. They are short.

- `configs/schema/site-config.v1.json`: every allowed field. `additionalProperties` is false everywhere, so an unknown key fails validation.
- `configs/templates/*.json`: `woocommerce.json`, `woodmart.json` (extends woocommerce), `prestashop.json`.
- `configs/sites/*.json`: existing sites. `badomjip.com.json` is a minimal WoodMart site. `hosseinibrothers.ir.json` is a PrestaShop site with a custom theme that overrides most selectors.

Merge rules: a site file deep-merges over its template. Objects merge key by key. **Arrays replace** the template's arrays. So if you set `call_for_price_words`, list every word you want, not only the new ones.

## Step 2: Download the sample page and identify the platform

```bash
D=$SCRATCH/<site_id>; mkdir -p $D
curl -sSL -A "Mozilla/5.0" -o $D/page.html \
  -w "%{http_code} %{size_download} %{url_effective}\n" "<product url>"
curl -sS -A "Mozilla/5.0" "<site url>robots.txt" | head -40
grep -o '<body[^>]*>' $D/page.html
grep -oE '(wp-content/themes|themes)/[a-zA-Z0-9_-]+' $D/page.html | sort | uniq -c
```

Check:

- **HTTP status is 200** and the size is reasonable. A 403, a Cloudflare challenge ("Just a moment..."), or a tiny page with only a `<div id="root">` means the service will not be able to fetch it either. Tell the user and stop.
- **robots.txt** does not disallow the product path. The service respects robots.txt by default.
- **Platform and theme**, from the body classes and theme paths:

| Signal | Platform | Template |
|---|---|---|
| `woocommerce`, `single-product`, `wp-theme-woodmart` / `themes/woodmart` | WooCommerce + WoodMart | `woodmart` |
| `woocommerce`, `single-product`, another WordPress theme | WooCommerce | `woocommerce` (override selectors that differ) |
| `body#product`, `prestashop` in scripts, `/modules/` and `/themes/<name>/` paths | PrestaShop | `prestashop` (custom themes need most selectors overridden) |
| Anything else | Unknown | No `extends`; write the whole `pages.product` block |

## Step 3: Check every selector against the page

For the chosen template, count each selector's key class in the page. Example for WooCommerce/WoodMart:

```bash
for p in 'class="product_title' 'type-product' '<p class="price' 'woocommerce-product-gallery' \
         'short-description' 'Tabs-panel--description' 'single_add_to_cart_button' \
         'stock out-of-stock' 'canonical'; do
  echo "== $p: $(grep -c -- "$p" $D/page.html)"; done
grep -oE '<h1[^>]*>[^<]*' $D/page.html
grep -oE '<p class="price">.{0,600}' $D/page.html | head -3
grep -o '<link rel="canonical"[^>]*>' $D/page.html
```

For each field decide: does the template selector match the right element? If not, find the correct selector in the page and override it in the site config.

| Field / setting | What to check |
|---|---|
| `pages.product.match` | Go regexes on the **URL path**. The sample product path must match; category, cart, and info paths must not. WooCommerce: `^/product/`. PrestaShop default: `.html` paths; custom URL schemes need an override (see hosseinibrothers). |
| `root` | Must match exactly the element that contains the product (title, price, button). Every other selector is searched inside it. |
| `fields.title` | One `h1` with the product name. |
| `fields.image_url` | The main gallery image. Use `attr` with the full-size attribute first (`data-large_image`, `data-src`, `data-image-large-src`) and `src` last. Lazy-load placeholders (`data:` URIs) are skipped automatically. |
| `fields.price_current` / `price_regular` | Current price and the crossed-out regular price. For ranges (variable products) the **lowest** number is taken. |
| `price_area` | Its text becomes `price.raw` and is searched for `call_for_price_words`. Keep it tight (the price block only), or a word elsewhere on the page will mark the product as call for price. |
| `fields.description` | Usually `pick: "join"` with `separator: "\n"` over paragraphs. |
| `availability.out_of_stock` | Selector that exists **only** when the product is out of stock. |
| `availability.order_button` | Selector that exists **only** when the product can be ordered. If configured and missing, the product is out of stock. |

### Price unit: rial or toman

Look at the currency text next to the price (`ریال` or `تومان`, or the `woocommerce-Price-currencySymbol` span).

- `تومان` → `"price_unit": "toman"` (the default)
- `ریال` → `"price_unit": "rial"`. Prices are divided by 10, and the output is always toman (`currency: "IRT"`).

Set `currency_words` to the words that appear on the site. Persian and Arabic digits and separators (`۱۲۳٬۴۵۶`) are handled by `internal/normalize`.

### Call for price words

Search the page for phrases like `تماس بگیرید` and `استعلام قیمت`. Include the ones the site uses in `locale.call_for_price_words`. Make sure the word does not appear inside `price_area` on a normal priced product.

## Step 4: Write the config

File: `configs/sites/<domain>.json`. Keep only what differs from the template. Always set `$comment` with the platform, theme, price unit, and the date you checked the live page.

Minimal WoodMart example (this is the real hmland.ir config):

```json
{
  "$comment": "WoodMart theme on WooCommerce. Prices are shown in rial (ریال). Checked against a live variable product page on 2026-11-11.",
  "schema_version": 1,
  "site_id": "hmland",
  "version": "2026-11-11.1",
  "enabled": true,
  "extends": "woodmart",
  "domains": ["hmland.ir", "www.hmland.ir"],

  "locale": {
    "price_unit": "rial",
    "currency_words": ["ریال", "تومان"],
    "call_for_price_words": ["تماس بگیرید", "استعلام قیمت"]
  }
}
```

Custom-theme example (overrides every selector): see `configs/sites/hosseinibrothers.ir.json`.

Rules:

- `version` is `YYYY-MM-DD.N` with today's date. Bump `N` on every later change to the same site; it is returned as `config_version`.
- `domains` lists the bare domain and `www.`, lowercase, no scheme or port. A canonical link on these domains becomes `product.url`.
- Do not change `fetch` unless the site needs it (slow site → higher `timeout_seconds`; strict rate limit → higher `delay_ms`).
- `validation.required_fields` defaults to `["title"]`. Add more only if the user asks.
- Never edit a template to fix one site. Override in the site file. Change a template only if the fix is correct for every site that extends it.

## Step 5: Validate and check live

```bash
go run ./cmd/sitectl validate
go run ./cmd/sitectl check --url "<product url>"
```

`validate` must print `0 invalid`. `check` must print `SUCCESS` and the product. Read the output field by field and compare it with the page in a browser or with the HTML:

- `title` is the product name, not the site name.
- `image_url` is the full-size product photo, an absolute URL.
- `price.current` is the right number **in toman** (divided by 10 for rial sites). `price.regular` is set only when there is a crossed-out price.
- `price.state` is `available` for an orderable product.
- `description` is product text, not menu or footer text.
- `warnings` is empty, or every warning is expected (for example no description on that product).

If `check` fails, it prints an error code, root cause, and the selector trace. Fix the config and run it again. Typical codes: `SELECTOR_NOT_FOUND` (wrong `root` or field selector), `NOT_PRODUCT_URL` (`match` regex), `PRICE_PARSE_FAILED` (price selector hits text without numbers; fix the selector or add a call-for-price word).

If the description is very long because it joins article text, mention it to the user and offer to narrow the selector. Do not narrow it without asking.

## Step 6: Save the fixture

```bash
go run ./cmd/sitectl fixture --url "<product url>" --name <fixture>
go run ./cmd/sitectl test
```

This writes `testdata/sites/<site_id>/<fixture>.html` (the real page) and `<fixture>.json` (the expected output). Open the `.json` and confirm the expected product is correct, because it becomes the truth the tests check against. `sitectl test` must show every fixture `PASS` and `0 failed`. Every enabled site needs at least one fixture. Add `--force` only to replace a fixture on purpose.

## Step 7: Write the end-to-end test for the sample product

File: `test/e2e/<site_id>_test.go`. It must:

1. Load the **real** `configs/sites/<domain>.json`, not a hand-written copy, so the test breaks when the config breaks.
2. Serve the **saved fixture HTML** from the fake shop, so it runs without the internet.
3. Call the real HTTP API (`POST /api/v1/extract`) and assert on the response.

### Harness facts you need

These live in `test/e2e/harness_test.go` and `test/e2e/shop_test.go`. Read them before writing the test.

- `Start(t, Options{...})` boots the whole service and a fake shop. Every response is also checked against the Swagger doc.
- `Options.ExtraSites map[string]map[string]any` adds site configs (key = file name without `.json`).
- `Options.Fetch func(*fetch.Options)` changes fetch settings. The suite limits bodies to 256 KB; real pages are often bigger, so raise `MaxBodyBytes` (for example `2 << 20`).
- `e.Shop.Handle(path, html)` serves `html` with status 200 at `path`. The path is **decoded** (`r.URL.Path`), so a Persian slug is registered in Persian. Unregistered paths return 404.
- Every host ending in `.test` resolves to the fake shop. Use `<site_id>.test` as the fake host.
- `e.Shop.URL(host, path)` builds the request URL. Escape the path with `(&url.URL{Path: path}).EscapedPath()`.
- `writeConfigs` copies every template from `configs/templates/`, so `extends` works.
- `repoRoot` (`../..`, the repository root) is declared in `harness_test.go`. Use it for paths to `configs/` and `testdata/`; do not declare it again.
- `test/e2e/hmland_test.go` defines `withPrice` (replace the WooCommerce `<p class="price">` block), `withoutButton` (remove `single_add_to_cart_button`), `rialAmount`, and `priceBlock`. They are in the same package, so reuse them for WooCommerce sites and do not redefine them. For other platforms write helpers with site-specific names (for example `<siteID>WithPrice`), because every test file shares one namespace.
- Result helpers: `r.Success(t)`, `r.Failed(t, code, stage, owner, retryable)`, `r.Str("product.title")`, `r.Int("product.price.current")`, `r.At(path)`, `r.Has(path)`.

### Loading the config for the test

Add the fake host to the real domains, and keep the real domains so the page's canonical link (`https://<domain>/...`) is honoured exactly as in production. Make fetch fast:

```go
func <siteID>Site(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "configs", "sites", "<domain>.json"))
	must(t, err)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	domains, _ := cfg["domains"].([]any)
	cfg["domains"] = append(domains, host<SiteID>)
	cfg["fetch"] = map[string]any{"delay_ms": 0, "timeout_seconds": 5, "retries": 2, "respect_robots_txt": true}
	return cfg
}
```

Load the fixture HTML and the `expect.product` object from `testdata/sites/<site_id>/<fixture>.json` (see `hmlandFixture` in `hmland_test.go`).

### Cases to write

Write one `Test<SiteID>` with these subtests. Use the sample product's real path (decoded) for the first ones.

| Subtest | How | Assert |
|---|---|---|
| Saved live page matches the fixture | Serve the fixture HTML at the sample path | `Success`; `site_id`; `title`, `url`, `image_url`, `description`, `price.currency`, `price.raw`, `price.state` equal the fixture; no warnings |
| Sample product details | Same page | Exact title, `price.current` in toman, `price.regular` (number or present-and-null), `state` |
| Canonical URL | Same page | `product.url` starts with `https://<domain>/` (only if the page has a canonical link) |
| Sale price | Replace the price block with `<del>` regular + `<ins>` current, in the site's markup and unit | Both prices, converted to toman |
| Out of stock | Remove the order button, add the out-of-stock element | `state = out_of_stock`, price kept |
| Call for price | Replace the price block with one of the site's call-for-price words, remove the button | `state = call_for_price`, `current` present and `null` |
| Layout change | Rename the root's class in the HTML | `Failed(t, "SELECTOR_NOT_FOUND", "extract", "site_config", false)` |
| Non-product URL | Request a category path that `match` rejects | `Failed(t, "NOT_PRODUCT_URL", "config", "client", false)` and `s.Hits(path) == 0` |
| Deleted product | Request an unregistered product path | `error.code = HTTP_NOT_FOUND` |

Build the variants by editing the **real** fixture HTML with `strings.Replace` or a regex, not by writing new HTML. That keeps the site's real markup in every case. If a replacement target is missing, call `t.Fatal`, so the test does not silently test nothing.

Keep failure messages short: print the fields you checked, not `r.Raw`. Real pages have long descriptions that flood the output.

The complete working example is `test/e2e/hmland_test.go`. Copy its structure and change the names, paths, prices, and markup for the new site.

### Prove the test can fail

After it passes, break the config on purpose and run the test again. For example change `price_unit`, or change the `root` selector. At least one subtest must fail. Then restore the file exactly (copy it to `$SCRATCH` first) and run again.

```bash
cp configs/sites/<domain>.json $SCRATCH/backup.json
sed -i 's/"price_unit": "rial"/"price_unit": "toman"/' configs/sites/<domain>.json
go test ./test/e2e/ -run Test<SiteID>        # must FAIL
cp $SCRATCH/backup.json configs/sites/<domain>.json
go test ./test/e2e/ -run Test<SiteID>        # must PASS
```

## Step 8: Run everything

```bash
gofmt -l test/ internal/          # must print nothing
go vet ./test/e2e/
go run ./cmd/sitectl validate
go run ./cmd/sitectl test
go test -race ./...
```

All must pass. Unit tests in `internal/sitectl` read the real `configs/` folder. If one of them breaks only because a site was added (for example it counts sites), fix the test to not depend on the number of sites. Do not delete the test or skip it.

## Step 9: Report to the user

Tell the user:

- The platform and theme you found, the template used, and the price unit.
- The live result for the sample product: title, price in toman (and the original number and unit on the page), state.
- The files you added or changed, as links.
- The test results, including the failure check from Step 7.
- Anything to decide: a very long description, a selector you were unsure about, or fixtures worth adding (out-of-stock, sale, or call-for-price pages from the real site).
- That nothing is committed.

## Checklist

- [ ] Sample page fetched with HTTP 200, not blocked by robots.txt or a challenge
- [ ] Platform and template identified
- [ ] Every selector checked against the real page
- [ ] Price unit and call-for-price words set from the page
- [ ] `configs/sites/<domain>.json` written with `$comment` and a dated `version`
- [ ] `sitectl validate` → 0 invalid
- [ ] `sitectl check` → SUCCESS, every field correct
- [ ] Fixture saved and its `.json` reviewed; `sitectl test` → 0 failed
- [ ] `test/e2e/<site_id>_test.go` written for the sample product, using the real config and fixture
- [ ] Test shown to fail when the config is broken, then restored
- [ ] `gofmt`, `go vet`, `go test -race ./...` all pass
- [ ] Report sent; nothing committed
