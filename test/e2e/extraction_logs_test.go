package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"go-wordpress/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestExtractionLogs(t *testing.T) {
	const providerKey = "private-provider-key"
	const queryToken = "private-product-query"
	const providerMessage = "private-provider-error-body"
	const requestID = "logged-product"
	const failedID = "logged-provider-failure"

	core, logs := observer.New(zap.InfoLevel)
	llm := newFakeLLM(t)
	llm.respond = func(w http.ResponseWriter, call llmCall, _ int) {
		if strings.Contains(call.PageURL, "/p/rate-limited") {
			chatError(w, http.StatusTooManyRequests, "rate_limit_exceeded", providerMessage,
				map[string]string{"X-Request-Id": "provider-request-42"})
			return
		}
		before := logs.FilterField(zap.String("request_id", requestID)).All()
		if len(before) != 1 || before[0].Message != "extraction started" {
			t.Error("expected the extraction start to be logged before the provider answers")
		}
		chatOK(w, llm.productFor(call.PageURL), "stop")
	}
	e := Start(t, Options{Env: gapgptEnv(llm.baseURL(), config.DefaultGapGPTModel, providerKey), Log: zap.New(core)})

	target := serveAI(e, llm, hostAI, oakChair)
	success := e.Post("/api/v1/extract", e.Key, map[string]string{
		"request_id": requestID, "url": target + "?token=" + queryToken,
	})
	success.Success(t)
	failed := e.Post("/api/v1/extract", e.Key, map[string]string{
		"request_id": failedID, "url": serveAI(e, llm, hostAI, oakChair.withSlug("rate-limited", oakChair.name)),
	})
	failed.Failed(t, "AI_RATE_LIMITED", "extract", "service", true)
	e.Post("/api/v1/extract", e.Key, map[string]string{"url": "ftp://shop.test/product"}).
		APIError(t, http.StatusBadRequest, "INVALID_URL")

	for _, tc := range []struct {
		id   string
		want map[string]any
	}{
		{requestID, map[string]any{
			"outcome": "success", "engine": config.EngineGapGPT, "model": config.DefaultGapGPTModel,
			"price_current": int64(12900), "price_currency": "USD", "input_tokens": int64(1000),
		}},
		{failedID, map[string]any{
			"outcome": "failed", "code": "AI_RATE_LIMITED", "stage": "extract", "retryable": true,
			"site_http_status": int64(http.StatusOK), "ai_http_status": int64(http.StatusTooManyRequests),
			"ai_provider": "gapgpt", "ai_request_id": "provider-request-42",
		}},
	} {
		entries := logs.FilterField(zap.String("request_id", tc.id)).All()
		if len(entries) != 2 || entries[0].Message != "extraction started" || entries[1].Message != "extraction finished" {
			t.Errorf("request %s: expected a start and finish log, got %d entries", tc.id, len(entries))
			continue
		}
		fields := entries[1].ContextMap()
		for key, want := range tc.want {
			if got := fields[key]; got != want {
				t.Errorf("request %s: %s = %v, want %v", tc.id, key, got, want)
			}
		}
		if fields["http_status"] != int64(http.StatusOK) {
			t.Errorf("request %s: extraction response should have HTTP status 200", tc.id)
		}
	}
	if rejected := logs.FilterMessage("extraction rejected").FilterField(zap.String("code", "INVALID_URL")); rejected.Len() != 1 {
		t.Error("expected a rejection log for the invalid URL")
	}
	for _, entry := range logs.FilterMessageSnippet("extraction ").All() {
		fields := fmt.Sprint(entry.ContextMap())
		for _, value := range []string{e.Key, queryToken, providerKey, providerMessage} {
			if strings.Contains(fields, value) {
				t.Fatal("extraction logs included a credential, URL query value or upstream error body")
			}
		}
	}
}
