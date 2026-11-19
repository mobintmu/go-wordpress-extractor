package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// fetch.ai_fetch "web_search": the site is never contacted. Only the URL
// goes to the provider's search model (APP_AI_CRAWLER_SEARCH_MODEL), which
// answers with the product and its sources, like Perplexity's sonar.

const webTestModel = "sonar-test"

func webSiteConfig() map[string]any {
	c := aiSiteConfig()
	c["fetch"].(map[string]any)["ai_fetch"] = "web_search"
	return c
}

// sonarAnswer writes a chat completion with Perplexity's extra fields.
func sonarAnswer(w http.ResponseWriter, content string, citations []string) {
	results := []any{}
	for _, u := range citations {
		results = append(results, map[string]any{"url": u, "title": "page", "snippet": "..."})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("avalai-request-id", "req_web_1")
	b, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-web", "object": "chat.completion", "created": 1763500000, "model": webTestModel,
		"choices": []any{map[string]any{"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": content}}},
		"usage":          map[string]any{"prompt_tokens": 70, "completion_tokens": 300, "total_tokens": 370},
		"citations":      citations,
		"search_results": results,
	})
	w.Write(b)
}

func webEnv(llm *fakeLLM, extra map[string]string) map[string]string {
	env := map[string]string{"APP_AI_CRAWLER_SEARCH_MODEL": webTestModel, "APP_AI_CRAWLER_CACHE_TTL": "1h"}
	for k, v := range extra {
		env[k] = v
	}
	return aiEnv(llm, env)
}

func TestAICrawlerWebSearch(t *testing.T) {
	llm := newFakeLLM(t)
	e := Start(t, Options{Env: webEnv(llm, nil), ExtraSites: map[string]map[string]any{hostAISite: webSiteConfig()}})
	target := e.Shop.URL(hostAISite, "/p/oak-chair")
	// The model's answer, with Perplexity-style citation markers in it.
	answer := strings.Replace(oakChair.answer(e.Shop, hostAISite), "finished by hand.", "finished by hand.[1][2]", 1)
	llm.respond = func(w http.ResponseWriter, c llmCall, n int) {
		sonarAnswer(w, answer, []string{target, "https://other-shop.example/oak-chair"})
	}

	r := e.Extract(target)
	r.Success(t)
	wantStr(t, r, map[string]string{
		"site_id":               "aisite",
		"source.engine":         "ai-crawler-avalai",
		"source.fetched_with":   "web_search",
		"source.model":          webTestModel,
		"source.prompt_version": "web-1",
		"source.sources.0":      target,
		"product.title":         "Oak Chair",
		"product.description":   oakChair.desc,
		"product.specs.0.value": "45 cm",
	})
	if r.At("source.site_contacted") != false {
		t.Errorf("source.site_contacted = %v, want false", r.At("source.site_contacted"))
	}
	if got := r.Int("product.price.current"); got != 12900 {
		t.Errorf("price.current = %d, want 12900", got)
	}
	if !hasWarning(r, "AI_UNVERIFIED") || !hasWarning(r, "AI_EXTRACTED") {
		t.Errorf("want warnings AI_UNVERIFIED and AI_EXTRACTED: %s", r.At("warnings"))
	}
	if got := unavailableReason(r, "availability"); got != "" && got != "NOT_IN_SEARCH_RESULTS" {
		t.Errorf("unavailable reason = %q, want NOT_IN_SEARCH_RESULTS", got)
	}
	if n := e.Shop.Hits("/p/oak-chair"); n != 0 {
		t.Errorf("the shop was contacted %d times; web_search must never fetch the site", n)
	}

	calls := llm.Calls()
	if len(calls) != 1 {
		t.Fatalf("LLM calls = %d, want 1", len(calls))
	}
	c := calls[0]
	if c.Model != webTestModel || c.ResponseFormat != "" || c.PageURL != target {
		t.Errorf("model %q, response_format %q, url %q", c.Model, c.ResponseFormat, c.PageURL)
	}
	if !strings.Contains(c.System, "web search") || strings.Contains(c.User, "<<<PAGE") {
		t.Errorf("web search prompt expected, got system %.80q / user %.120q", c.System, c.User)
	}

	// The same URL is served from the cache.
	r2 := e.Extract(target)
	r2.Success(t)
	if r2.At("source.cached") != true || len(llm.Calls()) != 1 || r2.Str("source.sources.0") != target {
		t.Errorf("second call: cached %v, LLM calls %d, sources %v", r2.At("source.cached"), len(llm.Calls()), r2.At("source.sources"))
	}
}

// An answer whose sources are all on other sites is not used.
func TestAICrawlerWebSearchNeedsSiteSource(t *testing.T) {
	llm := newFakeLLM(t)
	e := Start(t, Options{Env: webEnv(llm, nil), ExtraSites: map[string]map[string]any{hostAISite: webSiteConfig()}})
	target := e.Shop.URL(hostAISite, "/p/oak-chair")
	llm.respond = func(w http.ResponseWriter, c llmCall, n int) {
		sonarAnswer(w, oakChair.answer(e.Shop, hostAISite), []string{"https://other-shop.example/oak-chair"})
	}
	r := e.Extract(target)
	r.Failed(t, "SEARCH_NO_RESULT", "fetch", "source_site", false)
	if !strings.Contains(r.Str("error.root_cause"), "other-shop.example") {
		t.Errorf("root_cause = %q", r.Str("error.root_cause"))
	}
}

// Without a search model, a web_search site gets ENGINE_UNAVAILABLE.
func TestAICrawlerWebSearchWithoutModel(t *testing.T) {
	llm := newFakeLLM(t)
	e := Start(t, Options{Env: webEnv(llm, map[string]string{"APP_AI_CRAWLER_SEARCH_MODEL": "none"}),
		ExtraSites: map[string]map[string]any{hostAISite: webSiteConfig()}})
	r := e.Extract(e.Shop.URL(hostAISite, "/p/oak-chair"))
	r.Failed(t, "ENGINE_UNAVAILABLE", "fetch", "service", true)
	if n := len(llm.Calls()); n != 0 {
		t.Errorf("LLM called %d times", n)
	}
}
