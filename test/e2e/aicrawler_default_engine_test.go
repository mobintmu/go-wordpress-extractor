package e2e

import (
	"strings"
	"testing"
)

// defaultSiteConfig is aiSiteConfig with fetch.engine "default": the site
// follows APP_DEFAULT_ENGINE instead of naming an AI engine.
func defaultSiteConfig() map[string]any {
	cfg := aiSiteConfig()
	cfg["site_id"] = "follows-default"
	cfg["fetch"] = map[string]any{"engine": "default", "delay_ms": 0, "timeout_seconds": 5, "retries": 0}
	return cfg
}

// A site config with fetch.engine "default" is served with APP_DEFAULT_ENGINE,
// and the sites list shows the resolved engine and model.
func TestAICrawlerSiteFollowsDefaultEngine(t *testing.T) {
	llm := newFakeLLM(t)
	e := Start(t, Options{
		Env:        aiEnv(llm, nil),
		ExtraSites: map[string]map[string]any{hostAISite: defaultSiteConfig()},
	})
	r := e.Extract(serveAI(e, llm, hostAISite, oakChair))
	r.Success(t)
	wantStr(t, r, map[string]string{
		"site_id":       "follows-default",
		"config_origin": "config",
		"source.engine": "ai-crawler-avalai",
	})

	_, byID := e.listSites(t, e.Key)
	if s := byID["follows-default"]; s == nil || s["engine"] != "ai-crawler-avalai" || s["model"] != aiTestModel {
		t.Errorf("follows-default in the sites list = %v", s)
	}
}

// With APP_DEFAULT_ENGINE=none there is no AI engine to follow: the site is
// listed without an engine, and a request fails with ENGINE_UNAVAILABLE.
func TestAICrawlerSiteFollowsDefaultEngineNone(t *testing.T) {
	llm := newFakeLLM(t)
	e := Start(t, Options{
		Env:        aiEnv(llm, map[string]string{"APP_DEFAULT_ENGINE": "none"}),
		ExtraSites: map[string]map[string]any{hostAISite: defaultSiteConfig()},
	})
	r := e.Extract(serveAI(e, llm, hostAISite, oakChair))
	r.Failed(t, "ENGINE_UNAVAILABLE", "fetch", "service", true)
	if !strings.Contains(r.Str("error.root_cause"), "APP_DEFAULT_ENGINE") {
		t.Errorf("root_cause = %q, want APP_DEFAULT_ENGINE", r.Str("error.root_cause"))
	}
	if n := len(llm.Calls()); n != 0 {
		t.Errorf("provider called %d times for an unavailable default engine", n)
	}

	_, byID := e.listSites(t, e.Key)
	if s := byID["follows-default"]; s == nil || s["engine"] != nil {
		t.Errorf("follows-default in the sites list = %v, want no engine", s)
	}
}
