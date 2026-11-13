// Package search looks a product URL up in a web search provider
// (DuckDuckGo's HTML results page or the Brave Search API). The product's own
// site is never contacted: the search engines exist for sites whose bot
// protection the service does not get past.
package search

import (
	"context"
	"html"
	"regexp"
	"strings"

	"go-wordpress/internal/diagnose"
)

// Provider names, as used in fetch.engine.
const (
	ProviderDuckDuckGo = "duckduckgo"
	ProviderBrave      = "brave"
)

// Query is one search.
type Query struct {
	// Text is the search text, for example
	// `site:fergusonhome.com duravit 030549 0hole`.
	Text string
	// Region is "<country>-<lang>", or "wt-wt" for no region.
	Region string
}

// Result is one web result, as the provider showed it.
type Result struct {
	Title   string
	URL     string
	Snippet string
	// Rank is the 1-based position in the results.
	Rank int
}

// Provider runs one query against a search service. It returns the web
// results in the provider's order (ads left out), or a failure with one of
// the SEARCH_* codes. An empty slice with no failure means "no results".
type Provider interface {
	Name() string
	Search(ctx context.Context, q Query) ([]Result, *diagnose.Failure)
}

var tagPattern = regexp.MustCompile(`<[^>]*>`)

// cleanText removes HTML tags (Brave marks matches with <strong>), decodes
// entities and collapses white space.
func cleanText(s string) string {
	s = tagPattern.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

// splitRegion turns "us-en" into ("us", "en"). "wt-wt" or an empty region
// gives empty strings.
func splitRegion(region string) (country, lang string) {
	c, l, ok := strings.Cut(strings.ToLower(region), "-")
	if !ok || c == "wt" {
		return "", ""
	}
	return c, l
}
