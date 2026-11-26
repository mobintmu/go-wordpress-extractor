# go-wordpress-extraction: product extraction service

A stateless HTTP service that extracts one product from one product page. A client application sends a product URL with an API key. The service finds that site's parsing rules in a JSON file, fetches the page, and returns either the product or a structured root cause with evidence, in the same response.

For sites behind bot protection that the service does not get past (lumens.com), a search engine (`duckduckgo`, `brave`, `mojeek`, `yahoo` or `yandex`) builds the product from the search result instead, without contacting the site. Whatever is found is returned, and every field that could not be found is listed in `unavailable_fields`. See [Search engines](#search-engines-duckduckgo-brave-mojeek-yahoo-yandex), [md/ENGINES.md](md/ENGINES.md) and [Responsible use](#responsible-use).

For a shop that has no config at all, the AI crawler (off by default) is the default rule: it fetches the page and has an LLM read name, price, image, description, specs and features, each checked against the page. Two AI engines exist: `ai-crawler-avalai` (AvalAI, `deepseek-v4.1-flash`) and `ai-crawler-cloudflare` (Cloudflare AI Gateway, `@cf/deepseek-ai/deepseek-v4.1-flash-1017`); `APP_DEFAULT_ENGINE` picks the one for shops without a config. After a few products it learns CSS selectors for the shop, so later requests need no LLM call. See [AI crawler](#ai-crawler-ai-crawler-avalai-ai-crawler-cloudflare), [md/AI_CRAWLER_AVALAI_PROPOSAL.md](md/AI_CRAWLER_AVALAI_PROPOSAL.md) and [md/AI_CRAWLER_CLOUDFLARE_PROPOSAL.md](md/AI_CRAWLER_CLOUDFLARE_PROPOSAL.md).

There is no database and no Redis. Site configs and API keys are files that are re-read every 30 seconds. The design is in [md/SYNC_EXTRACTION_PROPOSAL.md](md/SYNC_EXTRACTION_PROPOSAL.md).

Because nodes share nothing, you can run as many as you like. Each node reports how busy it is, so the client can act as the dispatcher and spread requests across nodes. See [Run distributed](#run-distributed-client-side-load-balancing).

```text
POST /api/v1/extract   X-API-Key        {"url": "...", "request_id": "..."}
GET  /api/v1/sites     X-API-Key        enabled sites available for extraction
GET  /api/v1/active    X-API-Key        {"extract_active_requests": N}, extractions running now
GET  /metrics          X-API-Key        instance load + counters as JSON; Prometheus text for Accept: text/plain
GET  /health, /ready   public
GET  /swagger/index.html   operator credentials (HTTP Basic); off unless configured
```

## Documentation

| File | What it covers |
| --- | --- |
| [SERVER_SETUP.md](md/SERVER_SETUP.md) | Deploy with `deploy.sh`, run locally with `menu.sh`, and check service health and logs. |
| [BUILD_SMALL_SERVERS.md](md/BUILD_SMALL_SERVERS.md) | Build on a low-memory VPS using swap, build settings, or binaries compiled on another machine. |
| [SETUP_KEYS.md](md/SETUP_KEYS.md) | Create the server pepper, client API keys, operator credentials, and TLS certificates. |
| [CURL_EXAMPLES.md](md/CURL_EXAMPLES.md) | Try example extraction requests for individual websites or run them in bulk. |
| [websites.md](md/websites.md) | Look up supported websites, their platforms, currencies, and fetch engines. |
| [add-site-engines.md](md/add-site-engines.md) | See which engine each site config selects and why it uses that engine. |
| [ENGINES.md](md/ENGINES.md) | Compare fetch engines and see how per-site rules and default engine selection work. |
| [browser-engine.md](md/browser-engine.md) | Install, configure, and troubleshoot the Playwright browser engine and Xvfb. |
| [OUTBOUND_PROXY.md](md/OUTBOUND_PROXY.md) | Understand outbound HTTP proxy settings, DNS resolution, and SSRF checks. |
| [SYNC_EXTRACTION_PROPOSAL.md](md/SYNC_EXTRACTION_PROPOSAL.md) | Read the design for stateless HTTP extraction, file-based configuration, and error reporting. |
| <img src="md/SYNC_EXTRACTION_BIG_PICTURE.png" alt="Synchronous extraction service diagram" width="440"> | Overview of the synchronous extraction service. |
| <img src="md/fetch-engine-strategy.svg" alt="Fetch engine selection diagram" width="440"> | How the fetch router selects an engine from each site config. |
| [real-chrome-playwright.md](md/real-chrome-playwright.md) | Read the original proposal for rendering JavaScript pages with Playwright and Chrome. |
| [SEARCH_ENGINE_PROPOSAL.md](md/SEARCH_ENGINE_PROPOSAL.md) | Read the design for DuckDuckGo and Brave search engines and per-site engine selection. |
| [METAWEBSEARCH_ENGINE_PROPOSAL.md](md/METAWEBSEARCH_ENGINE_PROPOSAL.md) | Read the design for Mojeek, Yahoo, and Yandex search engines using `metawebsearch`. |
| [AI_CRAWLER_AVALAI_PROPOSAL.md](md/AI_CRAWLER_AVALAI_PROPOSAL.md) | Read the original design for AvalAI-powered extraction and selector learning. |
| [AI_CRAWLER_CLOUDFLARE_PROPOSAL.md](md/AI_CRAWLER_CLOUDFLARE_PROPOSAL.md) | Read the design for Cloudflare AI Gateway and configurable default AI engines. |
| [IMPLEMENTATION.md](md/IMPLEMENTATION.md) | Review a historical implementation snapshot covering the original crawler, APIs, and storage. |

## Layout

```text
docs/                   Swagger 2.0 doc generated by swag from the handler annotations (do not edit)
cmd/server              HTTP service
cmd/apikey              Key generation (single and bulk), listing, revocation
cmd/sitectl             Site config validation, live checks, fixtures
configs/                schema/, templates/ (woocommerce, woodmart, prestashop), sites/
internal/apikey         Key format, HMAC hashing, .env and JSON key files, reload, middleware
internal/siteconfig     Loader, template merge, validation, registry with hot reload
internal/extract        Pipeline (config, fetch, page_check, extract, normalize, validate) and handler
internal/fetch          Fetch engines (http/Colly, browser, search, AI) and router; SSRF-safe dialer, retries, redirects, size limit
internal/fetch/search   DuckDuckGo, Brave, Mojeek, Yahoo and Yandex providers, URL matching, query building
internal/aicrawler      LLM client (openai-go) with provider profiles (AvalAI, Cloudflare AI Gateway), page cleaner, prompts, cache
internal/sites          GET /api/v1/sites (site list, default rule, default AI engine)
internal/normalize      Digits, prices (ranges, rial to toman), text, URLs
internal/diagnose       Failure codes, page checks, selector trace, HTML samples
internal/limiter        Per-instance and per-site concurrency, per-site request spacing
internal/metrics        Prometheus text metrics
testdata/sites/         Saved HTML and expected output per site
testdata/search/        Saved DuckDuckGo result pages (real); synthetic Brave, Mojeek, Yahoo and Yandex responses
test/e2e/               End-to-end tests against the real server, a fake shop, a fake LLM and a fake Cloudflare AI Gateway
```

## Deploy on a Linux server

Put a fresh checkout in `/opt/go-wordpress-extractor`. The systemd service runs as `gwp` and needs access to the checkout. From that directory, run the interactive installer.

```bash
cd /opt/go-wordpress-extractor
sudo ./deploy.sh install
sudo ./deploy.sh status
```

The [server setup guide](md/SERVER_SETUP.md) walks through the individual deployment commands and the local `./menu.sh` workflow. For `compile: signal: killed` on a small VPS, see the [small-server build guide](md/BUILD_SMALL_SERVERS.md) for swap, build settings, and building on another machine.

### Recover from `200/CHDIR` under `/root`

If `gwp` cannot enter `/root/go-wordpress-extractor`, run this from the existing checkout with an up-to-date `deploy.sh`.

```bash
sudo ./deploy.sh relocate
```

`relocate` stops the service, moves the checkout to `/opt/go-wordpress-extractor`, updates absolute paths in `.env.production`, and reinstalls and starts the systemd service. It preserves the binaries, API keys, and pepper. If the destination already exists, it stops before moving anything.

Once it prints `service is up`, use the new path to check status. If setup was interrupted after the move, rerun `sudo ./deploy.sh relocate` from `/opt/go-wordpress-extractor`.

```bash
cd /opt/go-wordpress-extractor
sudo ./deploy.sh status
```

## Run locally

```bash
# 1. Create a development key with the pepper from .env.development
APP_API_KEY_PEPPER=$(grep '^APP_API_KEY_PEPPER=' .env.development | cut -d= -f2-) \
APP_API_KEYS_FILE=secrets/api_keys.json \
  go run ./cmd/apikey generate --name local-dev   # prints the key once

# 2. Start the service
go run ./cmd/server


# 2.2 
sudo apt-get install -y xvfb
xvfb-run -a go run ./cmd/server


# 3. Call it
curl --max-time 190 -X POST http://127.0.0.1:4000/api/v1/extract \
  -H "X-API-Key: gwp_..." -H 'Content-Type: application/json' \
  -d '{"url": "https://badomjip.com/product/example/"}'
```

Both extraction outcomes return HTTP 200. Always read the `status` field. Only `HTTP_NOT_FOUND` means the product is gone.

## API keys

Step-by-step setup of every key from scratch (pepper, API keys, operator login, TLS): [md/SETUP_KEYS.md](md/SETUP_KEYS.md).

Keys look like `gwp_<id>_<secret>`. Only an HMAC-SHA-256 of each key, keyed with `APP_API_KEY_PEPPER`, is stored. Files ending in `.json` are read as JSON; anything else is read as `.env` (production default: `secrets/.env.apikeys`).

```bash
apikey generate-pepper                                   # once; set it on every instance
apikey generate --name catalog-service --expires-in 8760h
apikey generate-bulk --count 100 --name-prefix app --plain-output /secure/app-keys.csv
apikey generate-bulk --names-file clients.txt --plain-output /secure/client-keys.csv
apikey list
apikey revoke --id k7f3a9c2
apikey revoke --name-prefix app
echo 'operator password' | apikey hash-ops-password     # value for APP_OPS_PASSWORD_HASH
```

- With `--plain-output`, full keys go only to that CSV (permission 0600, never overwritten), not to stdout.
- A bulk run is all or nothing: if the key file cannot be updated, the CSV is deleted.
- Copy the key file to every instance. Each instance logs the key count and file checksum on every reload; compare checksums across instances.
- A bad key file stops startup; during a reload it is rejected and the previous keys stay active.

## Site configs

One file per site in `configs/sites/`, usually extending a template in `configs/templates/`. Objects merge deeply; a site's arrays replace the template's.

```bash
go run ./cmd/sitectl validate                          # schema and references
go run ./cmd/sitectl check --url <product url>         # live extraction with full diagnostics
go run ./cmd/sitectl fixture --url <product url> --name product-sale   # save HTML + expected output
go run ./cmd/sitectl test                              # run all fixtures offline
go run ./cmd/sitectl coverage                          # sites, status, version, template, fixtures
```

Every command accepts `--json`. Every enabled site needs at least one fixture; `sitectl test` fails otherwise.

The two fixture sets in `testdata/sites/` were written by hand from typical WoodMart and PrestaShop markup. Replace them with real pages using `sitectl fixture` before relying on the selectors.


You can run it any of these three ways:

With Go, straight from the repo (nothing to install):

```bash
go run ./cmd/apikey generate-pepper
Install it once so plain apikey works:

```

```bash
go install ./cmd/apikey
apikey generate-pepper
```


This builds the program into ~/go/bin. That folder is already on your PATH, since swag runs from there.

With Docker, using the program inside the image:

```
docker compose run --rm apikey generate-pepper
```

## Responsible use

This service downloads product pages from shop websites. When a search engine is enabled, it also sends queries to third-party search services. Whoever runs an instance decides which websites and services it contacts, and is responsible for that use.

**The license covers this code only.** The MIT license ([LICENSE](LICENSE)) lets you use, copy and change the code in this repository. It gives you no rights to the websites listed in `configs/sites/` or their content, or to the services of DuckDuckGo, Brave, Mojeek, Yahoo, Yandex or any other provider. Those rights can only come from the website owners and the providers, under their own terms.

**Before you enable a site or an engine, check that your use is allowed by:**

- **The website's terms of use.** A config in this repository shows how a site's pages are built. It does not mean the site allows automated extraction.
- **DuckDuckGo's [Terms of Service](https://duckduckgo.com/terms) and [Acceptable Use Policy](https://duckduckgo.com/acceptable-use)**, for the `duckduckgo` engine. DuckDuckGo offers no API for web results and has not approved this project. The engine is off by default.
- **Brave's terms for your [Brave Search API](https://brave.com/search/api) plan**, for the `brave` engine, including whether you may store results. Use your own API key. The engine is off by default.
- **[Mojeek's](https://www.mojeek.com/about/terms/), [Yahoo's](https://legal.yahoo.com/) and [Yandex's](https://yandex.com/legal/) terms**, for the `mojeek`, `yahoo` and `yandex` engines. These engines read the provider's public results page with the honest `APP_SEARCH_USER_AGENT`; none of the three providers has approved this project, and none is contacted through an API. Each engine is off by default, and each has its own switch.
- **Your AI provider's terms**, for the `ai-crawler-avalai` engine (AvalAI and the model provider behind it) and the `ai-crawler-cloudflare` engine (Cloudflare and the model provider behind its gateway). Each engine sends the content of every page it serves, and its URL, to the provider. By default the Cloudflare gateway logs prompts and answers; this service sends `cf-aig-collect-log: false` unless `APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG=true`. A token with the AI Gateway Run permission works on every gateway of the account, so use a dedicated token and keep it in `secrets/`. Both engines are off by default.
- **The laws that apply to you**, for example on copyright, databases and personal data.

**What each engine does:**

| Engine | Contacts the website | Notes |
|---|---|---|
| `http` | Yes, with plain HTTP requests | Follows the site's `robots.txt` unless the site config sets `respect_robots_txt` to `false` |
| `browser` | Yes, in a real Chrome browser | Runs the page's JavaScript like a normal visitor's browser. This can get past some anti-bot checks, for example Cloudflare's "Just a moment..." page. Some websites' terms forbid this, so check before you use it for a site |
| `ai-crawler-avalai`, `ai-crawler-cloudflare` | Yes, with plain HTTP requests (Chrome for JavaScript pages when the browser engine is on) | Always follows `robots.txt` for hosts without a config. Sends the cleaned page to the AI provider (AvalAI, or Cloudflare AI Gateway). The one named by `APP_DEFAULT_ENGINE` serves any host without a rule |
| `default` (in a site config) | Follows `APP_DEFAULT_ENGINE` | The site uses the same AI engine as the built-in rule for hosts without a config (`ai-crawler-avalai` or `ai-crawler-cloudflare`). With `APP_DEFAULT_ENGINE=none` (or the AI crawler off) a request for the site fails with `ENGINE_UNAVAILABLE` |
| `duckduckgo`, `brave`, `mojeek`, `yahoo`, `yandex` | No | Sends search queries to the provider only, and builds the product from the search result. The queries carry the honest `APP_SEARCH_USER_AGENT`; the service never pretends to be a browser and never retries a block |

**No affiliation.** DuckDuckGo, Brave, Mojeek, Yahoo, Yandex, and the shop names used in `configs/sites/` are trademarks of their owners. This project is not affiliated with or endorsed by any of them.

**Not legal advice.** This section is general information, not legal advice. If you are unsure whether your use is allowed, ask a lawyer. As [LICENSE](LICENSE) states, the software is provided "as is", without warranty of any kind.

## Run with docker compose

```bash
docker compose down                          # stop the restart loop
sudo rm -rf secrets && mkdir secrets         # replace the root-owned empty folder (or: sudo chown $USER: secrets)
docker compose run --rm apikey generate --name my-client   # prints the key once
docker compose up --build
```

## Run distributed (client-side load balancing)

The service is stateless, so it can run on many nodes at once: different machines, containers, or exit IPs. The nodes do not talk to each other and share no state. The client is the dispatcher. It asks each node how busy it is and sends each crawl to the node best able to take it.

### Architecture

```mermaid
flowchart LR
    subgraph CLIENT["Client application (dispatcher)"]
        direction TB
        Q["Crawl queue<br/>product URLs"]
        LB["Node picker<br/>skip draining / full / site-busy<br/>pick most available"]
        POLL["Load poller<br/>every 1-5 s"]
        Q --> LB
        POLL -. node load table .-> LB
    end

    subgraph SHARED["Same files on every node (copied, not shared at runtime)"]
        direction TB
        CFG[("configs/sites/*.json<br/>parsing rules")]
        KEYS[("API key file<br/>+ APP_API_KEY_PEPPER")]
    end

    subgraph NODEA["Node A  (capacity 20)"]
        direction TB
        AUTH["API key middleware<br/>X-API-Key"]
        DRAIN{"draining?"}
        GL["Instance limiter<br/>APP_MAX_CONCURRENT_REQUESTS"]
        SL["Per-site limiter<br/>slots + delay_ms spacing"]
        SVC["Extract service<br/>lookup site, fetch, parse, diagnose"]
        FETCH["Fetcher<br/>SSRF checks, retries, timeouts"]
        REG["Site config registry<br/>hot reload every 30 s"]
        KS["Key store<br/>hot reload every 30 s"]
        MET["Metrics<br/>counters + live load"]
        AUTH --> DRAIN --> GL --> SL --> SVC --> FETCH
        REG --> SVC
        KS --> AUTH
        GL -. active / available .-> MET
        SL -. sites_active .-> MET
        SVC -. outcome, duration .-> MET
    end

    NODEB["Node B  (capacity 20)<br/>same components"]
    NODEC["Node C  (capacity 40)<br/>same components"]

    subgraph EGRESS["Outbound"]
        direction TB
        PXA["Proxy A<br/>APP_PROXY_HOST/PORT"]
        PXB["Proxy B"]
        PXC["Direct / proxy C"]
    end

    SITES(["Shop websites<br/>one site config per domain"])
    PROM["Prometheus<br/>scrapes /metrics as text"]

    LB == "POST /api/v1/extract" ==> AUTH
    LB == "POST /api/v1/extract" ==> NODEB
    LB == "POST /api/v1/extract" ==> NODEC

    POLL -- "GET /api/v1/active<br/>GET /metrics (JSON)" --> MET
    POLL -- "GET /metrics" --> NODEB
    POLL -- "GET /metrics" --> NODEC

    CFG --> REG
    KEYS --> KS
    CFG -.-> NODEB
    CFG -.-> NODEC
    KEYS -.-> NODEB
    KEYS -.-> NODEC

    FETCH --> PXA --> SITES
    NODEB --> PXB --> SITES
    NODEC --> PXC --> SITES

    PROM -. "X-API-Key" .-> MET
    PROM -.-> NODEB
    PROM -.-> NODEC
```

- **Client (dispatcher).** It owns the crawl queue and decides which node gets each URL. It is the only part that knows about every node.
- **Nodes.** They are identical and independent. Each one runs the whole pipeline (auth → drain check → instance limiter → per-site limiter → extract → fetch) and reports its own load. Nodes never call each other.
- **Shared files.** The site configs and the key file are copied to every node. Each node reloads them on its own every 30 seconds. Nothing is shared at runtime, so there is no single point of failure.
- **Outbound traffic.** Each node can fetch through its own proxy, so the cluster crawls from several IP addresses.

### One request, end to end

```mermaid
sequenceDiagram
    autonumber
    participant D as Client (dispatcher)
    participant A as Node A
    participant B as Node B
    participant S as Shop website

    loop every 1-5 s
        D->>A: GET /metrics (X-API-Key)
        A-->>D: load: available 0, sites_active {hmland: 1}
        D->>B: GET /metrics
        B-->>D: load: available 5, draining false
    end

    Note over D: URL is on hmland.ir<br/>A is full, B has 5 free slots → pick B<br/>count B as one busier until the next poll

    D->>B: POST /api/v1/extract {url}
    B->>B: auth, take instance slot, take hmland slot, wait delay_ms
    B->>S: GET product page (via B's proxy)
    S-->>B: HTML
    B->>B: parse, normalize price, diagnose
    B-->>D: 200 {status: success, product} or {status: failed, error}

    alt 429 TOO_MANY_REQUESTS (raced another dispatcher)
        B-->>D: 429 + Retry-After
        D->>A: retry the same request on the next best node
    else 503 SHUTTING_DOWN
        B-->>D: 503
        Note over D: drop B until GET /ready returns 200
    end
```

### Set up the nodes

Every node needs the same:

- `APP_API_KEY_PEPPER`, so one client key works on all nodes.
- Key file (`APP_API_KEYS_FILE`). Copy it to each node, then compare the checksum each node logs on reload.
- Site configs (`APP_SITE_CONFIG_DIR`). Use `GET /api/v1/sites` to check that each node serves the same versions.

Limits are per node. `APP_MAX_CONCURRENT_REQUESTS` caps a single node, so the cluster's total capacity is the sum over all nodes. Nodes can have different capacities. Give larger machines a higher value.

### Endpoints for the dispatcher

All of them need the client `X-API-Key` and respond with `Cache-Control: no-store`.

| Endpoint | Returns | Use it for |
|---|---|---|
| `GET /api/v1/active` | `{"extract_active_requests": N}` | Cheapest check: how many extractions this node is running now |
| `GET /metrics` | JSON `load` block plus counters | Full picture: capacity, free slots, busy sites, draining |
| `GET /ready` | 200, or 503 while draining (public) | Taking a node out of rotation during a deploy |

The `load` block of `GET /metrics`:

```jsonc
"load": {
  "draining": false,          // true: the node is shutting down; send it nothing
  "capacity": 8,              // APP_MAX_CONCURRENT_REQUESTS
  "active": 3,                // same number as /api/v1/active
  "available": 5,             // capacity - active; 0 means the next request gets 429
  "utilization": 0.375,       // active / capacity
  "per_site_limit": 1,        // APP_MAX_CONCURRENT_PER_SITE
  "sites_active": {"hmland": 1} // busy sites on this node
}
```

`active` includes requests that are waiting for a busy site's slot on this node, not only requests that are fetching.

### Choosing a node

1. Poll every node every 1–5 seconds, and also after each response. Use `/metrics` if the nodes have different capacities. If they are all the same size, `/api/v1/active` is enough.
2. Remove any node that is `draining`, that fails to answer, or that has `available` at 0.
3. For a URL on site X, prefer nodes where `sites_active[X]` is below `per_site_limit`. On any other node the request waits up to `APP_SITE_WAIT_TIMEOUT`, then gets a 429.
4. From the rest, pick the node with the most `available` slots, or the lowest `utilization` when capacities differ. Break ties at random so the dispatchers don't all pick the same node.
5. Between polls, add one to the chosen node's count yourself, so a burst of requests doesn't all land on one node.
6. The numbers can change between your poll and your request. On a 429, read `Retry-After` and try another node. On a 503 (`SHUTTING_DOWN`), drop the node until `/ready` returns 200 again.

```go
func pick(nodes []Node, site string) *Node {
	var best *Node
	for i := range nodes {
		n := &nodes[i]
		l := n.Load // from the last GET /metrics, plus requests sent since
		if n.Down || l.Draining || l.Available <= 0 || l.SitesActive[site] >= l.PerSiteLimit {
			continue
		}
		if best == nil || l.Available > best.Load.Available {
			best = n
		}
	}
	return best // nil: every node is busy; wait and poll again
}
```

### Things to know

- **Politeness is per node.** Each node keeps its own `delay_ms` spacing and per-site limit. Three nodes crawling the same site can hit it three times as often. To stay gentle, send each site to one node, or lower the per-site settings.
- **Retry the same request.** An extraction stores nothing, so you can resend the same request to another node after a 429, a 503, or a network error.
- **Different exit IPs.** Give each node its own `APP_PROXY_HOST` and `APP_PROXY_PORT` to crawl from different addresses.
- **Prometheus.** Scrape every node's `/metrics` with the `X-API-Key` header. Prometheus asks for the text format, so it gets the `extract_*` series. Labelled with `instance`, they show load per node.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `APP_ENV` | none | `development`, `test`, or `production` |
| `APP_HTTP_PORT` | `4000` | Listen port |
| `APP_HTTP_ADDRESS` | all interfaces | Listen address |
| `APP_SITE_CONFIG_DIR` | `configs` | Config root |
| `APP_API_KEYS_FILE` | `secrets/.env.apikeys` in production, otherwise `secrets/api_keys.json` | Key file |
| `APP_API_KEY_PEPPER` | none, required (32+ chars) | HMAC secret |
| `APP_OPS_USER`, `APP_OPS_PASSWORD_HASH` | none | Operator credentials for Swagger UI; it returns 404 when unset |
| `APP_TLS_CERT_FILE`, `APP_TLS_KEY_FILE` | none | Serve HTTPS directly |
| `APP_MAX_REQUEST_DURATION` | `3m` | Hard limit per request |
| `APP_SITE_WAIT_TIMEOUT` | `30s` | Wait for a busy site's turn before 429 |
| `APP_MAX_CONCURRENT_REQUESTS` | `20` | Per-instance capacity |
| `APP_MAX_CONCURRENT_PER_SITE` | `1` | Per-site capacity |
| `APP_RELOAD_INTERVAL` | `30s` | How often config and key files are checked |
| `APP_PROXY_HOST`, `APP_PROXY_PORT` | none (direct) | HTTP proxy for every page fetch, for example `127.0.0.1` and `3067`. Destinations are still checked for blocked ports and private addresses before being sent to the proxy. `sitectl` reads the same variables |
| `APP_BROWSER_ENABLED` | `false` | Turn on the `browser` fetch engine (real Chrome via Playwright) for sites with `"fetch": {"engine": "browser"}`. Chrome launches at startup; the app does not start if it cannot. Without it, such sites fail with `ENGINE_UNAVAILABLE` |
| `APP_BROWSER_CHANNEL` | `chromium` | `chromium` (Playwright's bundled build) or `chrome` (installed Google Chrome). `sitectl` reads it too |
| `APP_BROWSER_HEADLESS` | `true` | Run Chrome without a window; `false` needs a display (or Xvfb) |
| `APP_BROWSER_MAX_PAGES` | `3` | Browser pages open at once per instance (each uses 100-300 MB) |
| `PLAYWRIGHT_DRIVER_PATH` | user cache | Where the Playwright driver (Node.js and `playwright-core`) is installed |
| `APP_SEARCH_USER_AGENT` | `go-wordpress-extractor/1.0` | Honest client name sent to the search providers. Do not put personal contact details in it unless you mean to share them |
| `APP_SEARCH_CACHE_TTL` | `6h` | How long a matched search result is kept in memory. `0` turns the cache off |
| `APP_SEARCH_NO_RESULT_CACHE_TTL` | `1h` | How long a `SEARCH_NO_RESULT` answer is kept, so repeated requests for an unindexed product do not query the provider again |
| `APP_DUCKDUCKGO_ENABLED` | `false` | Turn on the `duckduckgo` engine. No key. Without it (and without an enabled alternate), such sites fail with `ENGINE_UNAVAILABLE` |
| `APP_DUCKDUCKGO_MIN_INTERVAL_MS` | `2000` | Minimum time between two DuckDuckGo queries from this instance, across all sites |
| `APP_BRAVE_ENABLED` | `false` | Turn on the `brave` engine. Needs a key; the app does not start without one |
| `APP_BRAVE_API_KEY` | none | **Secret.** Your Brave Search API key, sent as `X-Subscription-Token`. Never logged. Never put it in `.env.development` or `.env.test`: they are committed |
| `APP_BRAVE_API_KEY_FILE` | none | A file holding only the key (for example `secrets/brave_api_key`; `secrets/` is git-ignored). Used when `APP_BRAVE_API_KEY` is empty |
| `APP_BRAVE_MIN_INTERVAL_MS` | `1000` | Minimum time between two Brave queries. `1000` fits the free tier's 1 query per second |
| `APP_MOJEEK_ENABLED` | `false` | Turn on the `mojeek` engine (Mojeek's results page through `github.com/jcalvert/metawebsearch`). No key |
| `APP_MOJEEK_MIN_INTERVAL_MS` | `3000` | Minimum time between two Mojeek queries from this instance |
| `APP_YAHOO_ENABLED` | `false` | Turn on the `yahoo` engine (Yahoo's results page, Bing's index). No key |
| `APP_YAHOO_MIN_INTERVAL_MS` | `3000` | Minimum time between two Yahoo queries |
| `APP_YANDEX_ENABLED` | `false` | Turn on the `yandex` engine (Yandex's results page). No key. Its own switch, so an operator who does not want Yandex traffic leaves it off |
| `APP_YANDEX_MIN_INTERVAL_MS` | `3000` | Minimum time between two Yandex queries |
| `APP_DEFAULT_ENGINE` | `ai-crawler-avalai` with the AI crawler on, `none` with it off | Engine for any host with no site config and no active learned config: `ai-crawler-avalai`, `ai-crawler-cloudflare` or `none` (`SITE_NOT_CONFIGURED`). A site config can set `fetch.engine: default` to follow the same engine. The app does not start if it names an engine that is not configured, or an engine that needs selectors (`http`, `browser`, search). It replaced `APP_AI_CRAWLER_DEFAULT_RULE` (`true` → `ai-crawler-avalai`, `false` → `none`); the app does not start while that is still set |
| `APP_AI_CRAWLER_ENABLED` | `false` | Turn the AI engines on. Each engine is on when its own settings are complete: AvalAI needs a key, Cloudflare an account id and a token. With it on and no engine configured, the app does not start |
| `APP_AI_CRAWLER_BASE_URL` | `https://api.avalai.ir/v1` | AvalAI: OpenAI-compatible API base. Must be `https` (plain `http` only for localhost) |
| `APP_AI_CRAWLER_MODEL` | `deepseek-v4.1-flash` | AvalAI: the only model used; there is no fallback model |
| `APP_AI_CRAWLER_SEARCH_MODEL` | `sonar` | AvalAI: the model that searches the web itself, for sites with `fetch.ai_fetch: "web_search"` (the site is never contacted; values come back with the warning `AI_UNVERIFIED`). `none` turns that mode off |
| `APP_AI_CRAWLER_API_KEY` | none | **Secret.** AvalAI's API key, sent as `Authorization: Bearer`. Never logged or returned. Only in a git-ignored file (`.env`, `.env.production`) or the shell |
| `APP_AI_CRAWLER_API_KEY_FILE` | none | A file holding only the key, for example `secrets/avalai_api_key`. Used when `APP_AI_CRAWLER_API_KEY` is empty |
| `APP_AI_CRAWLER_TIMEOUT` | `45s` | One LLM call (each retry gets its own) |
| `APP_AI_CRAWLER_MAX_INPUT_CHARS` | `60000` | Cleaned page sent to the model |
| `APP_AI_CRAWLER_MAX_OUTPUT_TOKENS` | `2000` | The model's answer |
| `APP_AI_CRAWLER_TEMPERATURE` | `0` | Sampling temperature |
| `APP_AI_CRAWLER_RETRIES` | `2` | Retries of 408, 409, 429 and 5xx answers, honoring `Retry-After` (done by the `openai-go` SDK) |
| `APP_AI_CRAWLER_MIN_INTERVAL_MS` | `500` | AvalAI: minimum time between two LLM calls from this instance |
| `APP_AI_CRAWLER_CACHE_TTL` | `6h` | The same page and model reuse the answer; `0` turns the cache off |
| `APP_AI_CRAWLER_RESPONSE_FORMAT` | `json_object` | `json_object`, or `json_schema` (strict schema; test it on the model first) |
| `APP_AI_CRAWLER_DEFAULT_DELAY_MS` | `2000` | Gap between two fetches of the same host under the default rule |
| `APP_AI_CRAWLER_LEARN_SELECTORS` | `true` | Learn CSS selectors from AI results |
| `APP_AI_CRAWLER_LEARN_DIR` | `<APP_SITE_CONFIG_DIR>/learned` | Learned configs; must be writable by the service |
| `APP_AI_CRAWLER_LEARN_PROMOTE_AFTER` | `3` | Different product pages that must match in shadow before a learned config serves its host |
| `APP_AI_CRAWLER_LEARN_DEMOTE_AFTER` | `3` | Failures in a row that demote an active learned config |
| `APP_AI_CRAWLER_LEARN_MAX_ATTEMPTS_PER_DAY` | `3` | Learning calls per host per day |
| `APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID` | none | Cloudflare: account id (32 hex characters). The `ai-crawler-cloudflare` engine is configured only when it is set |
| `APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID` | `default` | Cloudflare: gateway id (at most 64 characters); `default` is created on first use |
| `APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT` | `rest` | `rest` (`https://api.cloudflare.com/client/v4/accounts/<id>/ai/v1`, gateway in `cf-aig-gateway-id`) or `compat` (`https://gateway.ai.cloudflare.com/v1/<id>/<gateway>/compat`, deprecated by Cloudflare for single-model calls) |
| `APP_AI_CRAWLER_CLOUDFLARE_BASE_URL` | none | Full override of the base URL (tests, a future endpoint). `https` only, except localhost |
| `APP_AI_CRAWLER_CLOUDFLARE_MODEL` | `@cf/deepseek-ai/deepseek-v4.1-flash-1017` | Model id from the Cloudflare catalog; `{provider}/{model}` for other providers, `dynamic/<route>` for a dynamic route. On `compat`, Workers AI models are written `workers-ai/@cf/...` |
| `APP_AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT` | `none` | Sent as `reasoning_effort`: `none`, `low`, `high` or `max` |
| `APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN` | none | **Secret.** Cloudflare API token with "Workers AI Read" and "AI Gateway Run" (`rest`), or the gateway token (`compat`) |
| `APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE` | none | A file holding only the token, for example `secrets/cloudflare_api_token` |
| `APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY` (`_FILE`) | none | **Secret**, `compat` only: the provider's own key. On `rest` the app does not start with it set; store the key in the gateway (BYOK) instead |
| `APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING` | `true` | Sends `cf-aig-no-wholesale: true`: a call with no usable key fails (`AI_AUTH_FAILED`) instead of spending Unified Billing credits. Workers AI models are billed to the account either way |
| `APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG` | `false` | `false` sends `cf-aig-collect-log: false`, so the gateway keeps no copy of the page text and the answer |
| `APP_AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS` | `500` | Cloudflare: minimum time between two LLM calls from this instance |

The shared AI settings (`TIMEOUT`, `MAX_INPUT_CHARS`, `MAX_OUTPUT_TOKENS`, `TEMPERATURE`, `RETRIES`, `CACHE_TTL`, `RESPONSE_FORMAT`, `DEFAULT_DELAY_MS`, `LEARN_*`) apply to both engines.

### Fetch engines

Each site picks how its pages are fetched with `fetch.engine` in its config (Strategy pattern: `fetch.Engine` is the interface, `fetch.Router` picks the engine per request):

| `fetch.engine` | Engine | Use for |
|---|---|---|
| `http` (default) | `fetch.Fetcher`: Colly over Go's HTTP client | Server-rendered pages. Fast (under 1 s), small memory |
| `browser` | `fetch.BrowserFetcher`: Playwright drives Chrome | Pages rendered by JavaScript (`JS_RENDERED_PAGE`). 3-10 s per page |
| `duckduckgo` | `fetch.SearchFetcher` with DuckDuckGo's HTML results page | Sites behind bot protection that should not be fetched at all. Never contacts the site. About 1-3 s |
| `brave` | `fetch.SearchFetcher` with the Brave Search API | The same, through a paid, licensed API with your own key |
| `ai-crawler-avalai` | `fetch.AIEngine`: fetches with `http` (or `browser`), then an LLM on AvalAI reads the page | Hosts without a site config (default rule), or sites whose layout changes often. 3-15 s, costs tokens |
| `ai-crawler-cloudflare` | The same `fetch.AIEngine`, with the LLM call through Cloudflare AI Gateway | The same, on Cloudflare (Workers AI or any provider behind the gateway) |
| `mojeek`, `yahoo`, `yandex` | `fetch.SearchFetcher` with that engine's HTML results page, parsed by [`github.com/jcalvert/metawebsearch`](https://github.com/jcalvert/metawebsearch) | The same, through other indexes (Mojeek's own, Bing's through Yahoo, Yandex's own). No key. Each engine has its own switch |

Every engine is described in [md/ENGINES.md](md/ENGINES.md). A site can also list `alternate_engines` (search engines only). The router uses the first engine from `[engine, ...alternate_engines]` that is enabled on the instance. That choice is made once per request: a failed fetch is never retried on another engine, and a request never switches from `http` or `browser` to search.

All of Chrome's traffic goes through an in-process egress proxy that dials with the same SSRF checks and outbound proxy as the `http` engine. Build the image with Chrome with `docker build --target runtime-browser .`. See [md/browser-engine.md](md/browser-engine.md) for how to run, start and stop it, and [md/real-chrome-playwright.md](md/real-chrome-playwright.md) for the design. For local development without a Chrome window on screen, start the server with `./deploy.sh dev` (`xvfb-run -a go run ./cmd/server`).

### Search engines (duckduckgo, brave, mojeek, yahoo, yandex)

For a site behind bot protection (Cloudflare, Akamai, ...) that the service should not get past, set a search engine. The service then never contacts the site: it looks the product URL up in the search engine and builds the product from the matching result. Used today by `lumens.com`; `fergusonhome.com` used it until 2026-11-19 and now uses `default` (follows `APP_DEFAULT_ENGINE`, resolved to `ai-crawler-avalai` on this instance) with `ai_fetch: web_search`. The design and the tests behind it are in [md/SEARCH_ENGINE_PROPOSAL.md](md/SEARCH_ENGINE_PROPOSAL.md) (DuckDuckGo, Brave) and [md/METAWEBSEARCH_ENGINE_PROPOSAL.md](md/METAWEBSEARCH_ENGINE_PROPOSAL.md) (Mojeek, Yahoo, Yandex).

| Engine | Source | Index | Key | Region |
|---|---|---|---|---|
| `duckduckgo` | DuckDuckGo's HTML results page | Mostly Bing | No | Yes |
| `brave` | Brave Search API | Brave's own | **Yes** | Yes |
| `mojeek` | Mojeek's results page, through `metawebsearch` | Mojeek's own | No | Yes |
| `yahoo` | Yahoo's results page, through `metawebsearch` | Bing | No | No |
| `yandex` | Yandex's results page, through `metawebsearch` | Yandex's own | No | No |

Pick an engine and an alternate that use **different indexes**: a product DuckDuckGo lacks is usually missing from Yahoo too. The `mojeek`, `yahoo` and `yandex` engines use only the request builders and result parsers of `github.com/jcalvert/metawebsearch`, not its browser-impersonating HTTP client: their queries go through the service's egress client with `APP_SEARCH_USER_AGENT`, a block is reported as `SEARCH_PROVIDER_BLOCKED` and never retried, and the results page is capped at 2 MiB.

```json
"fetch": {
  "engine": "duckduckgo",
  "alternate_engines": ["brave"],
  "timeout_seconds": 20,
  "search": {
    "region": "us-en",
    "title_strip": [" | Ferguson Home"],
    "ignore_params": ["uid"],
    "queries": ["site_slug", "url"]
  }
}
```

| Key | Default | Meaning |
|---|---|---|
| `search.region` | `wt-wt` (none) | `<country>-<lang>`, for example `us-en`, `de-de`. Sent as `kl` to DuckDuckGo, as `country` + `search_lang` to Brave and as the `arc` + `lb` cookies to Mojeek. Yahoo and Yandex have no region parameter (the loader warns when a region is set for them) |
| `search.title_strip` | `[]` | Suffixes or prefixes removed from the result title |
| `search.ignore_params` | `[]` | Query parameters ignored when comparing a result URL with the requested URL (Ferguson's variant `uid`) |
| `search.queries` | `["site_slug", "url"]` | Queries tried in order, at most 2: `site_slug` = `site:<domain> <words of the URL slug>`, `url` = the product URL in quotes |

**How a request runs.** The engine sends the first query, and the second only if the first had no match. A result is used only if its URL is exactly the requested URL (lowercase host without `www.`, no scheme, no trailing `/`, no tracking or ignored parameters), so a similar product (another size, another colour) is never returned instead. Queries to one provider are spaced by its `APP_<ENGINE>_MIN_INTERVAL_MS`, and answers are cached in memory.

**What comes back.** A search result has a title, a URL and a snippet, so a search response is almost always `"completeness": "partial"`:

| Field | From search |
|---|---|
| `title` | The result title, without `title_strip` and without the `...` a search engine adds to a long title (warning `SEARCH_TITLE_TRUNCATED`) |
| `url` | The matched result URL |
| `description` | The result snippet (warning `SEARCH_SNIPPET`: shortened, maybe out of date) |
| `image_url`, `price`, `specs` | Never. Listed in `unavailable_fields` with reason `NOT_IN_SEARCH_RESULTS`; `price.state` is `unknown` and `PRICE_MISSING` is warned as before |

`source` says which engine answered (`"site_contacted": false`, the query, the result's rank, and `cached`). Search indexes do not hold every product: on 2026-11-13 DuckDuckGo had the Ferguson sample product at rank 1 but not the Häfele one. When no result matches, the request fails with `SEARCH_NO_RESULT` (not retryable; retry in a few days), and `evidence.search` lists the queries, how many results each returned, and up to 3 `closest` product URLs on the site for a person to check. They are never used as the product.

| Failure code | Meaning |
|---|---|
| `SEARCH_NO_RESULT` | No result is the requested product URL |
| `SEARCH_PROVIDER_BLOCKED` | DuckDuckGo, Mojeek, Yahoo or Yandex answered HTTP 202, 403 or 429, or a bot-check page. The service does not work around it and does not retry |
| `SEARCH_QUOTA_EXCEEDED` | Brave answered 429 (per-second limit or plan quota) |
| `SEARCH_PROVIDER_AUTH_FAILED` | Brave answered 401/403: wrong or revoked key, or the plan lacks web search |
| `SEARCH_PROVIDER_ERROR` | Network error, 5xx, or a response the parser cannot read (for the results-page engines, usually a layout change) |

**Turning them on.** All are off by default; see [Responsible use](#responsible-use) first.

```bash
sudo ./deploy.sh search                     # status (never shows the key)
sudo ./deploy.sh search enable duckduckgo   # no key; shows DuckDuckGo's terms and asks
sudo ./deploy.sh search enable brave        # asks for your Brave API key without echoing it
sudo ./deploy.sh search enable mojeek       # no key; also yahoo, yandex
sudo ./deploy.sh search disable brave
sudo ./deploy.sh restart
```

Locally, `.env.development` turns `duckduckgo` on and lists `mojeek`, `yahoo` and `yandex` off. For Brave, put the key alone in `secrets/brave_api_key` (git-ignored) and set `APP_BRAVE_ENABLED=true`, or export `APP_BRAVE_API_KEY` in your shell. `.env.example` lists every search setting.

Unit tests never send real queries: they use the saved pages in `testdata/search/` (real DuckDuckGo pages; synthetic Brave, Mojeek, Yahoo and Yandex responses in each provider's markup). The end-to-end tests in `test/e2e/scraped_search_test.go` run the service with the three results-page engines pointed at the fake shop. To query the real providers once:

```bash
SEARCH_LIVE_TEST=1 go test ./internal/fetch/search/ -run Live -v                        # DuckDuckGo, Mojeek, Yahoo, Yandex
SEARCH_LIVE_TEST=1 APP_BRAVE_API_KEY=... go test ./internal/fetch/search/ -run Live -v   # also Brave
SEARCH_LIVE_TEST=1 SEARCH_LIVE_SAVE=/tmp/pages go test ./internal/fetch/search/ -run Live -v  # also save every raw answer
```

The live test sends the Ferguson sample request with the search settings `configs/sites/fergusonhome.com.json` used until 2026-11-19 (see its `$comment`) through every engine, uses `APP_PROXY_HOST`/`APP_PROXY_PORT` when set (as the server does), and ends with a summary that ranks the engines. A `SEARCH_PROVIDER_BLOCKED` answer from Mojeek, Yahoo or Yandex is a finding about that engine and the honest client, not a test bug. Result on 2026-11-15:

| Engine | Ferguson sample product |
|---|---|
| `duckduckgo` | Found, rank 1 of the first query (about 0.9-1.5 s). This network reaches it only through the proxy |
| `yahoo` | Found, rank 1 of the first query (about 0.9-1.2 s), with or without the proxy |
| `mojeek` | `SEARCH_PROVIDER_BLOCKED`: a CAPTCHA page |
| `yandex` | `SEARCH_PROVIDER_BLOCKED`: "Are you not a robot?" (once, through curl: other Ferguson products, not the requested one) |
| `brave` | Not tried (no API key) |
| `http` | HTTP 403 from the site, which is why it uses a search engine |

The saved answers are the fixtures in `testdata/search/` and are replayed through the real Ferguson config, with those former search settings, by `test/e2e/fergusonhome_search_test.go`.

### AI crawler (ai-crawler-avalai, ai-crawler-cloudflare)

The design, the prompts and every decision are in [md/AI_CRAWLER_AVALAI_PROPOSAL.md](md/AI_CRAWLER_AVALAI_PROPOSAL.md); the second engine is in [md/AI_CRAWLER_CLOUDFLARE_PROPOSAL.md](md/AI_CRAWLER_CLOUDFLARE_PROPOSAL.md).

- **Two engines, one pipeline.** `ai-crawler-avalai` calls AvalAI and `ai-crawler-cloudflare` calls Cloudflare AI Gateway. Cleaning, prompts, grounding, the cache, learning and metrics are shared; what differs (base URL, auth headers, extra headers, error mapping, request-id header) is a provider profile in `internal/aicrawler/provider.go`. There is no fallback from one engine to the other. The Cloudflare engine sends `cf-aig-skip-cache: true` (this service caches answers itself), `cf-aig-metadata` (host, purpose, prompt version) and, by default, `cf-aig-no-wholesale: true` and `cf-aig-collect-log: false`. It does not send the gateway's retry headers: the SDK already retries.

- **Cost on `/metrics`.** Every call to the AI provider is counted, including the one repair call after a non-JSON answer and the selector-learning calls. The JSON report has an `ai` block (`calls`, `failed`, `input_tokens`, `output_tokens`, `cache_hits`, and one `series` entry per engine, model, purpose, host and outcome). The Prometheus text has `extract_ai_calls_total`, `extract_ai_tokens_total{kind="input|output"}`, `extract_ai_call_seconds_total` and `extract_ai_cache_hits_total`, labelled with `engine`, `model`, `purpose` (`extract`, `repair`, `learn`), `host` and `outcome` (`ok` or the `AI_*` code). Counters start at 0 when the process starts; use Prometheus `increase(extract_ai_tokens_total[1d])` for tokens per day. Because the default rule accepts any domain, only the first 200 hosts get their own label and the rest are counted as `other`. The `engine` label was added with `ai-crawler-cloudflare`: dashboards that sum these series must add `engine` to their `sum by (...)`, or sum over it.
- **Default rule.** With `APP_AI_CRAWLER_ENABLED=true`, a URL whose host has no file in `configs/sites/` and no active learned config is served by the built-in rule `ai-crawler-default` (`config_origin: default`) with the engine `APP_DEFAULT_ENGINE` names, instead of failing with `SITE_NOT_CONFIGURED`. A site config always wins, then an active learned config. `config_version` is `builtin-1` with AvalAI and `builtin-1+cloudflare` with Cloudflare. `GET /api/v1/sites` lists the rule last, with `domains: ["*"]`, the engine and its model, sets `accepts_any_domain: true` and names the engine in `default_ai_engine`. `APP_DEFAULT_ENGINE=none` turns the rule off.
- **Per site.** A config can set `"fetch": {"engine": "ai-crawler-avalai"}` or `"ai-crawler-cloudflare"`, or `"engine": "default"` to follow `APP_DEFAULT_ENGINE`; it then needs no `root` or `fields`. `"ai_fetch": "web_search"` (AvalAI) never contacts the site: only the URL goes to `APP_AI_CRAWLER_SEARCH_MODEL`, at least one cited source must be on the site, and the values are unverified (see [md/ENGINES.md](md/ENGINES.md) section 3.7). An engine that is not configured on the instance gives `ENGINE_UNAVAILABLE`. `"locale": {"price_unit": "auto"}` reads the currency from each page, and `"fetch": {"ai_fetch": "browser"}` always renders in Chrome.
- **Pipeline.** The page is fetched with the `http` engine. A JavaScript shell is rendered once in Chrome when `APP_BROWSER_ENABLED=true`; otherwise it fails with `JS_RENDERED_PAGE`. The HTML is cleaned (title, meta, JSON-LD and visible text; scripts, header, nav, footer and hidden elements removed) and sent to `POST {APP_AI_CRAWLER_BASE_URL}/chat/completions` with the official [`openai-go`](https://github.com/openai/openai-go) SDK. The model returns JSON: `name`, `price` (copied text), `url`, `image`, `description`, `specs`, `features`.
- **Grounding.** Every value must appear on the page: the name and spec texts in the page text, the image among the page's image URLs, each price amount among the page's amounts (parsed by the Go price parser, never by the model), and at least 60 % of the description's words. A value that fails is dropped and listed in `unavailable_fields` with reason `AI_NOT_GROUNDED`. Every AI result carries the warning `AI_EXTRACTED`; `source` adds `model`, `prompt_version`, `fetched_with` and `tokens`.
- **Failures.** `AI_UNAVAILABLE`, `AI_RATE_LIMITED`, `AI_QUOTA_EXCEEDED`, `AI_AUTH_FAILED`, `AI_INPUT_TOO_LARGE`, `AI_MODEL_UNAVAILABLE` (wrong model, route or region) and `AI_BAD_OUTPUT` (after one repair request), with `evidence.ai` (model, provider, provider request id, HTTP status, provider code, tokens, and for Cloudflare `gateway_cache`). For Cloudflare the request id is `cf-aig-log-id`, the id to search for in the Cloudflare dashboard, and the provider code is Cloudflare's numeric code (`2009`, `2044`, ...). Keys and tokens are never in a response or a log.
- **Selector learning.** After a successful AI extraction on a host without a config, the model is asked once for CSS selectors. They are kept only if the normal selector pipeline gives the same product on the same page, and are written to `configs/learned/<domain>.json` as a `candidate`. Later AI requests for the host also run the selectors in shadow; after `APP_AI_CRAWLER_LEARN_PROMOTE_AFTER` (3) different pages match, the config becomes `active` and serves the host with no LLM call (`config_origin: learned`, warning `LEARNED_CONFIG`). Learning calls use the default engine, and the learned file records it in `learned.engine`. When learned selectors fail, that request is answered by the default engine (or, with `APP_DEFAULT_ENGINE=none`, the engine that learned them; warning `LEARNED_CONFIG_FAILED`); 3 failures in a row demote the config and the host is learned again. Review learned configs with `go run ./cmd/sitectl learned list|show|activate|reject|adopt` (or `sudo ./deploy.sh learned ...`); `adopt` moves one into `configs/sites/`.
- **Setup.** `sudo ./deploy.sh ai enable avalai` or `sudo ./deploy.sh ai enable cloudflare` on a server (asks for the key or token without echoing it and writes `secrets/avalai_api_key` or `secrets/cloudflare_api_token`), then `sudo ./deploy.sh default-engine` (lists the engines in recommended order and asks; or pass `ai-crawler-avalai`, `ai-crawler-cloudflare` or `none`). `sudo ./deploy.sh ai disable avalai|cloudflare` turns one engine off. Locally, `./menu.sh` item 7 sets up the engines and item 9 sets up the default engine from the same recommended list. `deploy.sh restart` and `menu.sh` "Run server" check `APP_DEFAULT_ENGINE` first, so a default engine that is not configured is reported before the server refuses to start. `deploy.sh` and `menu.sh` rewrite a leftover `APP_AI_CRAWLER_DEFAULT_RULE` into `APP_DEFAULT_ENGINE`. Keep secrets out of `.env.development` and `.env.test`: they are committed.
- **Cloudflare billing.** The default model runs on Cloudflare (Workers AI, "Zero data retention"): no DeepSeek key, billed to the account (Workers Paid plan or AI Gateway credits; not on Workers Free), $0.30 per M input and $1.20 per M output tokens, about $0.0036 per product page. The token needs "Workers AI Read" as well as "AI Gateway Run".

### Response: completeness and unavailable_fields

For every engine, the response returns whatever product data was found and lists every field that was not in `unavailable_fields` (an array in the JSON body; the HTTP status stays 200). The fields checked are `image_url`, `description`, `price`, `availability` and `specs` (and `title` when a failed result carries `partial.product`):

| Reason | Meaning |
|---|---|
| `NOT_ON_PAGE` | The page was fetched, but the field was not on it (`http`, `browser`) |
| `NOT_IN_SEARCH_RESULTS` | The search result does not include it (`duckduckgo`, `brave`, `mojeek`, `yahoo`, `yandex`) |
| `NOT_CONFIGURED` | The site config does not extract it for this site |
| `AI_NOT_GROUNDED` | The AI crawler returned a value that is not on the page, so it was left out (`ai-crawler-avalai`, `ai-crawler-cloudflare`) |

`completeness` is `full` when the list is empty and `partial` otherwise. The existing `FIELD_MISSING` and `PRICE_MISSING` warnings are unchanged.

### Installing Playwright (browser engine)

The browser engine requires Node.js, the driver pinned by `go.mod` (`playwright-go v0.6401.0` uses Playwright **1.64.1**), and the matching browser build. Playwright added Ubuntu 26.04 support in 1.61.0. [Playwright release notes](https://github.com/microsoft/playwright/releases/tag/v1.61.0)

If the machine reaches the internet through a proxy, export it first. Node.js, npm, Go and the browser download all use it:

```bash
export https_proxy=http://127.0.0.1:3067
export http_proxy=http://127.0.0.1:3067
```

**On a server, with deploy.sh (recommended).** `install` asks whether to enable the browser engine; on an existing server use `browser enable`. It installs Node.js, the driver and Chromium into `/opt/go-wordpress-extractor-playwright`, installs Chromium's system libraries (Debian/Ubuntu), writes `APP_BROWSER_*`, `PLAYWRIGHT_DRIVER_PATH` and `PLAYWRIGHT_BROWSERS_PATH` into `.env.production`, and checks that the browser starts as the service user. `-E` keeps the proxy variables through sudo.

```bash
sudo -E ./deploy.sh browser enable    # install Playwright + Chromium and turn the engine on
sudo -E ./deploy.sh restart           # apply
sudo ./deploy.sh browser              # status: enabled?, driver version, installed browsers
sudo -E ./deploy.sh browser install   # reinstall or update without changing settings
sudo ./deploy.sh browser disable      # turn off (files stay)
```

**Server without a screen (Xvfb).** Some pages only load in Chrome with a window, because headless Chrome is detected more often (for example, it stays on Cloudflare's "Just a moment..." page). On a server, that window is drawn on Xvfb, a virtual display in memory (no monitor or GPU, about 20-50 MB RAM). `browser enable` asks "Run Chrome in a window on Xvfb?" (default yes). With yes it sets `APP_BROWSER_HEADLESS=false`, installs Xvfb (`apt-get install -y xvfb`), checks that Chrome starts in a window on a temporary display, and adds a second systemd unit, `go-wordpress-extractor-xvfb`, that runs the display `:99`. The server unit requires it and gets `DISPLAY=:99`. Xvfb runs as its own unit instead of `xvfb-run` around the server, so a stop still sends SIGTERM to the server and running requests finish. To do the same by hand:

```bash
sudo apt-get install -y xvfb
# .env.production
APP_BROWSER_ENABLED=true
APP_BROWSER_CHANNEL=chrome
APP_BROWSER_HEADLESS=false
# then let deploy.sh write both units and restart
sudo ./deploy.sh service && sudo ./deploy.sh restart
sudo ./deploy.sh browser              # shows "window: Chrome runs in a window on Xvfb :99 (service ...: active)"
```

`sudo ./deploy.sh run` (foreground, no systemd) starts the server under `xvfb-run` when there is no display. Cloudflare also scores the server's IP address: a data-center address can still be challenged where a home connection passes, so test the site from the server itself.

`sudo -E ./deploy.sh update` also updates the driver when `go.mod` moves to a new playwright-go version. Use `APP_BROWSER_CHANNEL=chrome` (asked by `browser enable`) to install and use Google Chrome instead of Playwright's Chromium (x86_64 only).

**On a development machine, by hand.** Use Node.js 22 or newer (`node --version`). This puts the driver where playwright-go looks by default, so tests and `go run ./cmd/server` find it without extra variables.

```bash
PW_VERSION=1.64.1
D=~/.cache/ms-playwright-go/$PW_VERSION
mkdir -p "$D" && cd "$(mktemp -d)"
npm pack "playwright-core@$PW_VERSION" --silent
tar -xzf "playwright-core-$PW_VERSION.tgz" -C "$D"
ln -sf "$(command -v node)" "$D/node"
"$D/node" "$D/package/cli.js" --version                          # Version 1.64.1
"$D/node" "$D/package/cli.js" install chromium chromium-headless-shell
sudo "$D/node" "$D/package/cli.js" install-deps chromium         # system libraries, Debian/Ubuntu
cd -
```

Then run the server with the engine on, and the browser tests (they skip when Playwright is missing):

```bash
APP_BROWSER_ENABLED=true go run ./cmd/server
go test ./internal/fetch/ ./test/e2e/ -run 'Browser|Gate|Router|Engine' -v
```

With Google Chrome already installed you can skip the Chromium download and set `APP_BROWSER_CHANNEL=chrome`.

**In Docker.** `docker build --target runtime-browser -t go-wordpress-extractor:browser .` builds an image with all of the above. Behind a proxy on the host:

```bash
docker build --network=host \
  --build-arg https_proxy=$https_proxy --build-arg http_proxy=$http_proxy \
  --target runtime-browser -t go-wordpress-extractor:browser .
```

## Swagger

Swagger UI: http://127.0.0.1:4000/swagger/index.html (operator credentials required). The doc is generated from the `@` annotations on the handlers and the struct tags of the request and response types, as before:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init --parseInternal -g cmd/server/main.go -o docs
```

Regenerate after changing an annotation or an API type. Generate a client from `docs/swagger.json`:

```bash
openapi-generator-cli generate -i docs/swagger.json -g typescript-axios -o ./extract-client
```

How set admin for swagger

APP_OPS_USER=admin
APP_OPS_PASSWORD_HASH=<the hash from step 1>


```bash
echo 'choose-a-long-password' | APP_API_KEY_PEPPER=$(grep '^APP_API_KEY_PEPPER=' .env.development | cut -d= -f2-) go run ./cmd/apikey hash-ops-password
```

## Tests

```bash
go test ./...                       # unit, fixture, contract, and end-to-end tests
go test -race ./test/e2e/           # end-to-end only
go test -coverpkg=./internal/... -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
```

The end-to-end suite starts the real Fx application on a free port with temporary config and key files, and points it at a local fake shop. A test resolver maps hostnames such as `shop.test` to that shop, so redirects, SSRF checks, TLS errors, and DNS failures are all exercised without the internet. It covers every failure code, every price state, request validation, authentication, capacity and per-site limits, per-site delay, cancellation, deadlines, draining, operator endpoints, HTTPS, hot reload of keys and sites, and the `apikey` CLI. Every response in the suite is validated against the generated Swagger doc, and properties missing from it fail the test. `TestSwaggerDocsUpToDate` fails when `docs/` was not regenerated after an annotation or type change (it is skipped if `swag` is not installed).

## Docker

```bash
export APP_API_KEY_PEPPER=...            # the pepper used to hash secrets/.env.apikeys
docker compose up --build                 # service on :4000; configs/ and secrets/ are mounted read-only
docker compose --profile tools up sonar   # optional SonarQube
```

The key file is mounted rather than passed with `--env-file`, so revocations apply without a restart. `stop_grace_period` is 190s so running requests can finish.

## Sonar

```bash
go test -coverprofile=coverage.out -coverpkg=./internal/... ./...
sonar-scanner -Dsonar.host.url=http://localhost:9000 -Dsonar.token=<token>
```
