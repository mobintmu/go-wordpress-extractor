package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-wordpress/internal/apikey"
)

// The ai-crawler-cloudflare engine, against a fake Cloudflare AI Gateway.
const (
	cfTestAccount = "0123456789abcdef0123456789abcdef"
	cfTestGateway = "gw-e2e"
	cfTestToken   = "cf-test-token-0123456789"
	cfTestModel   = "@cf/deepseek-ai/deepseek-v4.1-flash-1017"
	cfTestLogID   = "01JCFLOG0000000000000000"
)

// newFakeGateway answers like Cloudflare AI Gateway's REST endpoint
// (or its compat endpoint) and returns cf-aig-log-id on every answer.
func newFakeGateway(t *testing.T, endpoint string) *fakeLLM {
	f := newFakeLLM(t)
	f.path = "/client/v4/accounts/" + cfTestAccount + "/ai/v1/chat/completions"
	if endpoint == "compat" {
		f.path = "/v1/" + cfTestAccount + "/" + cfTestGateway + "/compat/chat/completions"
	}
	f.respHeaders = map[string]string{"cf-aig-log-id": cfTestLogID, "cf-aig-cache-status": "MISS"}
	return f
}

func (f *fakeLLM) gatewayBaseURL() string {
	return f.srv.URL + strings.TrimSuffix(f.path, "/chat/completions")
}

