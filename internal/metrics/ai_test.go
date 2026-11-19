package metrics

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAICounters(t *testing.T) {
	m := New()
	m.ObserveAICall("ai-crawler-avalai", "m1", "extract", "a.test", "ok", 1000, 200, time.Second)
	m.ObserveAICall("ai-crawler-avalai", "m1", "extract", "a.test", "ok", 500, 100, time.Second)
	m.ObserveAICall("ai-crawler-avalai", "m1", "learn", "a.test", "AI_RATE_LIMITED", 0, 0, time.Second)
	m.ObserveAICacheHit("ai-crawler-avalai", "m1", "a.test")

	r := m.Snapshot().AI
	if r.Calls != 3 || r.Failed != 1 || r.InputTokens != 1500 || r.OutputTokens != 300 || r.CacheHits != 1 || len(r.Series) != 2 {
		t.Errorf("report = %+v", r)
	}
	var b bytes.Buffer
	m.Write(&b)
	for _, want := range []string{
		`extract_ai_calls_total{engine="ai-crawler-avalai",model="m1",purpose="extract",host="a.test",outcome="ok"} 2`,
		`extract_ai_tokens_total{engine="ai-crawler-avalai",model="m1",purpose="extract",host="a.test",outcome="ok",kind="input"} 1500`,
		`extract_ai_calls_total{engine="ai-crawler-avalai",model="m1",purpose="learn",host="a.test",outcome="AI_RATE_LIMITED"} 1`,
		`extract_ai_cache_hits_total{engine="ai-crawler-avalai",model="m1",host="a.test"} 1`,
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("text lacks %q\n%s", want, b.String())
		}
	}
}

// The default rule serves any domain, so the host label is capped.
func TestAIHostsCapped(t *testing.T) {
	m := New()
	for i := range maxAIHosts + 5 {
		m.ObserveAICall("ai-crawler-avalai", "m", "extract", fmt.Sprintf("h%d.test", i), "ok", 1, 1, 0)
	}
	r := m.Snapshot().AI
	if len(r.Series) != maxAIHosts+1 {
		t.Errorf("series = %d, want %d", len(r.Series), maxAIHosts+1)
	}
	for _, s := range r.Series {
		if s.Host == OtherHost && s.Calls != 5 {
			t.Errorf("other = %d calls, want 5", s.Calls)
		}
	}
}
