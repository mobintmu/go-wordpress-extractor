package aicrawler

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"go-wordpress/internal/siteconfig"
)

// Product is the model's answer, before it is checked against the page.
// Every value is untrusted.
type Product struct {
	IsProduct   bool     `json:"is_product"`
	Name        *string  `json:"name"`
	URL         *string  `json:"url"`
	Image       *string  `json:"image"`
	Description *string  `json:"description"`
	Price       Price    `json:"price"`
	Specs       []Spec   `json:"specs"`
	Features    []string `json:"features"`
}

// Price is the model's price answer. Amounts stay text: the Go price
// parser turns them into integers, never the model.
type Price struct {
	CurrentText *string `json:"current_text"`
	RegularText *string `json:"regular_text"`
	Currency    *string `json:"currency"`
	State       string  `json:"state"`
}

// Spec is one specification pair from the model.
type Spec struct {
	Group *string `json:"group"`
	Name  string  `json:"name"`
	Value string  `json:"value"`
}

// Str returns *p or "".
func Str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ErrNoJSON means the answer held no JSON object.
var ErrNoJSON = errors.New("the answer holds no JSON object")

// jsonObject returns the outermost {...} of s, dropping code fences and
// text around it, which models sometimes add despite the prompt.
func jsonObject(s string) (string, error) {
	s = strings.TrimSpace(s)
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return "", ErrNoJSON
	}
	return s[i : j+1], nil
}

// ParseProduct decodes the model's extraction answer.
func ParseProduct(content string) (*Product, error) {
	obj, err := jsonObject(content)
	if err != nil {
		return nil, err
	}
	var p Product
	if err := json.Unmarshal([]byte(obj), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Selectors is the model's selector learning answer.
type Selectors struct {
	Root      string               `json:"root"`
	PriceArea *string              `json:"price_area"`
	Fields    map[string]*FieldSel `json:"fields"`
	Specs     *SpecsSel            `json:"specs"`
	Features  *struct {
		Selector *string `json:"selector"`
	} `json:"features"`
	Availability *struct {
		OutOfStock  *string `json:"out_of_stock"`
		OrderButton *string `json:"order_button"`
	} `json:"availability"`
}

// FieldSel is one learned field selector.
type FieldSel struct {
	Selector  *string             `json:"selector"`
	Attr      siteconfig.AttrList `json:"attr"`
	Pick      *string             `json:"pick"`
	Separator *string             `json:"separator"`
}

// SpecsSel is the learned specifications layout.
type SpecsSel struct {
	Layout *string `json:"layout"`
	Group  *string `json:"group"`
	Title  *string `json:"title"`
	Row    *string `json:"row"`
	Name   *string `json:"name"`
	Value  *string `json:"value"`
}

// ParseSelectors decodes the model's selector learning answer.
func ParseSelectors(content string) (*Selectors, error) {
	obj, err := jsonObject(content)
	if err != nil {
		return nil, err
	}
	var s Selectors
	if err := json.Unmarshal([]byte(obj), &s); err != nil {
		return nil, err
	}
	if strings.TrimSpace(s.Root) == "" {
		return nil, errors.New("the answer has no root selector")
	}
	return &s, nil
}

// Cached is one extraction kept in the cache.
type Cached struct {
	Product      *Product
	InputTokens  int64
	OutputTokens int64
	RequestID    string
	// Model is the model that answered when it differs from the configured
	// one (a Cloudflare dynamic route); "" otherwise.
	Model string
	// Sources are the web pages a search model used (ai_fetch web_search).
	Sources []string
}

// Cache keeps extractions by key for a time. It holds at most maxEntries;
// when full, expired entries are dropped first, then the oldest.
type Cache struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]cacheEntry
}

type cacheEntry struct {
	v   Cached
	exp time.Time
}

const maxEntries = 2000

// NewCache returns a cache; ttl 0 turns it off (Get never hits).
func NewCache(ttl time.Duration) *Cache { return &Cache{ttl: ttl, m: map[string]cacheEntry{}} }

// Get returns the cached extraction for key.
func (c *Cache) Get(key string) (Cached, bool) {
	if c == nil || c.ttl <= 0 {
		return Cached{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.exp) {
		delete(c.m, key)
		return Cached{}, false
	}
	return e.v, true
}

// Put stores an extraction.
func (c *Cache) Put(key string, v Cached) {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxEntries {
		now := time.Now()
		var oldestK string
		var oldest time.Time
		for k, e := range c.m {
			if now.After(e.exp) {
				delete(c.m, k)
				continue
			}
			if oldestK == "" || e.exp.Before(oldest) {
				oldestK, oldest = k, e.exp
			}
		}
		if len(c.m) >= maxEntries {
			delete(c.m, oldestK)
		}
	}
	c.m[key] = cacheEntry{v: v, exp: time.Now().Add(c.ttl)}
}
