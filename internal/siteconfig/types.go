// Package siteconfig loads per-site parsing rules from JSON files,
// merges templates, validates them, and serves an immutable snapshot.
package siteconfig

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"regexp"
	"time"
)

// Field names a product page config may define.
const (
	FieldTitle        = "title"
	FieldImageURL     = "image_url"
	FieldPriceCurrent = "price_current"
	FieldPriceRegular = "price_regular"
	FieldDescription  = "description"
)

// KnownFields lists every field the extractor understands.
var KnownFields = []string{FieldTitle, FieldImageURL, FieldPriceCurrent, FieldPriceRegular, FieldDescription}

// Pick modes.
const (
	PickFirst = "first"
	PickLast  = "last"
	PickJoin  = "join"
)

// Price units.
const (
	UnitToman = "toman"
	UnitRial  = "rial"
	UnitUSD   = "usd"
	UnitEUR   = "eur"
)

// Output currencies, as returned in price.currency.
const (
	CurrencyIRT = "IRT" // toman; used for toman and rial sites
	CurrencyUSD = "USD" // amounts are in cents
	CurrencyEUR = "EUR" // amounts are in cents
)

// Site is one site's merged and validated configuration.
type Site struct {
	SchemaVersion int        `json:"schema_version"`
	SiteID        string     `json:"site_id"`
	Version       string     `json:"version"`
	Enabled       *bool      `json:"enabled"`
	Extends       string     `json:"extends"`
	Domains       []string   `json:"domains"`
	Fetch         Fetch      `json:"fetch"`
	Locale        Locale     `json:"locale"`
	Pages         Pages      `json:"pages"`
	Validation    Validation `json:"validation"`

	compiledMatch []*regexp.Regexp
}

// Fetch engines a site config can select with fetch.engine.
const (
	// EngineHTTP fetches with Colly over Go's HTTP client. It is the default.
	EngineHTTP = "http"
	// EngineBrowser loads the page in a real Chrome browser through Playwright.
	EngineBrowser = "browser"
	// EngineDuckDuckGo builds the product from DuckDuckGo's HTML search
	// results. The site itself is never contacted.
	EngineDuckDuckGo = "duckduckgo"
	// EngineBrave builds the product from the Brave Search API. The site
	// itself is never contacted.
	EngineBrave = "brave"
)

// IsSearchEngine reports whether name is an engine that reads search
// results instead of the site.
func IsSearchEngine(name string) bool {
	return name == EngineDuckDuckGo || name == EngineBrave
}

// Search query kinds, as listed in fetch.search.queries.
const (
	// QuerySiteSlug is "site:<domain> <words from the URL path>".
	QuerySiteSlug = "site_slug"
	// QueryURL is the normalized product URL in quotes.
	QueryURL = "url"
)

