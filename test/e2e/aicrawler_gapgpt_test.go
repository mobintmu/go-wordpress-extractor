package e2e

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go-wordpress/internal/config"
)

func gapgptEnv(endpoint, model, key string) map[string]string {
	return map[string]string{
		"APP_DEFAULT_ENGINE":                   config.EngineGapGPT,
		"APP_AI_CRAWLER_ENABLED":               "true",
		"APP_AI_CRAWLER_API_KEY":               "",
		"APP_AI_CRAWLER_API_KEY_FILE":          "",
		"APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID": "",
		"APP_AI_CRAWLER_GAPGPT_API_KEY":        key,
		"APP_AI_CRAWLER_GAPGPT_ENDPOINT":       endpoint,
		"APP_AI_CRAWLER_GAPGPT_MODEL":          model,
		"APP_AI_CRAWLER_DEFAULT_DELAY_MS":      "0",
		"APP_AI_CRAWLER_LEARN_SELECTORS":       "false",
		"APP_AI_CRAWLER_RESPONSE_FORMAT":       "json_object",
		"APP_AI_CRAWLER_RETRIES":               "0",
	}
}

func TestGapGPTExtraction(t *testing.T) {
	const key = "test-gapgpt-key"
	const engine = config.EngineGapGPT
	const model = "test-gapgpt-model"

	for _, mode := range []string{"default", "site"} {
		t.Run(mode, func(t *testing.T) {
			llm := newFakeLLM(t)
			env := gapgptEnv(llm.baseURL()+"/", model, key)
			opts := Options{Env: env}
			host := hostAI
			if mode == "site" {
				env["APP_DEFAULT_ENGINE"] = "none"
				host = hostAISite
				site := aiSiteConfig()
				site["fetch"].(map[string]any)["engine"] = engine
				opts.ExtraSites = map[string]map[string]any{host: site}
			}
			e := Start(t, opts)
			r := e.Extract(serveAI(e, llm, host, oakChair))
			r.Success(t)
			wantStr(t, r, map[string]string{
				"source.engine":       engine,
				"source.model":        model,
				"source.fetched_with": "http",
				"product.title":       "Oak Chair",
			})
			if got := r.Int("product.price.current"); got != 12900 {
				t.Errorf("current price = %d, want 12900 cents", got)
			}
			if !hasWarning(r, "AI_EXTRACTED") {
				t.Error("missing AI_EXTRACTED warning")
			}
			if mode == "default" {
				wantStr(t, r, map[string]string{
					"config_origin": "default", "config_version": "builtin-1+gapgpt",
				})
			} else {
				wantStr(t, r, map[string]string{"config_origin": "config", "site_id": "aisite"})
			}
			calls := llm.Calls()
			if len(calls) != 1 {
				t.Fatalf("GapGPT calls = %d, want 1", len(calls))
			}
			if calls[0].Auth != "Bearer "+key || calls[0].Model != model || calls[0].ResponseFormat != "json_object" {
				t.Error("unexpected authentication, model or response format in GapGPT request")
			}
			if !strings.Contains(calls[0].User, oakChair.name) || strings.Contains(calls[0].User, "SECRET_TRACKING_ID") {
				t.Error("GapGPT request should contain product text with the tracking script stripped")
			}
		})
	}
}

func TestGapGPTProviderFailure(t *testing.T) {
	const key = "test-gapgpt-rejected-key"
	const requestID = "req-gapgpt-auth"
	llm := newFakeLLM(t)
	llm.respond = func(w http.ResponseWriter, _ llmCall, _ int) {
		chatError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key: "+key,
			map[string]string{"X-Request-Id": requestID})
	}
	e := Start(t, Options{Env: gapgptEnv(llm.baseURL(), config.DefaultGapGPTModel, key)})
	r := e.Extract(serveAI(e, llm, hostAI, oakChair))
	if strings.Contains(string(r.Raw), key) {
		t.Fatal("GapGPT credential appeared in the extractor response")
	}
	r.Failed(t, "AI_AUTH_FAILED", "extract", "service", false)
	wantStr(t, r, map[string]string{
		"evidence.ai.provider":            "gapgpt",
		"evidence.ai.provider_code":       "invalid_api_key",
		"evidence.ai.provider_request_id": requestID,
	})
	if got := r.Int("evidence.ai.http_status"); got != http.StatusUnauthorized {
		t.Errorf("provider status = %d, want %d", got, http.StatusUnauthorized)
	}
	if !strings.Contains(r.Str("error.suggested_action"), "APP_AI_CRAWLER_GAPGPT_API_KEY") {
		t.Error("the suggested action should name the GapGPT credential setting")
	}
}

func TestGapGPTLive(t *testing.T) {
	if os.Getenv("GAPGPT_LIVE_TEST") != "1" {
		t.Skip("set GAPGPT_LIVE_TEST=1 to run extraction against GapGPT")
	}
	key := strings.TrimSpace(os.Getenv("APP_AI_CRAWLER_GAPGPT_API_KEY"))
	if key == "" {
		t.Fatal("set APP_AI_CRAWLER_GAPGPT_API_KEY for the live test")
	}
	env := gapgptEnv(os.Getenv("APP_AI_CRAWLER_GAPGPT_ENDPOINT"), os.Getenv("APP_AI_CRAWLER_GAPGPT_MODEL"), key)
	wantModel := strings.TrimSpace(env["APP_AI_CRAWLER_GAPGPT_MODEL"])
	if wantModel == "" {
		wantModel = config.DefaultGapGPTModel
	}
	env["APP_AI_CRAWLER_TIMEOUT"] = "90s"
	env["APP_MAX_REQUEST_DURATION"] = "100s"
	e := Start(t, Options{Env: env})
	e.Client.Timeout = 110 * time.Second

	path := "/p/" + oakChair.slug
	e.Shop.Handle(path, oakChair.html(e.Shop, hostAI))
	r := e.Extract(e.Shop.URL(hostAI, path))
	if strings.Contains(string(r.Raw), key) {
		t.Fatal("GapGPT credential appeared in the extractor response")
	}
	r.Success(t)
	wantStr(t, r, map[string]string{
		"source.engine":          config.EngineGapGPT,
		"source.fetched_with":    "http",
		"source.model":           wantModel,
		"product.title":          oakChair.name,
		"product.price.currency": "USD",
	})
	if got := r.Int("product.price.current"); got != 12900 {
		t.Errorf("current price = %d, want 12900 cents", got)
	}
}
