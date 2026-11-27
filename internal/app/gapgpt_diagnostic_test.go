package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-wordpress/internal/aicrawler"
)

func TestGapGPTDiagnosticTrace(t *testing.T) {
	const key = "diagnostic-secret-key"
	const reply = `{"ok":true}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("unexpected provider request")
		}
		var request struct {
			Model          string            `json:"model"`
			ResponseFormat map[string]string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil ||
			request.Model != "test-model" || request.ResponseFormat["type"] != "json_object" {
			t.Error("unexpected Chat Completions JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "upstream-request-42")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": key, // A provider may echo a secret in its response body.
			"choices": []any{map[string]any{
				"message": map[string]string{"content": reply}, "finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	var out bytes.Buffer
	trace := &gapgptTrace{out: &out, key: key}
	client := aicrawler.New(aicrawler.GapGPT(server.URL+"/v1", "test-model", key),
		aicrawler.Settings{Middleware: trace.exchange, MaxOutputTokens: 2000})
	result, callErr := client.Complete(context.Background(),
		[]aicrawler.Message{{Role: "user", Content: "Return an object"}}, "diagnostic", nil)
	if callErr != nil || result.Content != reply {
		t.Fatalf("SDK could not read the traced response: %v", callErr)
	}
	for _, want := range []string{
		"POST " + server.URL + "/v1/chat/completions", "Authorization: [redacted]",
		`"response_format":{"type":"json_object"}`, "Return an object",
		"HTTP 200", "upstream-request-42", `"id":"[redacted]"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("trace missing %q", want)
		}
	}
	if strings.Contains(out.String(), key) {
		t.Error("trace contains the API key")
	}

	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = trace.exchange(req, func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	if !strings.Contains(out.String(), "no HTTP response") || !strings.Contains(out.String(), "context deadline exceeded") {
		t.Error("trace did not explain the missing HTTP response")
	}
}
