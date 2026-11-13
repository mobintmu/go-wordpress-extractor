package search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go-wordpress/internal/diagnose"

	"go.uber.org/zap"
)

// BraveEndpoint is the Brave Search API web search endpoint.
const BraveEndpoint = "https://api.search.brave.com/res/v1/web/search"

// braveCount is how many results are asked for (the API allows up to 20).
const braveCount = 10

// Brave calls the Brave Search API with the instance's own API key.
type Brave struct {
	client    *http.Client
	endpoint  string
	key       string
	userAgent string
	log       *zap.Logger
}

// NewBrave builds the provider. key is APP_BRAVE_API_KEY; it is sent only
// in the X-Subscription-Token header and never logged.
func NewBrave(client *http.Client, key, userAgent string, log *zap.Logger) *Brave {
	if log == nil {
		log = zap.NewNop()
	}
	return &Brave{client: client, endpoint: BraveEndpoint, key: key, userAgent: userAgent, log: log}
}

// WithEndpoint returns a copy that sends queries to endpoint, for tests.
func (b *Brave) WithEndpoint(endpoint string) *Brave {
	c := *b
	c.endpoint = endpoint
	return &c
}

// Name implements Provider.
func (b *Brave) Name() string { return ProviderBrave }

// braveResponse is the part of the API response the engine reads.
type braveResponse struct {
	Web *struct {
		Results []struct {
			Title         string   `json:"title"`
			URL           string   `json:"url"`
			Description   string   `json:"description"`
			ExtraSnippets []string `json:"extra_snippets"`
		} `json:"results"`
	} `json:"web"`
}

// Search runs one web search.
func (b *Brave) Search(ctx context.Context, q Query) ([]Result, *diagnose.Failure) {
	params := url.Values{"q": {q.Text}, "count": {strconv.Itoa(braveCount)}}
	if country, lang := splitRegion(q.Region); country != "" {
		params.Set("country", strings.ToUpper(country))
		params.Set("search_lang", lang)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "The Brave request could not be built: %v.", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.key)
	req.Header.Set("User-Agent", b.userAgent)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, transportFailure("Brave", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResultsPage))
	if err != nil {
		return nil, transportFailure("Brave", err)
	}
	b.log.Debug("brave search",
		zap.Int("status", resp.StatusCode),
		zap.String("ratelimit_limit", resp.Header.Get("X-RateLimit-Limit")),
		zap.String("ratelimit_remaining", resp.Header.Get("X-RateLimit-Remaining")),
		zap.String("ratelimit_reset", resp.Header.Get("X-RateLimit-Reset")))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, diagnose.New(diagnose.SearchProviderAuthFailed, diagnose.ConfidenceHigh,
			"Brave answered HTTP %d: the API key is missing, wrong or revoked, or the plan does not include web search.", resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, diagnose.New(diagnose.SearchQuotaExceeded, diagnose.ConfidenceHigh,
			"Brave answered HTTP 429: the per-second limit or the plan's quota is used up (X-RateLimit-Remaining %q).", resp.Header.Get("X-RateLimit-Remaining"))
	case resp.StatusCode >= 500:
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "Brave answered HTTP %d.", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh,
			"Brave answered HTTP %d (for example 422 for an invalid parameter).", resp.StatusCode)
	}
	return ParseBrave(body)
}

// ParseBrave reads web.results from an API response. No "web" key, or an
// empty list, means no results.
func ParseBrave(body []byte) ([]Result, *diagnose.Failure) {
	var r braveResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "The Brave response is not the expected JSON: %v.", err)
	}
	out := []Result{}
	if r.Web == nil {
		return out, nil
	}
	for _, w := range r.Web.Results {
		u, err := url.Parse(strings.TrimSpace(w.URL))
		title := cleanText(w.Title)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || title == "" {
			continue
		}
		snippet := cleanText(w.Description)
		for _, extra := range w.ExtraSnippets {
			if e := cleanText(extra); e != "" {
				snippet = strings.TrimSpace(snippet + " " + e)
			}
		}
		out = append(out, Result{Title: title, URL: u.String(), Snippet: snippet, Rank: len(out) + 1})
	}
	return out, nil
}