// cfEnv turns on both AI engines: AvalAI against llm and Cloudflare
// against gw. extra wins.
func cfEnv(llm, gw *fakeLLM, extra map[string]string) map[string]string {
	env := aiEnv(llm, map[string]string{
		"APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID":            cfTestAccount,
		"APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID":            cfTestGateway,
		"APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT":              "rest",
		"APP_AI_CRAWLER_CLOUDFLARE_BASE_URL":              gw.gatewayBaseURL(),
		"APP_AI_CRAWLER_CLOUDFLARE_MODEL":                 cfTestModel,
		"APP_AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT":      "none",
		"APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN":             cfTestToken,
		"APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE":        "",
		"APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY":      "",
		"APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY_FILE": "",
		"APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING":    "true",
		"APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG":           "false",
		"APP_AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS":       "0",
	})
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// serveBoth registers the page with both fakes, so either engine can answer.
func serveBoth(e *Env, llm, gw *fakeLLM, host string, p aiProduct) string {
	target := serveAI(e, llm, host, p)
	gw.mu.Lock()
	gw.products["/p/"+p.slug] = p.answer(e.Shop, host)
	gw.mu.Unlock()
	return target
}

func cfSiteConfig() map[string]any {
	c := aiSiteConfig()
	c["fetch"].(map[string]any)["engine"] = "ai-crawler-cloudflare"
	return c
}

func wantStr(t *testing.T, r *Result, want map[string]string) {
	t.Helper()
	for path, w := range want {
		if got := r.Str(path); got != w {
			t.Errorf("%s = %q, want %q", path, got, w)
		}
	}
}

// 1. APP_DEFAULT_ENGINE=ai-crawler-cloudflare: an unknown host is served
// through the gateway, with the REST endpoint's headers.
func TestCloudflareDefaultEngine(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	e := Start(t, Options{Env: cfEnv(llm, gw, map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare"})})
	target := serveBoth(e, llm, gw, hostAI, oakChair)

	r := e.Extract(target)
	r.Success(t)
	wantStr(t, r, map[string]string{
		"site_id":        "ai-crawler-default",
		"config_version": "builtin-1+cloudflare",
		"config_origin":  "default",
		"source.engine":  "ai-crawler-cloudflare",
		"source.model":   cfTestModel,
		"product.title":  "Oak Chair",
	})
	if !hasWarning(r, "AI_EXTRACTED") {
		t.Errorf("want warning AI_EXTRACTED: %s", r.Raw)
	}
	if n := len(llm.Calls()); n != 0 {
		t.Errorf("AvalAI called %d times; the default engine is Cloudflare", n)
	}
	calls := gw.Calls()
	if len(calls) != 1 {
		t.Fatalf("gateway calls = %d, want 1", len(calls))
	}
	c := calls[0]
	for h, want := range map[string]string{
		"Authorization":        "Bearer " + cfTestToken,
		"Cf-Aig-Gateway-Id":    cfTestGateway,
		"Cf-Aig-Skip-Cache":    "true",
		"Cf-Aig-No-Wholesale":  "true",
		"Cf-Aig-Collect-Log":   "false",
		"Cf-Aig-Authorization": "",
	} {
		if got := c.Header.Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(c.Header.Get("cf-aig-metadata")), &meta); err != nil || meta["host"] != hostAI || meta["purpose"] != "extract" || meta["prompt_version"] != "1" {
		t.Errorf("cf-aig-metadata = %q", c.Header.Get("cf-aig-metadata"))
	}
	if c.Model != cfTestModel || c.ReasoningEffort != "none" || c.ResponseFormat != "json_object" {
		t.Errorf("model %q, reasoning_effort %q, response_format %q", c.Model, c.ReasoningEffort, c.ResponseFormat)
	}
	if strings.Contains(string(r.Raw), cfTestToken) {
		t.Error("the token appears in the response")
	}
}

// 2. The default stays ai-crawler-avalai: nothing reaches the gateway.
func TestCloudflareConfiguredButNotDefault(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	e := Start(t, Options{Env: cfEnv(llm, gw, nil)})
	r := e.Extract(serveBoth(e, llm, gw, hostAI, oakChair))
	r.Success(t)
	wantStr(t, r, map[string]string{"source.engine": "ai-crawler-avalai", "config_version": "builtin-1", "source.model": aiTestModel})
	if n := len(gw.Calls()); n != 0 {
		t.Errorf("gateway called %d times; the default engine is AvalAI", n)
	}
}

// 3. A site config selects Cloudflare while the default is AvalAI: each
// engine serves where it is configured, and the sites list shows both.
func TestCloudflarePerSiteEngine(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	e := Start(t, Options{Env: cfEnv(llm, gw, nil), ExtraSites: map[string]map[string]any{hostAISite: cfSiteConfig()}})

	r := e.Extract(serveBoth(e, llm, gw, hostAISite, oakChair))
	r.Success(t)
	wantStr(t, r, map[string]string{"site_id": "aisite", "config_origin": "config", "source.engine": "ai-crawler-cloudflare", "source.model": cfTestModel})

	r = e.Extract(serveBoth(e, llm, gw, hostAI, oakChair.withSlug("teak-table", "Teak Table")))
	r.Success(t)
	wantStr(t, r, map[string]string{"site_id": "ai-crawler-default", "source.engine": "ai-crawler-avalai"})

	if a, g := len(llm.Calls()), len(gw.Calls()); a != 1 || g != 1 {
		t.Errorf("AvalAI calls = %d, gateway calls = %d; want 1 each", a, g)
	}
	_, byID := e.listSites(t, e.Key)
	if s := byID["aisite"]; s == nil || s["engine"] != "ai-crawler-cloudflare" || s["model"] != cfTestModel {
		t.Errorf("aisite in the sites list = %v", s)
	}
}

// 4. The server does not start when APP_DEFAULT_ENGINE is wrong, or when
// the replaced APP_AI_CRAWLER_DEFAULT_RULE is still set.
func TestDefaultEngineStartupChecks(t *testing.T) {
	keys := filepath.Join(t.TempDir(), "api_keys.json")
	must(t, os.WriteFile(keys, []byte("[]"), 0o600))
	base := map[string]string{
		"APP_ENV": "test", "APP_HTTP_PORT": "1", "APP_HTTP_ADDRESS": "127.0.0.1", "APP_API_KEY_PEPPER": testPepper,
		"APP_SITE_CONFIG_DIR": filepath.Join(repoRoot, "configs"), "APP_API_KEYS_FILE": keys,
		"APP_AI_CRAWLER_ENABLED": "true", "APP_AI_CRAWLER_API_KEY": aiTestKey, "APP_AI_CRAWLER_API_KEY_FILE": "",
		"APP_AI_CRAWLER_BASE_URL": "https://api.avalai.ir/v1", "APP_AI_CRAWLER_DEFAULT_RULE": "",
		"APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID": "", "APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN": "", "APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE": "",
		"APP_AI_CRAWLER_CLOUDFLARE_BASE_URL": "",
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unconfigured engine", map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare"}, "APP_DEFAULT_ENGINE is ai-crawler-cloudflare, but that engine is not configured"},
		{"unknown engine", map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-openai"}, "invalid APP_DEFAULT_ENGINE"},
		{"needs selectors", map[string]string{"APP_DEFAULT_ENGINE": "http"}, "APP_DEFAULT_ENGINE \"http\" cannot serve websites without a rule"},
		{"legacy variable", map[string]string{"APP_DEFAULT_ENGINE": "", "APP_AI_CRAWLER_DEFAULT_RULE": "false"}, "APP_AI_CRAWLER_DEFAULT_RULE was replaced by APP_DEFAULT_ENGINE"},
		{"AI off", map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-avalai", "APP_AI_CRAWLER_ENABLED": "false"}, "APP_DEFAULT_ENGINE is ai-crawler-avalai, but APP_AI_CRAWLER_ENABLED is false"},
		{"token missing", map[string]string{"APP_DEFAULT_ENGINE": "", "APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID": cfTestAccount}, "the token is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range base {
				t.Setenv(k, v)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if err := newAppForStartupTest().Err(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("startup error = %v, want %q", err, tc.want)
			}
		})
	}
}

// 4b. APP_DEFAULT_ENGINE=none with only Cloudflare configured: unknown
// hosts get SITE_NOT_CONFIGURED, configured sites still work, nothing is
// learned.
func TestCloudflareDefaultEngineNone(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	e := Start(t, Options{
		Env: cfEnv(llm, gw, map[string]string{"APP_DEFAULT_ENGINE": "none", "APP_AI_CRAWLER_API_KEY": "",
			"APP_AI_CRAWLER_LEARN_SELECTORS": "true"}),
		ExtraSites: map[string]map[string]any{hostAISite: cfSiteConfig()},
	})
	e.Extract(serveBoth(e, llm, gw, hostAI, oakChair)).Failed(t, "SITE_NOT_CONFIGURED", "config", "site_config", false)
	r := e.Extract(serveBoth(e, llm, gw, hostAISite, oakChair))
	r.Success(t)
	wantStr(t, r, map[string]string{"source.engine": "ai-crawler-cloudflare"})
	s, byID := e.listSites(t, e.Key)
	if s.At("accepts_any_domain") != false || s.Has("default_ai_engine") {
		t.Errorf("accepts_any_domain = %v, default_ai_engine = %v", s.At("accepts_any_domain"), s.At("default_ai_engine"))
	}
	if _, ok := byID["ai-crawler-default"]; ok {
		t.Error("ai-crawler-default is listed with APP_DEFAULT_ENGINE=none")
	}
	if n := gw.count(true); n != 0 {
		t.Errorf("learning calls = %d with APP_DEFAULT_ENGINE=none", n)
	}
}

// 5. A site config naming Cloudflare on an instance where it is not
// configured gets ENGINE_UNAVAILABLE.
func TestCloudflareSiteEngineUnavailable(t *testing.T) {
	llm := newFakeLLM(t)
	e := Start(t, Options{Env: aiEnv(llm, nil), ExtraSites: map[string]map[string]any{hostAISite: cfSiteConfig()}})
	r := e.Extract(serveAI(e, llm, hostAISite, oakChair))
	r.Failed(t, "ENGINE_UNAVAILABLE", "fetch", "service", true)
	if !strings.Contains(r.Str("error.root_cause"), "ai-crawler-cloudflare") {
		t.Errorf("root_cause = %q", r.Str("error.root_cause"))
	}
	if n := len(llm.Calls()); n != 0 {
		t.Errorf("AvalAI called %d times for a Cloudflare site", n)
	}
}

// 6 and 8. Gateway errors map to the AI_* codes, with cf-aig-log-id as the
// provider request id. With NO_UNIFIED_BILLING, 400 / 2044 is an auth
// failure whose suggested action names BYOK.
func TestCloudflareErrors(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	cfError := func(w http.ResponseWriter, status, code int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		b, _ := json.Marshal(map[string]any{"success": false, "errors": []any{map[string]any{"code": code, "message": msg}}})
		w.Write(b)
	}
	gw.respond = func(w http.ResponseWriter, c llmCall, n int) {
		switch c.PageURL[strings.LastIndex(c.PageURL, "/")+1:] {
		case "auth":
			cfError(w, 401, 2009, "Unauthorized")
		case "byok":
			cfError(w, 400, 2044, "Provider credentials required")
		case "credits":
			cfError(w, 402, 2052, "Insufficient balance")
		case "rate":
			cfError(w, 429, 2018, "Rate limited")
		case "model":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"Model not found"}`))
		case "big":
			cfError(w, 400, 7003, "Run input is too large")
		case "down":
			cfError(w, 503, 2005, "Upstream error")
		}
	}
	e := Start(t, Options{Env: cfEnv(llm, gw, map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare"})})
	for _, tc := range []struct {
		slug, code, provider string
		status               int
		retryable            bool
	}{
		{"auth", "AI_AUTH_FAILED", "2009", 401, false},
		{"byok", "AI_AUTH_FAILED", "2044", 400, false},
		{"credits", "AI_QUOTA_EXCEEDED", "2052", 402, false},
		{"rate", "AI_RATE_LIMITED", "2018", 429, true},
		{"model", "AI_MODEL_UNAVAILABLE", "", 404, false},
		{"big", "AI_INPUT_TOO_LARGE", "7003", 400, false},
		{"down", "AI_UNAVAILABLE", "2005", 503, true},
	} {
		t.Run(tc.slug, func(t *testing.T) {
			e.Shop.Handle("/p/"+tc.slug, oakChair.html(e.Shop, hostAI))
			r := e.Extract(e.Shop.URL(hostAI, "/p/"+tc.slug))
			r.Failed(t, tc.code, "extract", "service", tc.retryable)
			wantStr(t, r, map[string]string{
				"source.engine":                   "ai-crawler-cloudflare",
				"evidence.ai.provider":            "cloudflare",
				"evidence.ai.provider_request_id": cfTestLogID,
				"evidence.ai.provider_code":       tc.provider,
			})
			if got := r.Int("evidence.ai.http_status"); int(got) != tc.status {
				t.Errorf("evidence.ai.http_status = %d, want %d", got, tc.status)
			}
			if tc.slug == "byok" && !strings.Contains(r.Str("error.suggested_action"), "BYOK") {
				t.Errorf("suggested_action = %q, want BYOK named", r.Str("error.suggested_action"))
			}
			if strings.Contains(string(r.Raw), cfTestToken) {
				t.Error("the token appears in the response")
			}
		})
	}
}

// 7. The compat endpoint: the gateway is in the path, the gateway token in
// cf-aig-authorization, and Authorization carries no Cloudflare token.
func TestCloudflareCompatEndpoint(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "compat")
	e := Start(t, Options{Env: cfEnv(llm, gw, map[string]string{
		"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare", "APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT": "compat",
		"APP_AI_CRAWLER_CLOUDFLARE_MODEL": "workers-ai/" + cfTestModel, "APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY": "sk-provider-e2e",
	})})
	e.Extract(serveBoth(e, llm, gw, hostAI, oakChair)).Success(t)
	calls := gw.Calls()
	if len(calls) != 1 {
		t.Fatalf("gateway calls = %d, want 1 on %s", len(calls), gw.path)
	}
	for h, want := range map[string]string{
		"Authorization":        "Bearer sk-provider-e2e",
		"Cf-Aig-Authorization": "Bearer " + cfTestToken,
		"Cf-Aig-Gateway-Id":    "",
	} {
		if got := calls[0].Header.Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
	if calls[0].Model != "workers-ai/"+cfTestModel {
		t.Errorf("model = %q", calls[0].Model)
	}
}

// 9. A dynamic route: the model that answered (cf-aig-model) is reported.
func TestCloudflareDynamicRoute(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	gw.respHeaders["cf-aig-model"] = "deepseek-v4.1-flash-1017"
	gw.respHeaders["cf-aig-provider"] = "workers-ai"
	e := Start(t, Options{Env: cfEnv(llm, gw, map[string]string{
		"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare", "APP_AI_CRAWLER_CLOUDFLARE_MODEL": "dynamic/extract",
	})})
	r := e.Extract(serveBoth(e, llm, gw, hostAI, oakChair))
	r.Success(t)
	wantStr(t, r, map[string]string{"source.model": "deepseek-v4.1-flash-1017"})
	if gw.Calls()[0].Model != "dynamic/extract" {
		t.Errorf("model sent = %q, want the route", gw.Calls()[0].Model)
	}
}

// 10. /metrics counts per engine.
func TestCloudflareMetricsPerEngine(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	e := Start(t, Options{Env: cfEnv(llm, gw, nil), ExtraSites: map[string]map[string]any{hostAISite: cfSiteConfig()}})
	e.Extract(serveBoth(e, llm, gw, hostAI, oakChair)).Success(t)
	e.Extract(serveBoth(e, llm, gw, hostAISite, oakChair)).Success(t)

	key := map[string]string{apikey.HeaderName: e.Key}
	r := e.Get("/metrics", key)
	byEngine := map[string]string{}
	for i := 0; r.At(fmt.Sprintf("ai.series.%d", i)) != nil; i++ {
		p := fmt.Sprintf("ai.series.%d.", i)
		byEngine[r.Str(p+"engine")] = r.Str(p+"model") + "@" + r.Str(p+"host")
	}
	if want := map[string]string{"ai-crawler-avalai": aiTestModel + "@" + hostAI, "ai-crawler-cloudflare": cfTestModel + "@" + hostAISite}; fmt.Sprint(byEngine) != fmt.Sprint(want) {
		t.Errorf("series by engine = %v, want %v", byEngine, want)
	}
	if got := r.Int("ai.calls"); got != 2 {
		t.Errorf("ai.calls = %d, want 2", got)
	}
	p := e.Get("/metrics?format=prometheus", key)
	want := fmt.Sprintf(`extract_ai_calls_total{engine="ai-crawler-cloudflare",model=%q,purpose="extract",host=%q,outcome="ok"} 1`, cfTestModel, hostAISite)
	if !strings.Contains(string(p.Raw), want) {
		t.Errorf("prometheus text lacks %q\n%s", want, p.Raw)
	}
}

// 11. Learning with Cloudflare as the default: the learning call goes
// through the gateway and the learned config records the engine.
func TestCloudflareLearnsSelectors(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	gw.selectors = selectorsAnswer
	e := Start(t, Options{Env: cfEnv(llm, gw, map[string]string{
		"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare", "APP_AI_CRAWLER_LEARN_SELECTORS": "true",
	})})
	e.Extract(serveBoth(e, llm, gw, hostAI, oakChair)).Success(t)
	l := waitLearned(t, e, hostAI, "candidate", status("candidate"))
	if l["engine"] != "ai-crawler-cloudflare" || l["model"] != cfTestModel {
		t.Errorf("learned.engine = %v, learned.model = %v", l["engine"], l["model"])
	}
	if n := gw.count(true); n != 1 {
		t.Errorf("gateway learning calls = %d, want 1", n)
	}
	if n := len(llm.Calls()); n != 0 {
		t.Errorf("AvalAI called %d times", n)
	}
	for _, c := range gw.Calls() {
		if c.Learn && !strings.Contains(c.Header.Get("cf-aig-metadata"), `"purpose":"learn"`) {
			t.Errorf("learning call cf-aig-metadata = %q", c.Header.Get("cf-aig-metadata"))
		}
	}
	raw, err := os.ReadFile(learnedPath(e, hostAI))
	must(t, err)
	if !strings.Contains(string(raw), "Learned by ai-crawler-cloudflare") {
		t.Errorf("$comment does not name the engine:\n%.300s", raw)
	}
}

// 12. GET /api/v1/sites shows the default engine and its model.
func TestCloudflareSitesListShowsDefaultEngine(t *testing.T) {
	llm, gw := newFakeLLM(t), newFakeGateway(t, "rest")
	e := Start(t, Options{Env: cfEnv(llm, gw, map[string]string{"APP_DEFAULT_ENGINE": "ai-crawler-cloudflare"})})
	s, byID := e.listSites(t, e.Key)
	if s.At("accepts_any_domain") != true || s.Str("default_ai_engine") != "ai-crawler-cloudflare" {
		t.Errorf("accepts_any_domain = %v, default_ai_engine = %q", s.At("accepts_any_domain"), s.Str("default_ai_engine"))
	}
	d := byID["ai-crawler-default"]
	if d == nil || d["engine"] != "ai-crawler-cloudflare" || d["model"] != cfTestModel || d["version"] != "builtin-1+cloudflare" {
		t.Errorf("default entry = %v", d)
	}
}
