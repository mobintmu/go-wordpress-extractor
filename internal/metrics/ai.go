package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// maxAIHosts caps the distinct host labels of the AI counters. The default
// rule serves any domain, so hosts beyond the cap are counted as "other".
const maxAIHosts = 200

// OtherHost is the host label used once maxAIHosts hosts are tracked.
const OtherHost = "other"

// aiKey is one AI series: model, purpose, host, outcome.
type aiKey [4]string

type aiCount struct {
	calls        uint64
	inputTokens  uint64
	outputTokens uint64
	seconds      float64
}

type aiState struct {
	calls     map[aiKey]*aiCount
	cacheHits map[[2]string]uint64 // model, host
	hosts     map[string]bool
}

func newAIState() aiState {
	return aiState{calls: map[aiKey]*aiCount{}, cacheHits: map[[2]string]uint64{}, hosts: map[string]bool{}}
}

func (m *Metrics) aiHost(h string) string {
	if h == "" {
		return "unknown"
	}
	if m.ai.hosts[h] {
		return h
	}
	if len(m.ai.hosts) >= maxAIHosts {
		return OtherHost
	}
	m.ai.hosts[h] = true
	return h
}

// ObserveAICall records one call to the AI provider. outcome is "ok" or
// the failure code.
func (m *Metrics) ObserveAICall(model, purpose, host, outcome string, inputTokens, outputTokens int64, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := aiKey{model, purpose, m.aiHost(host), outcome}
	c, ok := m.ai.calls[k]
	if !ok {
		c = &aiCount{}
		m.ai.calls[k] = c
	}
	c.calls++
	c.inputTokens += uint64(max(inputTokens, 0))
	c.outputTokens += uint64(max(outputTokens, 0))
	c.seconds += d.Seconds()
}

// ObserveAICacheHit records an AI answer served from the cache.
func (m *Metrics) ObserveAICacheHit(model, host string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ai.cacheHits[[2]string{model, m.aiHost(host)}]++
}

// AIReport is the AI crawler's usage since the process started.
type AIReport struct {
	// Calls counts requests sent to the AI provider, retries by the SDK not
	// included. Repair and selector-learning calls are counted too.
	Calls uint64 `json:"calls" binding:"required" example:"12"`
	// Failed counts calls that ended with an AI_* failure code.
	Failed       uint64 `json:"failed" binding:"required" example:"1"`
	InputTokens  uint64 `json:"input_tokens" binding:"required" example:"96000"`
	OutputTokens uint64 `json:"output_tokens" binding:"required" example:"4800"`
	// CacheHits counts answers served from the cache, which used no tokens.
	CacheHits uint64 `json:"cache_hits" binding:"required" example:"3"`
	// Series has one entry per model, purpose, host and outcome.
	Series []AICallCount `json:"series" binding:"required"`
	// CacheHitsByHost maps each host to its cache hits.
	CacheHitsByHost map[string]uint64 `json:"cache_hits_by_host" binding:"required"`
} //@name MetricsAIReport

// AICallCount is one extract_ai_calls_total series with its tokens.
type AICallCount struct {
	Model   string `json:"model" binding:"required" example:"deepseek-v4.1-flash"`
	Purpose string `json:"purpose" binding:"required" enums:"extract,repair,learn" example:"extract"`
	// Host is the shop's host, "other" once 200 hosts are tracked.
	Host string `json:"host" binding:"required" example:"www.wayfair.com"`
	// Outcome is "ok" or the failure code.
	Outcome      string  `json:"outcome" binding:"required" example:"ok"`
	Calls        uint64  `json:"calls" binding:"required" example:"4"`
	InputTokens  uint64  `json:"input_tokens" binding:"required" example:"32000"`
	OutputTokens uint64  `json:"output_tokens" binding:"required" example:"1600"`
	SumSeconds   float64 `json:"sum_seconds" binding:"required" example:"14.2"`
} //@name MetricsAICallCount

func (m *Metrics) aiSnapshot() AIReport {
	r := AIReport{Series: make([]AICallCount, 0, len(m.ai.calls)), CacheHitsByHost: map[string]uint64{}}
	for k, c := range m.ai.calls {
		r.Series = append(r.Series, AICallCount{Model: k[0], Purpose: k[1], Host: k[2], Outcome: k[3],
			Calls: c.calls, InputTokens: c.inputTokens, OutputTokens: c.outputTokens, SumSeconds: c.seconds})
		r.Calls += c.calls
		r.InputTokens += c.inputTokens
		r.OutputTokens += c.outputTokens
		if k[3] != "ok" {
			r.Failed += c.calls
		}
	}
	sort.Slice(r.Series, func(i, j int) bool {
		a, b := r.Series[i], r.Series[j]
		return strings.Join([]string{a.Host, a.Purpose, a.Outcome, a.Model}, "|") < strings.Join([]string{b.Host, b.Purpose, b.Outcome, b.Model}, "|")
	})
	for k, n := range m.ai.cacheHits {
		r.CacheHits += n
		r.CacheHitsByHost[k[1]] += n
	}
	return r
}

func (m *Metrics) writeAI(w io.Writer) {
	keys := make([]aiKey, 0, len(m.ai.calls))
	for k := range m.ai.calls {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.Join(keys[i][:], "|") < strings.Join(keys[j][:], "|") })
	labels := func(k aiKey) string {
		return fmt.Sprintf("model=%q,purpose=%q,host=%q,outcome=%q", esc(k[0]), esc(k[1]), esc(k[2]), esc(k[3]))
	}

	fmt.Fprintln(w, "# HELP extract_ai_calls_total Calls to the AI provider, by model, purpose (extract, repair, learn), host, and outcome (ok or failure code).")
	fmt.Fprintln(w, "# TYPE extract_ai_calls_total counter")
	for _, k := range keys {
		fmt.Fprintf(w, "extract_ai_calls_total{%s} %d\n", labels(k), m.ai.calls[k].calls)
	}
	fmt.Fprintln(w, "# HELP extract_ai_tokens_total Tokens used at the AI provider, by model, purpose, host, outcome, and kind (input or output).")
	fmt.Fprintln(w, "# TYPE extract_ai_tokens_total counter")
	for _, k := range keys {
		c := m.ai.calls[k]
		fmt.Fprintf(w, "extract_ai_tokens_total{%s,kind=\"input\"} %d\n", labels(k), c.inputTokens)
		fmt.Fprintf(w, "extract_ai_tokens_total{%s,kind=\"output\"} %d\n", labels(k), c.outputTokens)
	}
	fmt.Fprintln(w, "# HELP extract_ai_call_seconds_total Time spent waiting for the AI provider.")
	fmt.Fprintln(w, "# TYPE extract_ai_call_seconds_total counter")
	for _, k := range keys {
		fmt.Fprintf(w, "extract_ai_call_seconds_total{%s} %g\n", labels(k), m.ai.calls[k].seconds)
	}

	hits := make([][2]string, 0, len(m.ai.cacheHits))
	for k := range m.ai.cacheHits {
		hits = append(hits, k)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i][0]+"|"+hits[i][1] < hits[j][0]+"|"+hits[j][1] })
	fmt.Fprintln(w, "# HELP extract_ai_cache_hits_total AI answers served from the cache, which used no tokens.")
	fmt.Fprintln(w, "# TYPE extract_ai_cache_hits_total counter")
	for _, k := range hits {
		fmt.Fprintf(w, "extract_ai_cache_hits_total{model=%q,host=%q} %d\n", esc(k[0]), esc(k[1]), m.ai.cacheHits[k])
	}
}
