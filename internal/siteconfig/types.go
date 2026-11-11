// Package siteconfig loads per-site parsing rules from JSON files,
// merges templates, validates them, and serves an immutable snapshot.
package siteconfig

import (
	"encoding/json"
	"fmt"
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

// Fetch controls how a site's pages are retrieved.
type Fetch struct {
	UserAgent        string `json:"user_agent"`
	DelayMS          *int   `json:"delay_ms"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	Retries          *int   `json:"retries"`
	RespectRobotsTxt *bool  `json:"respect_robots_txt"`
}

// Locale holds language and currency settings.
type Locale struct {
	PriceUnit         string   `json:"price_unit"`
	CurrencyWords     []string `json:"currency_words"`
	CallForPriceWords []string `json:"call_for_price_words"`
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
