package aicrawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go-wordpress/internal/diagnose"
)

const (
	cfAccount = "0123456789abcdef0123456789abcdef"
	cfToken   = "cf-token-secret-0123456789"
	cfProvKey = "sk-provider-secret-0123456789"
)

// seen is one request the fake gateway received.
type seen struct {
	path    string
	headers http.Header
	body    map[string]any
}

// fakeGateway answers like Cloudflare AI Gateway. answer writes the
// response; nil answers with a valid completion.
func fakeGateway(t *testing.T, answer func(w http.ResponseWriter)) (*httptest.Server, func() []seen) {
	t.Helper()
	var mu sync.Mutex
	var calls []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		calls = append(calls, seen{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		mu.Unlock()
		if answer != nil {
			answer(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("cf-aig-log-id", "01JLOG")
		w.Header().Set("cf-aig-cache-status", "MISS")
		w.Header().Set("cf-aig-model", "deepseek-v4.1-flash-routed")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seen {
		mu.Lock()
		defer mu.Unlock()
		return append([]seen(nil), calls...)
	}
}

func testSettings() Settings { return Settings{MaxOutputTokens: 100, ResponseFormat: FormatJSONObject} }

func TestCloudflareBaseURLs(t *testing.T) {
	if got, want := CloudflareBaseURL(CloudflareREST, cfAccount, "gw"), "https://api.cloudflare.com/client/v4/accounts/"+cfAccount+"/ai/v1"; got != want {
		t.Errorf("rest = %q, want %q", got, want)
	}
	if got, want := CloudflareBaseURL(CloudflareCompat, cfAccount, "gw"), "https://gateway.ai.cloudflare.com/v1/"+cfAccount+"/gw/compat"; got != want {
		t.Errorf("compat = %q, want %q", got, want)
	}
	p := Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw", Endpoint: CloudflareREST, Model: "m", APIToken: cfToken})
	if p.BaseURL != CloudflareBaseURL(CloudflareREST, cfAccount, "gw") || p.Engine != EngineCloudflare || p.Name != ProviderCloudflare {
		t.Errorf("profile = %+v", p)
	}
}

func TestCloudflareRESTHeaders(t *testing.T) {
	srv, calls := fakeGateway(t, nil)
	c := New(Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw1", Endpoint: CloudflareREST, BaseURL: srv.URL + "/client/v4/accounts/" + cfAccount + "/ai/v1",
		Model: "@cf/deepseek-ai/deepseek-v4.1-flash-1017", ReasoningEffort: "none", APIToken: cfToken, NoUnifiedBilling: true}), testSettings())
	ctx := WithCall(context.Background(), PurposeExtract, "shop.test")
	comp, cerr := c.Complete(ctx, []Message{{Role: "user", Content: "hi"}}, "product", nil)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if comp.RequestID != "01JLOG" || comp.GatewayCache != "MISS" {
		t.Errorf("request id = %q, cache = %q", comp.RequestID, comp.GatewayCache)
	}
	if comp.Model != "" {
		t.Errorf("model = %q: cf-aig-model is only read for dynamic routes", comp.Model)
	}
	got := calls()[0]
	if want := "/client/v4/accounts/" + cfAccount + "/ai/v1/chat/completions"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	for h, want := range map[string]string{
		"Authorization":        "Bearer " + cfToken,
		"Cf-Aig-Gateway-Id":    "gw1",
		"Cf-Aig-Skip-Cache":    "true",
		"Cf-Aig-No-Wholesale":  "true",
		"Cf-Aig-Collect-Log":   "false",
		"Cf-Aig-Authorization": "",
	} {
		if v := got.headers.Get(h); v != want {
			t.Errorf("%s = %q, want %q", h, v, want)
		}
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(got.headers.Get("cf-aig-metadata")), &meta); err != nil {
		t.Fatalf("cf-aig-metadata = %q: %v", got.headers.Get("cf-aig-metadata"), err)
	}
	if meta["host"] != "shop.test" || meta["purpose"] != "extract" || meta["prompt_version"] != PromptVersion {
		t.Errorf("cf-aig-metadata = %v", meta)
	}
	if got.body["reasoning_effort"] != "none" || got.body["model"] != "@cf/deepseek-ai/deepseek-v4.1-flash-1017" {
		t.Errorf("body = %v", got.body)
	}
}

func TestCloudflareCompatHeaders(t *testing.T) {
	srv, calls := fakeGateway(t, nil)
	// Without a provider key: no Authorization header at all.
	c := New(Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw1", Endpoint: CloudflareCompat, BaseURL: srv.URL + "/v1/" + cfAccount + "/gw1/compat",
		Model: "dynamic/route", APIToken: cfToken, CollectLog: true}), testSettings())
	comp, cerr := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, "product", nil)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if comp.Model != "deepseek-v4.1-flash-routed" {
		t.Errorf("model = %q, want cf-aig-model for a dynamic route", comp.Model)
	}
	got := calls()[0]
	if want := "/v1/" + cfAccount + "/gw1/compat/chat/completions"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	for h, want := range map[string]string{
		"Authorization":        "",
		"Cf-Aig-Authorization": "Bearer " + cfToken,
		"Cf-Aig-Gateway-Id":    "",
		"Cf-Aig-No-Wholesale":  "",
		"Cf-Aig-Collect-Log":   "",
	} {
		if v := got.headers.Get(h); v != want {
			t.Errorf("%s = %q, want %q", h, v, want)
		}
	}
	if _, ok := got.body["reasoning_effort"]; ok {
		t.Errorf("reasoning_effort sent although not set: %v", got.body)
	}

	// With a provider key: it goes in Authorization.
	c = New(Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw1", Endpoint: CloudflareCompat, BaseURL: srv.URL + "/v1/" + cfAccount + "/gw1/compat",
		Model: "openai/gpt-5-mini", APIToken: cfToken, ProviderAPIKey: cfProvKey}), testSettings())
	if _, cerr := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, "product", nil); cerr != nil {
		t.Fatal(cerr)
	}
	if v := calls()[1].headers.Get("Authorization"); v != "Bearer "+cfProvKey {
		t.Errorf("Authorization = %q, want the provider key", v)
	}
}

