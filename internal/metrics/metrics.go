// Package metrics keeps request counters and writes them in the
// Prometheus text exposition format.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

var durationBuckets = []float64{0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 180}

// Metrics holds every counter the service exposes.
type Metrics struct {
	mu        sync.Mutex
	requests  map[[4]string]uint64 // outcome, site, code, price_state
	rejected  map[string]uint64    // reason
	durations map[string]*histogram
	active    func() int
}

type histogram struct {
	counts []uint64
	sum    float64
	count  uint64
}

// New creates an empty registry.
func New() *Metrics {
	return &Metrics{
		requests:  map[[4]string]uint64{},
		rejected:  map[string]uint64{},
		durations: map[string]*histogram{},
	}
}

// SetActive registers a function that reports active requests.
func (m *Metrics) SetActive(f func() int) { m.active = f }

// ObserveExtraction records one processed request.
func (m *Metrics) ObserveExtraction(outcome, site, code, priceState string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[[4]string{outcome, site, code, priceState}]++
	h, ok := m.durations[outcome]
	if !ok {
		h = &histogram{counts: make([]uint64, len(durationBuckets))}
		m.durations[outcome] = h
	}
	s := d.Seconds()
	for i, b := range durationBuckets {
		if s <= b {
			h.counts[i]++
		}
	}
	h.sum += s
	h.count++
}

// Reject records a request answered without processing (401, 429, 503).
func (m *Metrics) Reject(reason string) {
	m.mu.Lock()
	m.rejected[reason]++
	m.mu.Unlock()
}

// Write renders all metrics.
func (m *Metrics) Write(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Fprintln(w, "# HELP extract_requests_total Extraction requests processed, by outcome, site, failure code, and price state.")
	fmt.Fprintln(w, "# TYPE extract_requests_total counter")
	keys := make([][4]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.Join(keys[i][:], "|") < strings.Join(keys[j][:], "|") })
	for _, k := range keys {
		fmt.Fprintf(w, "extract_requests_total{outcome=%q,site=%q,code=%q,price_state=%q} %d\n", esc(k[0]), esc(k[1]), esc(k[2]), esc(k[3]), m.requests[k])
	}

	fmt.Fprintln(w, "# HELP extract_rejected_total Requests rejected before processing, by reason.")
	fmt.Fprintln(w, "# TYPE extract_rejected_total counter")
	reasons := make([]string, 0, len(m.rejected))
	for r := range m.rejected {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		fmt.Fprintf(w, "extract_rejected_total{reason=%q} %d\n", esc(r), m.rejected[r])
	}

	fmt.Fprintln(w, "# HELP extract_request_duration_seconds Duration of processed extraction requests.")
	fmt.Fprintln(w, "# TYPE extract_request_duration_seconds histogram")
	outcomes := make([]string, 0, len(m.durations))
	for o := range m.durations {
		outcomes = append(outcomes, o)
	}
	sort.Strings(outcomes)
	for _, o := range outcomes {
		h := m.durations[o]
		for i, b := range durationBuckets {
			fmt.Fprintf(w, "extract_request_duration_seconds_bucket{outcome=%q,le=%q} %d\n", esc(o), trimFloat(b), h.counts[i])
		}
		fmt.Fprintf(w, "extract_request_duration_seconds_bucket{outcome=%q,le=\"+Inf\"} %d\n", esc(o), h.count)
		fmt.Fprintf(w, "extract_request_duration_seconds_sum{outcome=%q} %g\n", esc(o), h.sum)
		fmt.Fprintf(w, "extract_request_duration_seconds_count{outcome=%q} %d\n", esc(o), h.count)
	}

	fmt.Fprintln(w, "# HELP extract_active_requests Extraction requests running now.")
	fmt.Fprintln(w, "# TYPE extract_active_requests gauge")
	active := 0
	if m.active != nil {
		active = m.active()
	}
	fmt.Fprintf(w, "extract_active_requests %d\n", active)
}

func esc(s string) string { return strings.ReplaceAll(s, "\n", " ") }

func trimFloat(f float64) string { return strings.TrimSuffix(fmt.Sprintf("%g", f), ".0") }
