# Proposal: AI-Crawler-Cloudflare (engine `ai-crawler-cloudflare`), a second AI engine through Cloudflare AI Gateway, and a default engine for websites without a rule

Status: implemented on 2026-11-19 (plan steps 2-5). Step 1, the live check with a real token, is still open: the points in 2.7, 2.8 and 4.1 marked "checked in the live test" are not confirmed yet.

Implementation notes (where the code differs from the text below):
- An empty `APP_DEFAULT_ENGINE` resolves to `ai-crawler-avalai` when `APP_AI_CRAWLER_ENABLED=true` and to `none` when it is false, so a fresh install with the AI crawler off still starts. Only an explicit AI engine with the AI crawler off stops startup.
- A missing secret file (`APP_AI_CRAWLER_API_KEY_FILE`, `..._CLOUDFLARE_API_TOKEN_FILE`) leaves that engine unconfigured instead of stopping startup, unless that engine is the default; then the error names the file.
- An error body the SDK cannot decode (for example `{"error": "Model not found"}`) is still mapped from its HTTP status and text.
- With `APP_DEFAULT_ENGINE=none`, a learned config whose selectors fail falls back to the engine recorded in `learned.engine` (or `ai-crawler-avalai` for older files).
- `scripts/ai_live_check.sh` (section 6) is not written yet.
Builds on: [AI_CRAWLER_AVALAI_PROPOSAL.md](AI_CRAWLER_AVALAI_PROPOSAL.md) (implemented on 2026-11-17).

## 1. Summary

- **New engine `ai-crawler-cloudflare`.** It does exactly what `ai-crawler-avalai` does today, with the same steps:
  - fetches and cleans the page and sends it with the same prompts
  - checks every value against the page
  - uses the cache, learns selectors and counts calls in `/metrics`
  - The one difference is where the LLM call goes: **Cloudflare AI Gateway** instead of AvalAI.
- **What the gateway adds.** Cloudflare AI Gateway sits in front of many model providers: Workers AI, OpenAI, Anthropic, Google, DeepSeek, Grok and more. It adds its own logging, caching, retries, rate limits, spend limits and analytics.
- **New general setting `APP_DEFAULT_ENGINE`.** It is the engine used for any website that has no rule of its own (no file in `configs/sites/` and no active learned config). It is not an AI-crawler setting: it decides what happens to every unknown website.
  - `ai-crawler-avalai` (default): nothing changes for existing installs.
  - `ai-crawler-cloudflare`: unknown websites go to Cloudflare.
  - `none`: unknown websites fail with `SITE_NOT_CONFIGURED`, as before the AI crawler existed.
  - It replaces `APP_AI_CRAWLER_DEFAULT_RULE` (`false` there is `none` here), so there is one switch, not two.
- **Per-site engine choice.** A site config can still pick either engine with `fetch.engine`.
- **The code change is mostly in one place.** The provider-specific parts move into a small "provider profile":
  - base URL
  - auth headers
  - extra headers
  - error mapping
  - request-id header

  `internal/aicrawler` keeps one OpenAI-compatible client (`openai-go`). Grounding, prompts, learning and the tests are shared.

## 2. Cloudflare AI Gateway: what the docs say (read 2026-11-18)