func TestAvalAIProfileUnchanged(t *testing.T) {
	srv, calls := fakeGateway(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("avalai-request-id", "req_1")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}]}`))
	})
	c := New(AvalAI(srv.URL+"/v1", "deepseek-v4.1-flash", "sk-avalai", "sonar"), testSettings())
	comp, cerr := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, "product", nil)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if comp.RequestID != "req_1" || c.Engine() != EngineAvalAI || c.Provider() != ProviderAvalAI {
		t.Errorf("request id = %q, engine = %q", comp.RequestID, c.Engine())
	}
	got := calls()[0]
	for _, h := range []string{"cf-aig-metadata", "cf-aig-skip-cache", "cf-aig-gateway-id"} {
		if v := got.headers.Get(h); v != "" {
			t.Errorf("AvalAI call sends %s: %q", h, v)
		}
	}
	if _, ok := got.body["reasoning_effort"]; ok {
		t.Error("AvalAI call sends reasoning_effort")
	}
}

// Every row of the proposal's error table (section 2.6), with both body
// shapes: OpenAI's {"error": {...}} and Cloudflare's {"errors": [...]}.
func TestCloudflareErrorMapping(t *testing.T) {
	cfBody := func(code int, msg string) string {
		b, _ := json.Marshal(map[string]any{"success": false, "errors": []any{map[string]any{"code": code, "message": msg}}})
		return string(b)
	}
	oaBody := func(code, msg string) string {
		b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": msg, "type": "invalid_request_error"}})
		return string(b)
	}
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"2009 unauthorized", 401, cfBody(2009, "Unauthorized"), diagnose.AIAuthFailed},
		{"401 plain", 401, `{"error":"Invalid token"}`, diagnose.AIAuthFailed},
		{"10000 token lacks Workers AI", 401, cfBody(10000, "Authentication error"), diagnose.AIAuthFailed},
		{"2044 provider credentials required", 400, cfBody(2044, "Provider credentials required"), diagnose.AIAuthFailed},
		{"2049 unified billing unavailable", 403, cfBody(2049, "Unified Billing not available"), diagnose.AIAuthFailed},
		{"2042 gateway not enabled", 403, cfBody(2042, "Gateway not enabled"), diagnose.AIAuthFailed},
		{"2036 feature not enabled", 403, cfBody(2036, "Feature not enabled"), diagnose.AIAuthFailed},
		{"2052 no balance", 402, cfBody(2052, "Insufficient balance"), diagnose.AIQuotaExceeded},
		{"2021 byok payment", 402, cfBody(2021, "Payment error"), diagnose.AIQuotaExceeded},
		{"402 plain", 402, `{}`, diagnose.AIQuotaExceeded},
		{"2018 rate limit", 429, cfBody(2018, "Rate limited"), diagnose.AIRateLimited},
		{"429 provider", 429, oaBody("rate_limit_exceeded", "slow down"), diagnose.AIRateLimited},
		{"413", 413, `{}`, diagnose.AIInputTooLarge},
		{"7003 input too large", 400, cfBody(7003, "Run input is too large"), diagnose.AIInputTooLarge},
		{"context_length_exceeded", 400, oaBody("context_length_exceeded", "too long"), diagnose.AIInputTooLarge},
		{"404 model not found", 404, cfBody(0, "Model not found"), diagnose.AIModelUnavailable},
		{"403 model access denied", 403, `{"error":"Model access denied"}`, diagnose.AIModelUnavailable},
		{"451 region", 451, `{}`, diagnose.AIModelUnavailable},
		{"7000 no route", 400, cfBody(7000, "No route for that URI"), diagnose.AIModelUnavailable},
		{"404 dynamic route", 404, `{"error":"Dynamic route not found"}`, diagnose.AIModelUnavailable},
		{"500", 500, `{}`, diagnose.AIUnavailable},
		{"2002", 400, cfBody(2002, "Gateway error"), diagnose.AIUnavailable},
		{"2005", 400, cfBody(2005, "Provider error"), diagnose.AIUnavailable},
		{"7005", 400, cfBody(7005, "Upstream error"), diagnose.AIUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := fakeGateway(t, func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("cf-aig-log-id", "01JERR")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			c := New(Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw", Endpoint: CloudflareREST, BaseURL: srv.URL, Model: "m", APIToken: cfToken}), testSettings())
			_, cerr := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, "product", nil)
			if cerr == nil {
				t.Fatal("no error")
			}
			if cerr.Code != tc.want {
				t.Errorf("code = %s, want %s (%s)", cerr.Code, tc.want, cerr.Message)
			}
			if cerr.Status != tc.status || cerr.RequestID != "01JERR" {
				t.Errorf("status = %d, request id = %q", cerr.Status, cerr.RequestID)
			}
			if strings.Contains(cerr.Message, cfToken) {
				t.Error("the token appears in the error")
			}
		})
	}
}

func TestCloudflareErrorCodeFromBody(t *testing.T) {
	srv, _ := fakeGateway(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":2044,"message":"Provider credentials required"}]}`))
	})
	c := New(Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw", Endpoint: CloudflareREST, BaseURL: srv.URL, Model: "m", APIToken: cfToken}), testSettings())
	_, cerr := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, "product", nil)
	if cerr == nil || cerr.ProviderCode != "2044" || !strings.Contains(cerr.Message, "Provider credentials required") {
		t.Fatalf("error = %+v", cerr)
	}
}

