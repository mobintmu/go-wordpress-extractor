package metrics

import (
	"sort"
	"time"
)

// Report is the JSON form of the metrics. Load tells a client how busy
// this instance is right now, so it can pick the least loaded instance;
// the counters cover the time since the process started.
type Report struct {
	Instance    string               `json:"instance" binding:"required" example:"extractor-1"`
	GeneratedAt time.Time            `json:"generated_at" binding:"required" example:"2026-11-11T08:00:00Z"`
	StartedAt   time.Time            `json:"started_at" binding:"required" example:"2026-11-11T07:00:00Z"`
	Load        Load                 `json:"load" binding:"required"`
	Requests    []RequestCount       `json:"requests" binding:"required"`
	Rejected    map[string]uint64    `json:"rejected" binding:"required"`
	Durations   map[string]Histogram `json:"durations" binding:"required"`
} //@name MetricsReport

// Load is the live state of this instance.
type Load struct {
	// Draining is true while the instance shuts down; send nothing to it.
	Draining bool `json:"draining" binding:"required" example:"false"`
	// Capacity is the most extractions this instance runs at once.
	Capacity int `json:"capacity" binding:"required" example:"8"`
	// Active is the number of extractions running now.
	Active int `json:"active" binding:"required" example:"3"`
	// Available is Capacity minus Active; 0 means the next request gets 429.
	Available int `json:"available" binding:"required" example:"5"`
	// Utilization is Active divided by Capacity, from 0 to 1.
	Utilization float64 `json:"utilization" binding:"required" example:"0.375"`
	// PerSiteLimit is the most extractions that run at once for one site.
	PerSiteLimit int `json:"per_site_limit" binding:"required" example:"1"`
	// SitesActive maps each busy site_id to its running extractions.
	// A site at PerSiteLimit makes new requests for it wait or get 429.
	SitesActive map[string]int `json:"sites_active" binding:"required"`
} //@name MetricsLoad

// RequestCount is one extract_requests_total series.
type RequestCount struct {
	Outcome    string `json:"outcome" binding:"required" enums:"success,failed,cancelled" example:"success"`
	SiteID     string `json:"site_id" example:"hmland"`
	Code       string `json:"code" example:""`
	PriceState string `json:"price_state" example:"available"`
	Count      uint64 `json:"count" binding:"required" example:"42"`
} //@name MetricsRequestCount

// Histogram is the duration distribution for one outcome.
type Histogram struct {
	Count      uint64   `json:"count" binding:"required" example:"42"`
	SumSeconds float64  `json:"sum_seconds" binding:"required" example:"131.4"`
	AvgSeconds float64  `json:"avg_seconds" binding:"required" example:"3.13"`
	Buckets    []Bucket `json:"buckets" binding:"required"`
} //@name MetricsHistogram

// Bucket counts requests that took at most LeSeconds (cumulative).
type Bucket struct {
	LeSeconds float64 `json:"le_seconds" binding:"required" example:"5"`
	Count     uint64  `json:"count" binding:"required" example:"40"`
} //@name MetricsBucket

// Snapshot copies the counters into a Report. The caller fills Instance
// and Load.
func (m *Metrics) Snapshot() Report {
	m.mu.Lock()
	defer m.mu.Unlock()

	r := Report{
		GeneratedAt: time.Now().UTC(),
		StartedAt:   m.started,
		Requests:    make([]RequestCount, 0, len(m.requests)),
		Rejected:    make(map[string]uint64, len(m.rejected)),
		Durations:   make(map[string]Histogram, len(m.durations)),
	}
	for k, n := range m.requests {
		r.Requests = append(r.Requests, RequestCount{Outcome: k[0], SiteID: k[1], Code: k[2], PriceState: k[3], Count: n})
	}
	sort.Slice(r.Requests, func(i, j int) bool {
		a, b := r.Requests[i], r.Requests[j]
		if a.SiteID != b.SiteID {
			return a.SiteID < b.SiteID
		}
		if a.Outcome != b.Outcome {
			return a.Outcome < b.Outcome
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.PriceState < b.PriceState
	})
	for reason, n := range m.rejected {
		r.Rejected[reason] = n
	}
	for o, h := range m.durations {
		out := Histogram{Count: h.count, SumSeconds: h.sum, Buckets: make([]Bucket, len(durationBuckets))}
		if h.count > 0 {
			out.AvgSeconds = h.sum / float64(h.count)
		}
		for i, b := range durationBuckets {
			out.Buckets[i] = Bucket{LeSeconds: b, Count: h.counts[i]}
		}
		r.Durations[o] = out
	}
	return r
}