// Fetch controls how a site's pages are retrieved.
type Fetch struct {
	// Engine selects the fetch strategy: "http" (default), "browser",
	// "duckduckgo" or "brave".
	Engine string `json:"engine"`
	// AlternateEngines are used, in order, when Engine is not enabled on
	// the instance. The choice is made once per request; a failed fetch is
	// never retried on another engine. Only search engines may be listed.
	AlternateEngines []string `json:"alternate_engines"`
	// Search configures the search engines. Only allowed when Engine is a
	// search engine.
	Search    *Search `json:"search"`
	UserAgent string  `json:"user_agent"`
	// UserAgents, when set, replaces UserAgent: each fetch picks one of
	// them at random and keeps it for its retries and redirects.
	UserAgents       []string `json:"user_agents"`
	DelayMS          *int     `json:"delay_ms"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	Retries          *int     `json:"retries"`
	RespectRobotsTxt *bool    `json:"respect_robots_txt"`
}

// Search configures how a search engine looks a product URL up.
type Search struct {
	// Region is "<country>-<lang>", for example "us-en"; "wt-wt" means no
	// region. Sent as kl to DuckDuckGo and as country + search_lang to Brave.
	Region string `json:"region"`
	// TitleStrip lists suffixes and prefixes removed from the result title.
	TitleStrip []string `json:"title_strip"`
	// IgnoreParams lists extra query parameters ignored when matching URLs.
	IgnoreParams []string `json:"ignore_params"`
	// Queries lists the queries to try, in order (QuerySiteSlug, QueryURL).
	Queries []string `json:"queries"`
}

// Locale holds language and currency settings.
type Locale struct {
	PriceUnit         string   `json:"price_unit"`
	CurrencyWords     []string `json:"currency_words"`
	CallForPriceWords []string `json:"call_for_price_words"`
}

// Currency is the price.currency returned for this locale's price unit.
func (l Locale) Currency() string {
	switch l.PriceUnit {
	case UnitUSD:
		return CurrencyUSD
	case UnitEUR:
		return CurrencyEUR
	}
	return CurrencyIRT
}

// Pages holds the page types the service reads. Only product pages exist.
type Pages struct {
	Product ProductPage `json:"product"`
}

// ProductPage holds the selectors for a product page.
type ProductPage struct {
	Match        []string         `json:"match"`
	Root         string           `json:"root"`
	PriceArea    string           `json:"price_area"`
	Fields       map[string]Field `json:"fields"`
	Availability Availability     `json:"availability"`
	Specs        *Specs           `json:"specs"`
}

// Specs reads optional name/value pairs, such as a specifications table.
// Every selector is searched inside the one before it: group inside the
// product root, title and row inside a group, name and value inside a row.
type Specs struct {
	// Elements that each hold one group of rows. Empty means the product
	// root is a single group without a title.
	Group string `json:"group"`
	// The group's heading, inside a group. Optional.
	Title string `json:"title"`
	Row   string `json:"row"`
	Name  string `json:"name"`
	Value string `json:"value"`
	// Elements whose text is left out of titles, names, and values, such
	// as hidden tooltips inside a name. Optional.
	Exclude string `json:"exclude"`
}

// Field describes how one value is read inside the root element.
type Field struct {
	Selector  string   `json:"selector"`
	Attr      AttrList `json:"attr"`
	Pick      string   `json:"pick"`
	Separator *string  `json:"separator"`
}

// Availability selectors decide whether the product can be ordered.
type Availability struct {
	OutOfStock  string `json:"out_of_stock"`
	OrderButton string `json:"order_button"`
}

// Validation holds acceptance rules.
type Validation struct {
	RequiredFields []string `json:"required_fields"`
}

// AttrList accepts a single attribute name or a list tried in order.
type AttrList []string

func (a *AttrList) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		if one == "" {
			*a = nil
		} else {
			*a = AttrList{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("attr must be a string or a list of strings")
	}
	*a = many
	return nil
}

// IsEnabled reports whether the site accepts requests.
func (s *Site) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// DelayMS returns the minimum gap between requests to the site.
func (s *Site) Delay() time.Duration { return time.Duration(*s.Fetch.DelayMS) * time.Millisecond }

// Timeout returns the per-attempt fetch timeout.
func (s *Site) Timeout() time.Duration { return time.Duration(s.Fetch.TimeoutSeconds) * time.Second }

// Retries returns how many times a transient failure is retried.
func (s *Site) Retries() int { return *s.Fetch.Retries }

// PickUserAgent returns a random entry of user_agents, or user_agent when
// the list is empty.
func (s *Site) PickUserAgent() string {
	if n := len(s.Fetch.UserAgents); n > 0 {
		return s.Fetch.UserAgents[rand.IntN(n)]
	}
	return s.Fetch.UserAgent
}

// Engines returns the engine and its alternates, in the order they are
// tried when choosing an engine for a request.
func (s *Site) Engines() []string {
	return append([]string{s.Fetch.Engine}, s.Fetch.AlternateEngines...)
}

// SearchSettings returns the site's search settings, with defaults applied
// by Validate. It is never nil for a validated search site.
func (s *Site) SearchSettings() Search {
	if s.Fetch.Search == nil {
		return Search{}
	}
	return *s.Fetch.Search
}

// RespectRobots reports whether robots.txt is honored.
func (s *Site) RespectRobots() bool { return *s.Fetch.RespectRobotsTxt }

// MatchesProductPath reports whether path is a product page for this site.
func (s *Site) MatchesProductPath(path string) bool {
	for _, re := range s.compiledMatch {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// HasDomain reports whether host is one of the site's domains.
func (s *Site) HasDomain(host string) bool {
	host = NormalizeHost(host)
	for _, d := range s.Domains {
		if d == host {
			return true
		}
	}
	return false
}

// SeparatorOr returns the field's separator or def.
func (f Field) SeparatorOr(def string) string {
	if f.Separator == nil {
		return def
	}
	return *f.Separator
}