Sources:
- [cloudflare.com/products/ai-gateway](https://www.cloudflare.com/products/ai-gateway/)
- [developers.cloudflare.com/ai-gateway](https://developers.cloudflare.com/ai-gateway/) and its pages: chat-completion, authentication, caching, request-handling, bring-your-own-keys, unified-billing, limits, troubleshooting, glossary, providers/workersai, dynamic-routing

### 2.1 Two OpenAI-compatible endpoints

| Endpoint | Base URL for `openai-go` | Auth | Gateway selected by | Status in the docs |
|---|---|---|---|---|
| **REST** (recommended) | `https://api.cloudflare.com/client/v4/accounts/{ACCOUNT_ID}/ai/v1` | `Authorization: Bearer <Cloudflare API token>` | header `cf-aig-gateway-id: <gateway>` | Recommended for new integrations |
| **compat** | `https://gateway.ai.cloudflare.com/v1/{ACCOUNT_ID}/{GATEWAY_ID}/compat` | Provider key in `Authorization: Bearer`, gateway token in `cf-aig-authorization: Bearer <token>` | the URL path | "For standard single-model chat completions, this endpoint is deprecated", but "will continue to work for existing integrations" |

Both endpoints answer `POST .../chat/completions` in the OpenAI shape, so the existing `openai-go` client works with a different base URL and extra headers. **This proposal uses REST by default** and keeps compat as an option (decision D1).

### 2.2 Model names

- Model names have the form `{provider}/{model}`. Examples copied from the docs:
  - `openai/gpt-5-mini`
  - `google-ai-studio/gemini-2.5-flash`
  - `anthropic/claude-sonnet-4-5`
  - `workers-ai/@cf/meta/llama-3.3-70b-instruct-fp8-fast`
  - `dynamic/<route-name>` (a dynamic route configured in the dashboard)
- The Workers AI page also shows bare `@cf/...` models on the REST endpoint.
- The docs disagree in places (`grok/` vs `xai/`, `claude-opus-5-5` vs `claude-opus-5.5`), and they say "Use the exact model ID from the model catalog" (developers.cloudflare.com/ai/models). **The model string is copied from the catalog and checked with a live call before it ships** (plan step 1).

### 2.3 Who pays, and which key is used

The gateway picks credentials in this order (unified-billing page):

1. **A provider key on the request.** If one is sent, nothing else is checked.
2. **BYOK.** A provider key stored in the gateway under the alias `default`.
3. **Unified Billing.** Cloudflare's own credentials, paid from prepaid credits: 5% fee on credits, provider prices passed through with no markup. It needs an authenticated gateway, and has a limit of 200 requests per 60 s per gateway.

Two more rules:
- **Workers AI needs no provider key.** Cloudflare bills it on your account.
- **Blocking Unified Billing.** Send `cf-aig-no-wholesale: true` (per request), or turn on "Require provider credentials" for the gateway. Without a usable key the call then fails with `400 / 2044` instead of spending credits.

### 2.4 Request headers we use

| Header | Use in this engine |
|---|---|
| `cf-aig-gateway-id` | REST only: which gateway to use (`APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID`) |
| `cf-aig-authorization` | compat only: the gateway token |
| `cf-aig-metadata` | Tags each log entry with `host`, `purpose` (`extract`, `repair`, `learn`) and `prompt_version`, so Cloudflare's logs line up with ours. Max 5 entries. The exact value format is to be checked on the custom-metadata page |
| `cf-aig-skip-cache: true` | Always sent. We cache answers ourselves (`APP_AI_CRAWLER_CACHE_TTL`, keyed on the cleaned page), and the gateway's cache would hide stale pages behind our check. See decision D4 |
| `cf-aig-request-timeout` | Not sent. `openai-go`'s timeout (`APP_AI_CRAWLER_TIMEOUT`) already bounds the call |
| `cf-aig-max-attempts`, `cf-aig-retry-delay`, `cf-aig-backoff` | **Not sent.** The SDK already retries 408/409/429/5xx, and retrying in both places would multiply the attempts (3 × up to 5) |
| `cf-aig-no-wholesale: true` | Sent when `APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING=true`, so a missing key fails instead of spending credits |
| `cf-aig-collect-log` | Sent as `false` when `APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG=false`: the gateway then keeps no copy of the page text and the answer. The exact value format is to be checked on the logging page |

### 2.5 Response headers we read

| Header | Use |
|---|---|
| `cf-aig-log-id` (single step: same as `cf-aig-request-id`) | Stored as `evidence.ai.provider_request_id`, in place of `avalai-request-id`. It is the id to search for in the Cloudflare dashboard |
| `cf-aig-cache-status` (`HIT` / `MISS`) | Stored as `evidence.ai.gateway_cache`. With `cf-aig-skip-cache` it should always be `MISS`; a `HIT` is logged as a warning |
| `cf-aig-model`, `cf-aig-provider` | Only for `dynamic/...` models: the model that answered goes into `source.model` |

### 2.6 Errors

The docs give an HTTP status plus a numeric Cloudflare code. The new profile maps them to the existing codes:

| Cloudflare | Meaning | Our code | Retryable |
|---|---|---|---|
| 401 / 2009, missing or wrong token | Credentials rejected (gateway token, API token, or provider key) | `AI_AUTH_FAILED` | no |
| 400 / 2044 | Provider credentials required (no BYOK key and Unified Billing blocked) | `AI_AUTH_FAILED` | no |
| 403 / 2049, 2042, 2036 | Unified Billing not available, gateway not enabled, or feature not enabled | `AI_AUTH_FAILED` | no |
| 402 / 2052, 2021 | Not enough Unified Billing balance, or a BYOK payment error | `AI_QUOTA_EXCEEDED` | no |
| 429 / 2018, other 429 | Rate limit (Unified Billing 200/min, or the provider's own) | `AI_RATE_LIMITED` | yes, later |
| 413, 7003 "Run input is too large", provider `context_length_exceeded` | Page too long | `AI_INPUT_TOO_LARGE` | no |
| 404 "Model not found", 403 "Model access denied", 451 "Model unavailable in this region", 7000 "No route for that URI", 404 "Dynamic route not found" | Wrong model, account or route | **`AI_MODEL_UNAVAILABLE`** (new) | no |
| 5xx, 2002, 2005, 7005, network errors | Provider or gateway down | `AI_UNAVAILABLE` | yes |

The new code `AI_MODEL_UNAVAILABLE` (stage `extract`, owner `service`) also replaces the AvalAI profile's current mapping of `model_not_found` to `AI_UNAVAILABLE`.

### 2.7 Default model: DeepSeek V4.1 Flash

From the model page ([workers-ai/models/deepseek-v4.1-flash-1017](https://developers.cloudflare.com/workers-ai/models/deepseek-v4.1-flash-1017/), read 2026-11-18):

| | |
|---|---|
| Model ID | `@cf/deepseek-ai/deepseek-v4.1-flash-1017` (REST); `workers-ai/@cf/deepseek-ai/deepseek-v4.1-flash-1017` (compat) |
| Hosting | "Cloudflare-hosted" (Workers AI), "Zero data retention". No DeepSeek key, no BYOK |
| Price | $0.30 per M input tokens, $1.20 per M output tokens, $0.006 per M cached input tokens |
| Plan | "Paid access required": Workers Paid plan or prepaid AI Gateway credits; not on Workers Free |
| Context | 1,048,576 tokens (the 60,000-character page cap is far below it) |
| Reasoning | Levels `none`, `low`, `high` (default), `max`. Sent as `reasoning_effort: "none"`: extraction needs no reasoning, and reasoning tokens cost output tokens and time. Checked in the live test |
| `response_format` | Listed as a parameter; which formats it accepts is not stated. Checked in the live test |

Notes:
- The REST endpoint needs a token with **"Workers AI Read"** permission. A token with only an AI Gateway permission gets `401` / `10000` (REST API page). The token therefore needs both permissions.
- Workers AI does not use provider credentials, so `NO_UNIFIED_BILLING=true` does not block it (bring-your-own-keys page).
- At about 10k input and 500 output tokens per product page, one extraction costs about $0.0036. That is roughly $3.60 per 1,000 pages before the cache and learned selectors.
- The AvalAI model is called `deepseek-v4.1-flash`. Both should be the same model family, but the providers may serve different builds. The accuracy check compares them.

### 2.8 Things the docs do not say (checked by the live test, plan step 1)

- **JSON mode** (also for DeepSeek V4.1 Flash, section 2.7). It is not stated whether `response_format: json_object` / `json_schema` reaches the provider; the troubleshooting page suggests it is passed through. The system prompt already contains the word "JSON", which some providers require for `json_object`. **Default `json_object`.** Some Workers AI models ignore `response_format`; the existing repair call and `ParseProduct` (which takes the outermost `{...}`) cover that.
- **Token counts.** It is not stated whether `usage` is in the response. If a model returns no usage, `/metrics` counts the call with 0 tokens and the evidence says so.
- **Error body.** The JSON shape of an error is not documented. The profile reads the status, then tries both `error.code` (OpenAI shape) and `errors[0].code` (Cloudflare API shape).

## 3. Configuration (`.env`)

### 3.1 The default engine (general setting)

```dotenv
# Engine for any website that has no rule of its own (no site config and
# no active learned config): ai-crawler-avalai (default),
# ai-crawler-cloudflare, or none (unknown websites fail with
# SITE_NOT_CONFIGURED). The chosen engine must be configured.
APP_DEFAULT_ENGINE=ai-crawler-avalai
```

Behavior:
- **Where it applies.** It applies only when no rule matches the URL's host. A site config always wins, then an active learned config, then `APP_DEFAULT_ENGINE`.
- **Allowed values.** `http`, `browser` and the search engines are not allowed: they need a site's CSS selectors, and an unknown website has none. If a future engine can read any page, it can be added to the list.
- **What the default rule shows.** The built-in rule `ai-crawler-default` runs with `APP_DEFAULT_ENGINE`. `GET /api/v1/sites` shows that engine and its model on the default entry, and `accepts_any_domain` is `false` with `none`.
- **Learning calls** use the default engine. With `none`, nothing is learned from unknown websites, because nothing is extracted from them.
- **Replaces `APP_AI_CRAWLER_DEFAULT_RULE`.**
  - `APP_DEFAULT_ENGINE=none` does what `APP_AI_CRAWLER_DEFAULT_RULE=false` does today.
  - The old variable is removed, with no alias, as decided for AvalAI.
  - If it is still set, the server refuses to start with "APP_AI_CRAWLER_DEFAULT_RULE was replaced by APP_DEFAULT_ENGINE", so no setting is silently ignored.
- **Turning AI off.** `APP_AI_CRAWLER_ENABLED=false` still turns every AI engine off. `APP_DEFAULT_ENGINE` must then be `none`, or the server does not start.
- **Engines are turned on by their settings.**
  - An engine is on when `APP_AI_CRAWLER_ENABLED=true` and its own settings are complete: AvalAI needs a key, Cloudflare needs an account id and a token.
  - A site config that names an engine that is not configured gets `ENGINE_UNAVAILABLE`, the same as `browser` today.
- **Startup check.** The server refuses to start, with a message that names `APP_DEFAULT_ENGINE`, if it names an unknown engine, an engine that cannot read unknown websites, or an AI engine that is not configured.

### 3.2 Cloudflare settings

```dotenv
# ---------------------------------------------------------------------------
# AI crawler: engine "ai-crawler-cloudflare" (Cloudflare AI Gateway)
# ---------------------------------------------------------------------------
# Account id (dashboard: right column of the account home) and gateway id.
# The gateway "default" is created automatically on first use.
APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID=
APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID=default
# rest (recommended): api.cloudflare.com/client/v4/accounts/<id>/ai/v1
# compat (deprecated by Cloudflare for single-model calls):
#   gateway.ai.cloudflare.com/v1/<id>/<gateway>/compat
APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT=rest
# Optional full override of the base URL (tests, or a future endpoint).
APP_AI_CRAWLER_CLOUDFLARE_BASE_URL=
# DeepSeek V4.1 Flash, hosted by Cloudflare: no DeepSeek key needed.
# On ENDPOINT=compat it is written workers-ai/@cf/deepseek-ai/deepseek-v4.1-flash-1017.
APP_AI_CRAWLER_CLOUDFLARE_MODEL=@cf/deepseek-ai/deepseek-v4.1-flash-1017
# none (default), low, high or max.
APP_AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT=none
# Secret: Cloudflare API token with "Workers AI Read" and "AI Gateway Run"
# permissions (rest), or the gateway authentication token (compat).
APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN=
APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE=secrets/cloudflare_api_token
# Optional secret: the provider's own key (OpenAI, DeepSeek, ...). Leave
# empty to use a BYOK key stored in the gateway, or Workers AI.
APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY=
APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY_FILE=
# true: never fall back to Unified Billing credits (sends cf-aig-no-wholesale).
APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING=true
# false: ask the gateway not to store prompts (page text) and answers.
APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG=false
# Gap between two calls from this instance (the AvalAI one stays separate).
APP_AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS=500
```

**Shared settings.** These keep their current names and apply to both engines:
- limits: `TIMEOUT`, `MAX_INPUT_CHARS`, `MAX_OUTPUT_TOKENS`, `TEMPERATURE`, `RETRIES`
- cache and format: `CACHE_TTL`, `RESPONSE_FORMAT`
- default rule: `DEFAULT_DELAY_MS`
- learning: `LEARN_*`

**Existing settings stay AvalAI's.** `APP_AI_CRAWLER_BASE_URL`, `APP_AI_CRAWLER_MODEL`, `APP_AI_CRAWLER_API_KEY(_FILE)` and `APP_AI_CRAWLER_MIN_INTERVAL_MS` are not renamed, and there are no aliases, as decided for AvalAI. `.env.example` puts them under an "AvalAI" heading. Renaming them to `APP_AI_CRAWLER_AVALAI_*` is decision D2.

**Checks** (`ValidateAICrawler`, run only when the engine is configured):
- the account id is 32 hex characters, and the gateway id is at most 64 characters
- the endpoint is `rest` or `compat`
- a base URL override must use https, except on localhost
- the model is non-empty and contains no whitespace
- the token is loaded from the file when the variable is empty, and is never logged

### 3.3 Env files

| File | Change |
|---|---|
| `.env` (git-ignored) | `APP_DEFAULT_ENGINE=ai-crawler-avalai` and the Cloudflare block, empty. `APP_AI_CRAWLER_DEFAULT_RULE` removed |
| `.env.example` | Full documented Cloudflare block and `APP_DEFAULT_ENGINE` (empty: `ai-crawler-avalai` with the AI crawler on, `none` with it off). `APP_AI_CRAWLER_DEFAULT_RULE` removed |
| `.env.development` | `APP_DEFAULT_ENGINE=ai-crawler-avalai`; Cloudflare token via `secrets/cloudflare_api_token`, as with AvalAI |
| `.env.test` | AI stays off, so `APP_DEFAULT_ENGINE=none`; Cloudflare unconfigured (no account id). The e2e tests set their own env |

Reminder: `.env` is read only when no `.env.<APP_ENV>` file exists, and `.env.development` is committed. To switch the default engine in development, set it in `.env.development` or in the shell.

## 4. Design

### 4.1 Provider profiles

```go
// internal/aicrawler/provider.go
type Provider struct {
    Engine        string            // "ai-crawler-avalai" | "ai-crawler-cloudflare"
    Name          string            // "avalai" | "cloudflare", for logs and metrics
    BaseURL       string
    APIKey        string            // goes into Authorization: Bearer
    Headers       map[string]string // static extra headers (cf-aig-gateway-id, ...)
    RequestIDHdrs []string          // avalai-request-id | cf-aig-log-id, cf-aig-request-id
    MapError      func(status int, providerCode, cfCode, msg string) string
    Metadata      bool              // send cf-aig-metadata per call
}
```

- **`aicrawler.New`** takes a `Provider` plus the shared `Settings`. `Settings.BaseURL/Model/APIKey` move into the provider.
- **Headers** are added with `option.WithHeader`. The per-call `cf-aig-metadata` is added in `Complete` from the call's context (`WithCall` already carries purpose and host).
- **compat** puts the provider key, if any, in `Authorization` and the token in `cf-aig-authorization`.
- **rest** puts the Cloudflare token in `Authorization`. A provider key cannot be sent on rest without clashing with `Authorization`. Rest therefore relies on BYOK stored in the gateway, or Workers AI. **The validator rejects `PROVIDER_API_KEY` with `ENDPOINT=rest`** and says to store the key in the gateway (BYOK) or use `compat`. This is exactly what the docs leave open; plan step 1 checks it.

### 4.2 Engines

- **Reasoning.** The Cloudflare profile sends `reasoning_effort: "none"` for DeepSeek V4.1 Flash, through `openai-go`'s `ReasoningEffort` field. The setting `APP_AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT` (default `none`) can change it.
- `siteconfig`:
  - new constant `EngineAICloudflare = "ai-crawler-cloudflare"`
  - `IsAI()` covers both engines
  - schema enum and Swagger enums updated
- `fetch.AIEngine` becomes one type with two instances, one per provider. `Name()` returns the provider's engine, and both are registered in the `fetch.Router` when configured.
- **Default rule:** `siteconfig.DefaultAIEntry(engine, delayMS)` takes the engine from `APP_DEFAULT_ENGINE`; with `none` there is no default entry. The registry's version string includes the engine (`builtin-1+cloudflare`), so `config_version` changes when the default engine changes.
- **The cache key** adds the engine: the same model name on two providers is not the same model.
- **Learned configs** hold only selectors, so they are shared. The `$comment` and the `learned` block record which engine and model learned them (`learned.engine`, new, optional).

### 4.3 Response, evidence, metrics

- `source.engine`: `ai-crawler-cloudflare`. `source.model`: the configured model, or `cf-aig-model` for a dynamic route.
- `evidence.ai`:
  - new `provider` (`avalai` / `cloudflare`) and `gateway_cache` (from `cf-aig-cache-status`)
  - `provider_request_id` is `cf-aig-log-id`
- `/metrics`:
  - every `extract_ai_*` series gets an `engine` label
  - the JSON `ai.series[]` gets `engine`
  - The Prometheus label set changes, so existing dashboards must add `engine` to their `sum by`. This is noted in the README.
- `GET /api/v1/sites`: the default entry shows `engine` and `model` of the chosen default. A new list field `default_ai_engine` names it.

### 4.4 Privacy and responsible use

- **Who sees the page.** Cloudflare sees every page sent, and so does the provider behind it.
- **Gateway logs.** By default the gateway logs prompts and answers. Here that means the page text and the product data, kept up to the account's log limits. `COLLECT_LOG=false` (the default in this proposal) asks it not to.
- **Token scope.** A Cloudflare token with AI Gateway Run permission works on **every** gateway in the account and can use its BYOK keys. The docs say this permission cannot be limited to one gateway. Create a dedicated token, keep it in `secrets/` (git-ignored, mode 600), and never in a committed file.

## 5. Scripts and docs

- **`deploy.sh ai`:**
  - shows both engines (configured or not, model) and the default engine
  - `ai enable cloudflare` asks for account id, gateway id, endpoint and model, and reads the token hidden into `secrets/cloudflare_api_token`
  - `default-engine <engine|none>` sets `APP_DEFAULT_ENGINE`
- **`menu.sh` item 7** gets a submenu: AvalAI / Cloudflare / choose the default engine.
- **Docs:**
  - README: engines table, AI crawler section, env-var table
  - `md/ENGINES.md`: new section 3.8
  - `md/CURL_EXAMPLES.md`: one example with `"engine": "ai-crawler-cloudflare"` in `source`
  - Swagger: an example response from the Cloudflare engine

## 6. Tests

**Unit tests:**
- the Cloudflare profile builds the right URL and headers for `rest` and `compat`
- error mapping covers every row of 2.6, including the `errors[0].code` body shape
- the token never appears in errors or logs

**e2e tests** (`test/e2e/aicrawler_cloudflare_test.go`) use a fake gateway built on the existing `fakeLLM`. The fake checks the path, `Authorization`, `cf-aig-gateway-id` / `cf-aig-authorization`, `cf-aig-skip-cache` and `cf-aig-metadata`, and returns `cf-aig-log-id`. Scenarios:

1. `APP_DEFAULT_ENGINE=ai-crawler-cloudflare`: an unknown host is served by Cloudflare, with `source.engine` and `evidence.ai.provider_request_id = cf-aig-log-id`.
2. Default unchanged (`ai-crawler-avalai`): an unknown host goes to AvalAI, and no call reaches the fake gateway.
3. A site config with `fetch.engine: ai-crawler-cloudflare` while the default is AvalAI: each engine is used where configured.
4. `APP_DEFAULT_ENGINE` naming an unconfigured engine, an unknown name, or `http`; or the old `APP_AI_CRAWLER_DEFAULT_RULE` still set: the server does not start, and the message names the variable.
4b. `APP_DEFAULT_ENGINE=none`: an unknown host fails with `SITE_NOT_CONFIGURED`, and configured sites still work. This replaces today's `TestAICrawlerDefaultRuleOff`.
5. A site config naming an unconfigured Cloudflare engine: `ENGINE_UNAVAILABLE`.
6. Every error in 2.6 maps to its code.
7. `compat` endpoint: path and both auth headers.
8. `NO_UNIFIED_BILLING=true` sends `cf-aig-no-wholesale: true`, and 400 / 2044 gives `AI_AUTH_FAILED` with a suggested action that names BYOK.
9. Dynamic route: `cf-aig-model` ends up in `source.model`.
10. `/metrics` counts per engine.
11. Learning with Cloudflare as the default: the learned config records `engine: ai-crawler-cloudflare`.
12. `GET /api/v1/sites` shows the default engine.

**Live check** (`scripts/ai_live_check.sh`, run by hand, not in CI): one product page through each configured engine, printing the model, the tokens and the grounded fields.

## 7. Decisions needed

| # | Question | Proposal |
|---|---|---|
| D1 | Endpoint: `rest` (recommended by Cloudflare) or `compat` (deprecated for single-model calls, but the only one that takes a provider key on the request) | Default `rest`; `compat` as an option |
| D2 | Rename `APP_AI_CRAWLER_BASE_URL/MODEL/API_KEY` to `APP_AI_CRAWLER_AVALAI_*`? | No; keep the names, no aliases |
| D3 | Default Cloudflare model | **Decided 2026-11-18: DeepSeek V4.1 Flash, `@cf/deepseek-ai/deepseek-v4.1-flash-1017`** (section 2.7). The same model family as AvalAI's `deepseek-v4.1-flash`, hosted by Cloudflare, so no DeepSeek key |
| D4 | Gateway cache | Off (`cf-aig-skip-cache: true`); our own cache already covers it and is keyed on the cleaned page |
| D5 | Gateway logs | Off (`COLLECT_LOG=false`) |
| D6 | Unified Billing fallback | Blocked (`NO_UNIFIED_BILLING=true`), so nothing is charged to credits by surprise |
| D7 | Fallback from one engine to the other when a call fails | No, in line with "don't change model". Cloudflare dynamic routes can do fallback inside the gateway if wanted (`dynamic/<route>`) |

## 8. Plan

| Step | Work | Done when |
|---|---|---|
| 1 | Live check by hand with a real token: `@cf/deepseek-ai/deepseek-v4.1-flash-1017` through the gateway, `reasoning_effort: none`, `rest` + `cf-aig-gateway-id`, `json_object`, `usage`, error body shape, BYOK on `rest` | The open points in 2.7, 2.8 and 4.1 are answered; this proposal is updated |
| 2 | `Provider` profiles; the AvalAI profile with no change in behavior | All current tests still pass |
| 3 | `ai-crawler-cloudflare` profile, config, validation, `APP_DEFAULT_ENGINE` (replacing `APP_AI_CRAWLER_DEFAULT_RULE` in code, env files, `deploy.sh`, `menu.sh` and docs) | Unit tests for 2.6 and 3.2 pass |
| 4 | Router, default rule, cache key, evidence, `/metrics` `engine` label, `/api/v1/sites` | e2e scenarios 1–12 pass |
| 5 | `deploy.sh`, `menu.sh`, env files, README, ENGINES.md, curl and Swagger examples | Docs updated; `TestSwaggerDocsUpToDate` passes |
