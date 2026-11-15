package search

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"go-wordpress/internal/diagnose"

	mws "github.com/jcalvert/metawebsearch"
	"go.uber.org/zap"
)

// Scraped reads one search engine's HTML results page (Mojeek, Yahoo or
// Yandex) with a request builder and a result parser from
// github.com/jcalvert/metawebsearch. It needs no API key.
//
// Only those two parts of the library are used. Its own HTTP client
// impersonates a browser's TLS fingerprint and headers and retries blocks,
// which the service does not do: requests go through the service's egress
// client with the honest APP_SEARCH_USER_AGENT, a block (202, 403, 429) is
// reported as SEARCH_PROVIDER_BLOCKED and never retried, and the results
// page is capped at maxResultsPage bytes. See harden.
type Scraped struct {
	name      string
	label     string
	engine    mws.EngineConfig
	client    *http.Client
	userAgent string
	// endpoint, when set, replaces the scheme and host of the engine's own
	// URL. For tests against a fake provider.
	endpoint string
	log      *zap.Logger
}

// NewMojeek builds the provider for www.mojeek.com. Mojeek has its own
// index and honours the site's search.region.
func NewMojeek(client *http.Client, userAgent string, log *zap.Logger) *Scraped {
	return newScraped(ProviderMojeek, "Mojeek", mws.Mojeek, client, userAgent, log)
}

// NewYahoo builds the provider for search.yahoo.com, whose results come
// from Bing's index. It ignores search.region.
func NewYahoo(client *http.Client, userAgent string, log *zap.Logger) *Scraped {
	return newScraped(ProviderYahoo, "Yahoo", mws.Yahoo, client, userAgent, log)
}

// NewYandex builds the provider for yandex.com. It ignores search.region.
func NewYandex(client *http.Client, userAgent string, log *zap.Logger) *Scraped {
	return newScraped(ProviderYandex, "Yandex", mws.Yandex, client, userAgent, log)
}

func newScraped(name, label string, engine mws.EngineConfig, client *http.Client, userAgent string, log *zap.Logger) *Scraped {
	if log == nil {
		log = zap.NewNop()
	}
	return &Scraped{name: name, label: label, engine: engine, client: client, userAgent: userAgent, log: log}
}

// WithEndpoint returns a copy whose queries go to endpoint's scheme and
// host instead of the engine's own, for tests.
func (s *Scraped) WithEndpoint(endpoint string) *Scraped {
	c := *s
	c.endpoint = endpoint
	return &c
}

// Name implements Provider.
func (s *Scraped) Name() string { return s.name }

// Search runs one query: BuildRequest, one HTTP request through the egress
// client, ParseResponse, PostProcess.
func (s *Scraped) Search(ctx context.Context, q Query) ([]Result, *diagnose.Failure) {
	country, _ := splitRegion(q.Region)
	engine, err := harden(s.engine, s.userAgent, s.endpoint, country == "")
	if err != nil {
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "The %s request could not be built: %v.", s.label, err)
	}
	client := &limitedClient{client: s.client}
	results, err := mws.Execute(ctx, client, engine, q.Text, mws.SearchOpts{Region: q.Region})
	if err != nil {
		return nil, s.failure(client, err)
	}
	out := convertResults(results)
	if len(out) == 0 {
		title := pageTitle(client.body)
		if isChallengePage(client.body, client.finalURL) {
			return nil, diagnose.New(diagnose.SearchProviderBlocked, diagnose.ConfidenceMedium,
				"%s answered HTTP %d, but the page is a bot check or a challenge, not results (%d bytes, title %q). The service does not work around it.",
				s.label, client.status, len(client.body), title)
		}
		s.log.Info("search returned no results", zap.String("engine", s.name), zap.Int("bytes", len(client.body)), zap.String("title", title),
			zap.String("note", "if this happens for every query, the results page layout may have changed"))
	}
	return out, nil
}

// failure maps an Execute error to a SEARCH_* failure from what the
// client recorded, not from the error text.
func (s *Scraped) failure(c *limitedClient, err error) *diagnose.Failure {
	switch {
	case c.err != nil:
		return transportFailure(s.label, c.err)
	case c.status == 0:
		return diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "The %s request could not be built: %v.", s.label, err)
	case isBlockedStatus(c.status):
		return diagnose.New(diagnose.SearchProviderBlocked, diagnose.ConfidenceHigh,
			"%s answered HTTP %d instead of results, which it does when it rate-limits or checks automated use. The service does not work around it.", s.label, c.status)
	case c.status >= 500:
		return diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "%s answered HTTP %d.", s.label, c.status)
	case c.status < 200 || c.status >= 300:
		return diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "%s answered HTTP %d, which the service does not handle.", s.label, c.status)
	default:
		return diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh,
			"The %s results page could not be read (%v). The page layout may have changed.", s.label, err)
	}
}