// A transport error that echoes a secret header never shows the secret.
func TestSecretsRedacted(t *testing.T) {
	c := New(Cloudflare(CloudflareSettings{AccountID: cfAccount, GatewayID: "gw", Endpoint: CloudflareCompat, BaseURL: "http://127.0.0.1:1/x", Model: "m", APIToken: cfToken, ProviderAPIKey: cfProvKey}), testSettings())
	msg := c.redact("dial failed: Authorization: Bearer " + cfProvKey + " cf-aig-authorization: Bearer " + cfToken)
	if strings.Contains(msg, cfToken) || strings.Contains(msg, cfProvKey) {
		t.Errorf("secret not redacted: %s", msg)
	}
	_, cerr := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, "product", nil)
	if cerr == nil || cerr.Code != diagnose.AIUnavailable {
		t.Fatalf("error = %+v", cerr)
	}
	if strings.Contains(cerr.Message, cfToken) || strings.Contains(cerr.Message, cfProvKey) {
		t.Errorf("secret in error: %s", cerr.Message)
	}
}

func TestModelNotFoundSharedMapping(t *testing.T) {
	if got := sharedError(ProviderError{Status: 404, Code: "model_not_found"}); got != diagnose.AIModelUnavailable {
		t.Errorf("model_not_found = %s, want AI_MODEL_UNAVAILABLE", got)
	}
}
