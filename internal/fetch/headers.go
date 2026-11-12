package fetch

import (
	"net/http"
	"strings"

	"go-wordpress/internal/siteconfig"
)

// A browser identifies itself with more than a User-Agent: a top-level
// document navigation also carries Accept, Accept-Language, the Sec-CH-UA
// client hints (Chromium) and the Sec-Fetch metadata. CDN bot protection such
// as Akamai or Cloudflare scores that set, so a request with nothing but a
// User-Agent is refused even when the string is a browser's. The values below
// are derived from the site's own user agent: a Chrome hint is never attached
// to a Firefox or a bot string, because an incoherent pair scores worse than
// no hints at all.
const (
	// acceptDocument is what a browser asks for when a person opens a page.
	acceptDocument = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"

	acceptLanguageEnglish = "en-US,en;q=0.9"
	acceptLanguageGerman  = "de-DE,de;q=0.9,en;q=0.6"
	acceptLanguagePersian = "fa-IR,fa;q=0.9,en;q=0.7"

	// greyBrand is the deliberately meaningless brand Chrome puts first.
	greyBrand = `"Not_A Brand";v="24"`
)

// requestHeaders returns the headers to send for one product page fetch.
func requestHeaders(site *siteconfig.Site, userAgent string) http.Header {
	h := http.Header{}
	h.Set("Accept", acceptDocument)
	h.Set("Accept-Language", acceptLanguage(site))
	if version, brand, ok := chromium(userAgent); ok {
		h.Set("Sec-CH-UA", greyBrand+`, "Chromium";v="`+version+`", `+brand+`;v="`+version+`"`)
		platform, mobile := platformOf(userAgent)
		if platform != "" {
			h.Set("Sec-CH-UA-Platform", `"`+platform+`"`)
		}
		h.Set("Sec-CH-UA-Mobile", mobile)
		h.Set("Sec-Fetch-Dest", "document")
		h.Set("Sec-Fetch-Mode", "navigate")
		h.Set("Sec-Fetch-Site", "none")
		h.Set("Sec-Fetch-User", "?1")
		h.Set("Upgrade-Insecure-Requests", "1")
	}
	return h
}

// acceptLanguage prefers the language of the market the site sells to: the
// dollar sites here are US shops, the euro site is German, and the toman and
// rial sites are Iranian.
func acceptLanguage(site *siteconfig.Site) string {
	switch site.Locale.Currency() {
	case siteconfig.CurrencyUSD:
		return acceptLanguageEnglish
	case siteconfig.CurrencyEUR:
		return acceptLanguageGerman
	}
	return acceptLanguagePersian
}

// chromium reports the major version and the sec-ch-ua brand of a Chromium
// browser, and false for anything else, including a custom bot string.
func chromium(ua string) (version, brand string, ok bool) {
	brand = `"Google Chrome"`
	marker := "Chrome/"
	for _, b := range []struct{ marker, name string }{
		{"Edg/", `"Microsoft Edge"`},
		{"OPR/", `"Opera"`},
	} {
		if strings.Contains(ua, b.marker) {
			marker, brand = b.marker, b.name
			break
		}
	}
	if !strings.Contains(ua, marker) {
		return "", "", false
	}
	if version = versionAfter(ua, marker); version == "" {
		return "", "", false
	}
	return version, brand, true
}

// versionAfter returns the digits that follow marker in ua.
func versionAfter(ua, marker string) string {
	rest := ua[strings.Index(ua, marker)+len(marker):]
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return rest[:i]
		}
	}
	return rest
}

// platformOf returns the sec-ch-ua-platform and sec-ch-ua-mobile values of
// the operating system in ua. The platform is empty when ua names none.
func platformOf(ua string) (platform, mobile string) {
	switch {
	case strings.Contains(ua, "Android"):
		return "Android", "?1"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iPod"):
		return "iOS", "?1"
	case strings.Contains(ua, "Windows"):
		return "Windows", "?0"
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		return "macOS", "?0"
	case strings.Contains(ua, "CrOS"):
		return "Chrome OS", "?0"
	case strings.Contains(ua, "Linux"), strings.Contains(ua, "X11"):
		return "Linux", "?0"
	}
	return "", "?0"
}
