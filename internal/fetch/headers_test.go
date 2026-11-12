package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"go-wordpress/internal/siteconfig"
)

const (
	chromeWindows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
	chromeMac     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
	firefoxMac    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:143.0) Gecko/20100101 Firefox/143.0"
	chromeAndroid = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36"
	edgeWindows   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0"
	botAgent      = "ProductExtractor/1.0"
)

func siteWithLocale(unit, userAgent string) *siteconfig.Site {
	s := testSite()
	s.Locale.PriceUnit = unit
	s.Fetch.UserAgent = userAgent
	return s
}

// A Chrome user agent gets the client hints and fetch metadata a Chrome
// browser sends, and they agree with the string they come from.
func TestRequestHeadersChrome(t *testing.T) {
	h := requestHeaders(siteWithLocale(siteconfig.UnitUSD, chromeWindows), chromeWindows)
	for name, want := range map[string]string{
		"Accept":                    acceptDocument,
		"Accept-Language":           acceptLanguageEnglish,
		"Sec-CH-UA":                 `"Not_A Brand";v="24", "Chromium";v="141", "Google Chrome";v="141"`,
		"Sec-CH-UA-Mobile":          "?0",
		"Sec-CH-UA-Platform":        `"Windows"`,
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	} {
		if got := h.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// The hints follow the brand and the platform named in the user agent.
func TestRequestHeadersBrandAndPlatform(t *testing.T) {
	for _, c := range []struct {
		name, ua, brand, platform, mobile string
	}{
		{"edge", edgeWindows, `"Microsoft Edge";v="141"`, `"Windows"`, "?0"},
		{"android", chromeAndroid, `"Google Chrome";v="141"`, `"Android"`, "?1"},
		{"mac", chromeMac, `"Google Chrome";v="141"`, `"macOS"`, "?0"},
	} {
		h := requestHeaders(siteWithLocale(siteconfig.UnitUSD, c.ua), c.ua)
		if got := h.Get("Sec-CH-UA"); !strings.Contains(got, c.brand) {
			t.Errorf("%s: Sec-CH-UA = %q, want %s", c.name, got, c.brand)
		}
		if got := h.Get("Sec-CH-UA-Platform"); got != c.platform {
			t.Errorf("%s: Sec-CH-UA-Platform = %q, want %q", c.name, got, c.platform)
		}
		if got := h.Get("Sec-CH-UA-Mobile"); got != c.mobile {
			t.Errorf("%s: Sec-CH-UA-Mobile = %q, want %q", c.name, got, c.mobile)
		}
	}
}

// Firefox and a custom bot string send no client hints, so neither gets the
// rest of the navigation metadata, and the language follows the market.
func TestRequestHeadersWithoutClientHints(t *testing.T) {
	for _, c := range []struct {
		name, ua, unit, language string
	}{
		{"firefox", firefoxMac, siteconfig.UnitUSD, acceptLanguageEnglish},
		{"bot", botAgent, siteconfig.UnitToman, acceptLanguagePersian},
	} {
		h := requestHeaders(siteWithLocale(c.unit, c.ua), c.ua)
		for _, name := range []string{
			"Sec-CH-UA", "Sec-CH-UA-Mobile", "Sec-CH-UA-Platform",
			"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site", "Sec-Fetch-User",
			"Upgrade-Insecure-Requests",
		} {
			if got := h.Get(name); got != "" {
				t.Errorf("%s: %s = %q, want it absent", c.name, name, got)
			}
		}
		if got := h.Get("Accept"); got != acceptDocument {
			t.Errorf("%s: Accept = %q", c.name, got)
		}
		if got := h.Get("Accept-Language"); got != c.language {
			t.Errorf("%s: Accept-Language = %q, want %q", c.name, got, c.language)
		}
	}
}

// The wire carries the whole set, and the cookie a site sets on the first
// response comes back on the next fetch of that site, but not on another's.
func TestFetchSendsBrowserHeadersAndKeepsCookies(t *testing.T) {
	var mu sync.Mutex
	var seen []http.Header
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		n := len(seen)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		if n == 1 {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc", Path: "/"})
		}
		w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)

	last := func() http.Header {
		mu.Lock()
		defer mu.Unlock()
		return seen[len(seen)-1]
	}

	f := testFetcher(pu, publicResolver)
	site := siteWithLocale(siteconfig.UnitUSD, chromeWindows)
	target, _ := url.Parse("http://shop.example/product/1")
	if _, fail := f.Fetch(context.Background(), site, target); fail != nil {
		t.Fatalf("first fetch failed: %+v", fail)
	}

	first := last()
	for name, want := range map[string]string{
		"User-Agent":                chromeWindows,
		"Accept":                    acceptDocument,
		"Accept-Language":           acceptLanguageEnglish,
		"Sec-CH-UA-Platform":        `"Windows"`,
		"Sec-CH-UA-Mobile":          "?0",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	} {
		if got := first.Get(name); got != want {
			t.Errorf("on the wire: %s = %q, want %q", name, got, want)
		}
	}
	if got := first.Get("Sec-CH-UA"); !strings.Contains(got, `"Google Chrome";v="141"`) {
		t.Errorf("on the wire: Sec-CH-UA = %q", got)
	}

	// A second fetch is a new collector, but the site's jar lives on.
	if _, fail := f.Fetch(context.Background(), site, target); fail != nil {
		t.Fatalf("second fetch failed: %+v", fail)
	}
	if got := last().Get("Cookie"); !strings.Contains(got, "sid=abc") {
		t.Errorf("second fetch Cookie = %q, want the session cookie", got)
	}

	// Another site has its own jar and must not see that cookie.
	other := siteWithLocale(siteconfig.UnitToman, botAgent)
	other.SiteID = "other"
	if _, fail := f.Fetch(context.Background(), other, target); fail != nil {
		t.Fatalf("third fetch failed: %+v", fail)
	}
	if got := last().Get("Cookie"); got != "" {
		t.Errorf("another site's fetch Cookie = %q, want none", got)
	}
	if got := last().Get("Sec-CH-UA"); got != "" {
		t.Errorf("a bot user agent must not carry client hints, got %q", got)
	}
}
