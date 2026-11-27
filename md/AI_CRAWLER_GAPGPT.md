# GapGPT AI crawler

`ai-crawler-gapgpt` fetches the product page with HTTP or Chrome and sends its URL and cleaned content to GapGPT. The extractor checks the model’s JSON against the page using the same validation, cache, metrics and selector-learning pipeline as the other AI crawlers.

## API references

- The GapGPT quickstart explains how to create an API key in the developer dashboard and shows `https://api.gapgpt.ir/v1` as the base URL for OpenAI-compatible clients. GapGPT’s OpenWebUI guide documents Bearer authentication and model discovery via `GET /v1/models`. The model catalog lists `gapgpt-deepseek-v4.1-flash`, the default for this engine. [GapGPT quickstart](https://gapgpt.ir/platform-v2/docs/quickstart) · [GapGPT OpenWebUI guide](https://gapgpt.ir/platform-v2/docs/use-cases/openwebui) · [GapGPT model catalog](https://aigap.app/platform-v2/pricing)
- The OpenAI reference explains the Bearer header and Chat Completions request and response format. The existing Go client uses the GapGPT API key and base URL, and sends the configured model, messages, token limit and JSON response format. With the default base URL the SDK sends `POST https://api.gapgpt.ir/v1/chat/completions`. [OpenAI API overview](https://developers.openai.com/api/reference/overview) · [Chat Completions reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
- Check GapGPT’s status page in a browser for provider incidents when API requests time out or return 5xx. The link returned HTTP 200, and its HTML required client-side rendering to display status details. [GapGPT status](https://status.gapgpt.ir/status)

## Configuration

### Local menu

Run `./menu.sh` with the project’s Git-ignored `.env`. Choose **7 → g** to enable GapGPT. The menu prompts for the endpoint, model, and API key. It hides key entry and saves the key as `APP_AI_CRAWLER_GAPGPT_API_KEY` in the private env file. Use **9 → 4** to select GapGPT as the default engine and **7 → z** to disable it. Restart the extractor to apply changes.

### Server deployment

```bash
sudo ./deploy.sh ai enable gapgpt
sudo ./deploy.sh ai
sudo ./deploy.sh restart
```

The setup command prompts for the endpoint and model and saves the hidden API key in the installation’s private `.env.production`. It also offers to select GapGPT as the default. You can change that later with `sudo ./deploy.sh default-engine ai-crawler-gapgpt`. To turn off GapGPT, run `sudo ./deploy.sh ai disable gapgpt` and restart. Disabling GapGPT clears its key from the env file. If GapGPT was the default, the scripts select a configured AvalAI or Cloudflare engine, in that order, and use `none` if neither is configured.

### Environment variables

| Environment variable | Value |
| --- | --- |
| `APP_AI_CRAWLER_ENABLED` | Set to `true` to enable AI engines. |
| `APP_AI_CRAWLER_GAPGPT_ENDPOINT` | Defaults to `https://api.gapgpt.ir/v1`. Use the base URL ending in `/v1`. The SDK appends `/chat/completions`. |
| `APP_AI_CRAWLER_GAPGPT_MODEL` | Defaults to `gapgpt-deepseek-v4.1-flash`. Use a model ID available to your GapGPT key that supports Chat Completions and the configured JSON response format. |
| `APP_AI_CRAWLER_GAPGPT_API_KEY` | GapGPT API key. Supply it through the process environment or a private env file. |

For Docker Compose, put these variables in the project’s Git-ignored `.env` and recreate the extractor after editing it. For a direct `go run ./cmd/server`, export the variables before starting the server. The server reads `.env.development` before `.env` when `APP_ENV=development`.

```dotenv
APP_AI_CRAWLER_ENABLED=true
APP_AI_CRAWLER_GAPGPT_ENDPOINT=https://api.gapgpt.ir/v1
APP_AI_CRAWLER_GAPGPT_MODEL=gapgpt-deepseek-v4.1-flash
APP_AI_CRAWLER_GAPGPT_API_KEY=your-gapgpt-api-key
```

Restrict access to any env file that contains an API key. Rotate any key shared in chat before deploying.

## Select the engine

To use GapGPT for a specific site, set these keys under that site’s `fetch` object.

```json
{
  "engine": "ai-crawler-gapgpt",
  "ai_fetch": "http"
}
```

For all hosts without a site rule or active learned configuration, set `APP_DEFAULT_ENGINE=ai-crawler-gapgpt`. This also changes sites with `fetch.engine: default`. GapGPT supports `http` and `browser` page fetching. Sites using `ai_fetch: web_search` need an engine with a search model, such as AvalAI. Keep those sites on AvalAI explicitly or change their fetch mode before switching the default. Set `APP_DEFAULT_ENGINE=none` for a GapGPT-only instance that serves explicit site rules.

## Check the connection

From the deployment host, use the same endpoint, model and key as the service. This lists the available models and sends a small JSON-mode request without product data.

```bash
base_url="${APP_AI_CRAWLER_GAPGPT_ENDPOINT:-https://api.gapgpt.ir/v1}"
base_url="${base_url%/}"
model="${APP_AI_CRAWLER_GAPGPT_MODEL:-gapgpt-deepseek-v4.1-flash}"
key="${APP_AI_CRAWLER_GAPGPT_API_KEY:?Set APP_AI_CRAWLER_GAPGPT_API_KEY first}"

curl -fsS --max-time 30 "$base_url/models" \
  -H @<(printf 'Authorization: Bearer %s\n' "$key") | jq -r '.data[].id'

jq -n --arg model "$model" '{
  model: $model,
  messages: [{role: "user", content: "Reply with the JSON object {\"ok\":true}."}],
  temperature: 0,
  max_tokens: 2000,
  response_format: {type: "json_object"}
}' | curl -fsS --max-time 60 "$base_url/chat/completions" \
  -H @<(printf 'Authorization: Bearer %s\n' "$key") \
  -H 'Content-Type: application/json' \
  --data-binary @- | jq -er '.choices[0].message.content | fromjson | .ok'
unset key
```

Then send a product URL to the extractor’s `POST /api/v1/extract` with your extractor `X-API-Key`. A successful AI extraction reports `source.engine: ai-crawler-gapgpt` and the configured model in `source.model`. Provider failures may include `evidence.ai.provider: gapgpt` and the provider’s request ID when it returns one.

## Run the tests

The end-to-end tests start the extractor with a local product page and a mock GapGPT API. They check default and per-site routing, the Chat Completions request, the extracted product and redacted authentication errors.

```bash
go test ./test/e2e -run '^TestGapGPT(Extraction|ProviderFailure)$'
```

The live test starts the extractor with a synthetic product page and sends its cleaned content to the real GapGPT API. Export `APP_AI_CRAWLER_GAPGPT_API_KEY` first. The test honors the endpoint and model overrides in your environment, allows up to 90 seconds for a model call, and consumes API tokens when you opt in. The service uses the shared `APP_AI_CRAWLER_TIMEOUT` setting for model calls.

```bash
GAPGPT_LIVE_TEST=1 go test ./test/e2e -run '^TestGapGPTLive$' -count=1 -v
```