// harden returns a copy of engine that follows the service's rules. Every
// EngineConfig field is public, so the library is not forked:
//
//   - ClientProfile asks for a browser TLS fingerprint. Such an engine
//     (Google) is refused.
//   - BuildRequest is wrapped to send userAgent instead of the library's
//     Chrome user agent, to drop Mojeek's region cookies when the site has
//     no region, and to redirect to endpoint when one is set.
//   - MinDelay 0 turns off the library's global rate limiter, which sleeps
//     without a context. SearchFetcher spaces queries instead.
//   - RetryableStatus never retries: the first 202, 429 or 503 is the
//     answer.
//
// TestHardenCoversEveryEngineConfigField fails when a library upgrade adds
// a field, so this list is reviewed with it.
func harden(engine mws.EngineConfig, userAgent, endpoint string, noRegion bool) (mws.EngineConfig, error) {
	if engine.ClientProfile != "" {
		return mws.EngineConfig{}, fmt.Errorf("engine %s needs the browser TLS profile %q, which the service does not impersonate", engine.Name, engine.ClientProfile)
	}
	var base *url.URL
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return mws.EngineConfig{}, fmt.Errorf("endpoint %q is not an absolute URL", endpoint)
		}
		base = u
	}
	build := engine.BuildRequest
	engine.BuildRequest = func(query string, opts mws.SearchOpts) (*http.Request, error) {
		req, err := build(query, opts)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		if noRegion {
			dropCookies(req, "arc", "lb")
		}
		if base != nil {
			req.URL.Scheme, req.URL.Host, req.Host = base.Scheme, base.Host, ""
		}
		return req, nil
	}
	engine.MinDelay = 0
	engine.MaxRetries = 1
	engine.RetryableStatus = func(int) bool { return false }
	return engine, nil
}

// dropCookies removes the named cookies from the request.
func dropCookies(req *http.Request, names ...string) {
	drop := map[string]bool{}
	for _, n := range names {
		drop[n] = true
	}
	cookies := req.Cookies()
	req.Header.Del("Cookie")
	for _, c := range cookies {
		if !drop[c.Name] {
			req.AddCookie(c)
		}
	}
}

// browserOnlyHeaders are added by the library after BuildRequest. They
// claim a browser navigation, so they are removed before the request is
// sent. Accept and Accept-Language stay: they state a preference, not an
// identity.
var browserOnlyHeaders = []string{
	"Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform",
	"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site", "Sec-Fetch-User",
	"Upgrade-Insecure-Requests",
}

// isBlockedStatus reports the statuses a search engine answers with when it
// rate-limits or checks for automated use: 202 (DuckDuckGo's and Yandex's
// bot check), 403 and 429.
func isBlockedStatus(status int) bool {
	return status == http.StatusAccepted || status == http.StatusForbidden || status == http.StatusTooManyRequests
}

// errBlocked stops the library's pipeline at a blocked status. The library
// treats 202 as success, so the client has to refuse it before the parser
// reads what is a challenge page.
var errBlocked = fmt.Errorf("blocked")

// limitedClient is the HTTPClient the library calls. It removes the
// browser-only headers, reads at most maxResultsPage bytes, and records the
// status, the final URL and the body, so failures and challenge pages are
// recognised from them. err holds a transport error only; a blocked status
// is in status.
type limitedClient struct {
	client   *http.Client
	status   int
	finalURL string
	body     []byte
	err      error
}

func (c *limitedClient) Do(req *http.Request) (*http.Response, error) {
	for _, h := range browserOnlyHeaders {
		req.Header.Del(h)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.err = err
		return nil, err
	}
	c.status = resp.StatusCode
	if resp.Request != nil && resp.Request.URL != nil {
		c.finalURL = resp.Request.URL.String()
	}
	if isBlockedStatus(c.status) {
		resp.Body.Close()
		return nil, errBlocked
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResultsPage))
	resp.Body.Close()
	if err != nil {
		c.err = err
		return nil, err
	}
	c.body = body
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// convertResults keeps the results with a title and an absolute http(s)
// URL, in the engine's order.
func convertResults(results []mws.Result) []Result {
	out := []Result{}
	for _, r := range results {
		u, err := url.Parse(strings.TrimSpace(r.URL))
		title := cleanText(r.Title)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || title == "" {
			continue
		}
		out = append(out, Result{Title: title, URL: u.String(), Snippet: cleanText(r.Snippet), Rank: len(out) + 1})
	}
	return out
}

var (
	titlePattern     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	challengePattern = regexp.MustCompile(`(?i)captcha|just a moment|unusual traffic|verify (?:that )?you are (?:a )?human|are you a robot|access denied|attention required`)
)

// pageTitle returns the page's <title>, for logs and failures.
func pageTitle(body []byte) string {
	m := titlePattern.FindSubmatch(body)
	if m == nil {
		return ""
	}
	return cleanText(string(m[1]))
}

// isChallengePage reports whether a page with no results is a bot check:
// a CAPTCHA, a "just a moment" interstitial or a redirect to one (Yandex
// sends /showcaptcha).
func isChallengePage(body []byte, finalURL string) bool {
	if u, err := url.Parse(finalURL); err == nil && strings.Contains(strings.ToLower(u.Path), "captcha") {
		return true
	}
	return challengePattern.Match(body)
}
