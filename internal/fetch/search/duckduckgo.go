package search

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go-wordpress/internal/diagnose"

	"github.com/PuerkitoBio/goquery"
)

// DuckDuckGoEndpoint is DuckDuckGo's results page for browsers without
// JavaScript. Its robots.txt allows every path. It is not an API: there is
// no documentation or versioning, and the HTML can change.
const DuckDuckGoEndpoint = "https://html.duckduckgo.com/html/"

// maxResultsPage caps how much of a results page is read.
const maxResultsPage = 2 << 20

// DuckDuckGo reads DuckDuckGo's HTML results page. It needs no API key.
type DuckDuckGo struct {
	client    *http.Client
	endpoint  string
	userAgent string
}

// NewDuckDuckGo builds the provider. client must apply the service's
// egress rules; userAgent is the honest client name (APP_SEARCH_USER_AGENT).
func NewDuckDuckGo(client *http.Client, userAgent string) *DuckDuckGo {
	return &DuckDuckGo{client: client, endpoint: DuckDuckGoEndpoint, userAgent: userAgent}
}

// WithEndpoint returns a copy that sends queries to endpoint, for tests.
func (d *DuckDuckGo) WithEndpoint(endpoint string) *DuckDuckGo {
	c := *d
	c.endpoint = endpoint
	return &c
}

// Name implements Provider.
func (d *DuckDuckGo) Name() string { return ProviderDuckDuckGo }

// Search posts the query the way the page's own search form does.
func (d *DuckDuckGo) Search(ctx context.Context, q Query) ([]Result, *diagnose.Failure) {
	form := url.Values{"q": {q.Text}}
	if q.Region != "" {
		form.Set("kl", q.Region)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "The DuckDuckGo request could not be built: %v.", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", d.userAgent)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, transportFailure("DuckDuckGo", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResultsPage))
	if err != nil {
		return nil, transportFailure("DuckDuckGo", err)
	}
	switch {
	case resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden:
		return nil, diagnose.New(diagnose.SearchProviderBlocked, diagnose.ConfidenceHigh,
			"DuckDuckGo answered HTTP %d instead of results, which it does when it rate-limits or checks automated use. The service does not work around it.", resp.StatusCode)
	case resp.StatusCode >= 500:
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "DuckDuckGo answered HTTP %d.", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "DuckDuckGo answered HTTP %d, which the service does not handle.", resp.StatusCode)
	}
	return ParseDuckDuckGo(body)
}

// ParseDuckDuckGo reads the web results from a results page. Ads
// (div.result--ad) are skipped. A page with the "No results" block gives an
// empty slice. A page with neither results nor that block is DuckDuckGo's
// bot check ("anomaly") page, or a changed layout, and gives
// SEARCH_PROVIDER_BLOCKED.
func ParseDuckDuckGo(body []byte) ([]Result, *diagnose.Failure) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh, "The DuckDuckGo results page could not be parsed: %v.", err)
	}
	if doc.Find("div.result--no-result, .no-results__message").Length() > 0 {
		return []Result{}, nil
	}
	var out []Result
	doc.Find("div.result.web-result").Not(".result--ad, .result--no-result").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("a.result__a").First()
		link := resultURL(a.AttrOr("href", ""))
		title := cleanText(a.Text())
		if link == "" || title == "" {
			return
		}
		out = append(out, Result{
			Title:   title,
			URL:     link,
			Snippet: cleanText(s.Find(".result__snippet").First().Text()),
			Rank:    len(out) + 1,
		})
	})
	if len(out) == 0 {
		return nil, diagnose.New(diagnose.SearchProviderBlocked, diagnose.ConfidenceMedium,
			"DuckDuckGo answered HTTP 200, but the page has neither results nor the \"No results\" message. That is DuckDuckGo's bot check page, or the results page layout changed (%d bytes, title %q).",
			len(body), strings.TrimSpace(doc.Find("title").First().Text()))
	}
	return out, nil
}

// resultURL returns the destination of a result link. Links are usually the
// direct URL; an older form goes through //duckduckgo.com/l/?uddg=<URL>.
func resultURL(href string) string {
	href = strings.TrimSpace(href)
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if target := u.Query().Get("uddg"); target != "" && strings.HasSuffix(u.Hostname(), "duckduckgo.com") {
		href = target
		if u, err = url.Parse(href); err != nil {
			return ""
		}
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

// transportFailure maps a network error to SEARCH_PROVIDER_ERROR. The
// request's own deadline is reported by the caller as DEADLINE_EXCEEDED.
func transportFailure(provider string, err error) *diagnose.Failure {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return diagnose.New(diagnose.SearchProviderError, diagnose.ConfidenceHigh,
		"The request to %s failed: %s.", provider, strings.TrimSuffix(err.Error(), "."))
}

// String describes a result for logs.
func (r Result) String() string { return fmt.Sprintf("#%d %s", r.Rank, r.URL) }
