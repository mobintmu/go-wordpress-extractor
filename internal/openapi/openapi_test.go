package openapi

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-wordpress/docs"
)

func build(t *testing.T) map[string]any {
	t.Helper()
	raw, err := FromSwagger2([]byte(docs.SwaggerInfo.ReadDoc()))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func obj(v any) map[string]any { x, _ := v.(map[string]any); return x }

func TestFromSwagger2IsOpenAPI3(t *testing.T) {
	doc := build(t)
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
	if _, ok := doc["swagger"]; ok {
		t.Error("converted doc still has a swagger field")
	}
	servers, _ := doc["servers"].([]any)
	if len(servers) != 1 || obj(servers[0])["url"] != "/" {
		t.Errorf("servers = %v, want [{url: /}]", doc["servers"])
	}
	schemes := obj(obj(doc["components"])["securitySchemes"])
	if obj(schemes["ApiKeyAuth"])["name"] != "X-API-Key" {
		t.Errorf("ApiKeyAuth = %v", schemes["ApiKeyAuth"])
	}
}

func TestExtractExamples(t *testing.T) {
	media := obj(obj(obj(obj(obj(obj(build(t)["paths"])[ExtractPath])["post"])["requestBody"])["content"])["application/json"])
	if _, ok := media["example"]; ok {
		t.Error("a single example would override the Examples dropdown")
	}
	got := obj(media["examples"])
	if len(got) != len(ExtractExamples) {
		t.Fatalf("%d examples in doc, want %d", len(got), len(ExtractExamples))
	}

	sites, err := filepath.Glob(filepath.Join("..", "..", "configs", "sites", "*.json"))
	if err != nil || len(sites) == 0 {
		t.Fatalf("no site configs: %v", err)
	}
	if len(ExtractExamples) != len(sites) {
		t.Errorf("%d examples for %d site configs; add one example per site", len(ExtractExamples), len(sites))
	}

	seen := map[string]bool{}
	for i, ex := range ExtractExamples {
		e := obj(got[ExampleKey(i)])
		if e == nil {
			t.Errorf("example %s missing", ExampleKey(i))
			continue
		}
		if e["summary"] == "" || e["summary"] == nil {
			t.Errorf("%s has no summary", ex.Key)
		}
		v := obj(e["value"])
		if v["url"] != ex.URL || v["request_id"] != ex.RequestID {
			t.Errorf("%s value = %v", ex.Key, v)
		}
		if len(ex.RequestID) > 128 {
			t.Errorf("%s request_id is over 128 chars", ex.Key)
		}
		u, err := url.Parse(ex.URL)
		if err != nil || u.Scheme != "https" {
			t.Errorf("%s url %q is not an https URL", ex.Key, ex.URL)
			continue
		}
		domain := strings.TrimPrefix(u.Hostname(), "www.")
		if _, err := os.Stat(filepath.Join("..", "..", "configs", "sites", domain+".json")); err != nil {
			t.Errorf("%s: no site config for %s", ex.Key, domain)
		}
		if seen[domain] {
			t.Errorf("two examples for %s", domain)
		}
		seen[domain] = true
	}
}
