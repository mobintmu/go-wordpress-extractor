package search

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

// trackingParams are dropped before two URLs are compared.
var trackingParams = map[string]bool{"gclid": true, "fbclid": true, "msclkid": true, "_ga": true}

// Normalize returns a URL key for comparing a search result with the
// requested URL: lowercase host without "www.", no scheme, no trailing "/",
// no fragment, and the query without tracking parameters (utm_*, gclid,
// fbclid) or the parameters in ignore, sorted. It returns "" when raw is not
// an absolute http(s) URL.
func Normalize(raw string, ignore []string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")), "www.")
	p := u.EscapedPath()
	if dec, err := url.PathUnescape(p); err == nil {
		p = dec
	}
	p = strings.TrimRight(p, "/")

	skip := map[string]bool{}
	for _, name := range ignore {
		skip[strings.ToLower(name)] = true
	}
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || trackingParams[lk] || skip[lk] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(host)
	b.WriteString(p)
	for i, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for j, v := range vals {
			if i == 0 && j == 0 {
				b.WriteByte('?')
			} else {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k) + "=" + url.QueryEscape(v))
		}
	}
	return b.String()
}

// SameURL reports whether a result URL is the requested product URL once
// both are normalized. This exact match is what keeps a similar product
// (another size, another colour) from being returned as the requested one.
func SameURL(result, requested string, ignore []string) bool {
	a := Normalize(result, ignore)
	return a != "" && a == Normalize(requested, ignore)
}

var wordSplit = regexp.MustCompile(`[^\pL\pN]+`)

// SlugWords returns the words of the URL path's slug: the path segment with
// the most words, so "/de/product/universalverbinder-haefele-ixconnect-uc-16-64/P-01901571/"
// gives "universalverbinder haefele ixconnect uc 16 64". A file extension
// such as ".html" is dropped.
func SlugWords(p string) string {
	best, bestN := "", 0
	for _, seg := range strings.Split(p, "/") {
		if dec, err := url.PathUnescape(seg); err == nil {
			seg = dec
		}
		seg = strings.TrimSuffix(seg, path.Ext(seg))
		words := strings.Fields(strings.ToLower(wordSplit.ReplaceAllString(seg, " ")))
		if len(words) > bestN {
			best, bestN = strings.Join(words, " "), len(words)
		}
	}
	return best
}

// Queries builds the search texts for target, in the order kinds lists
// them ("site_slug", "url"). Empty or repeated texts are left out.
func Queries(target *url.URL, kinds []string) []string {
	host := strings.TrimPrefix(strings.ToLower(target.Hostname()), "www.")
	var out []string
	seen := map[string]bool{}
	for _, k := range kinds {
		var q string
		switch k {
		case "site_slug":
			if words := SlugWords(target.Path); words != "" {
				q = "site:" + host + " " + words
			}
		case "url":
			p := strings.TrimRight(target.Path, "/")
			if dec, err := url.PathUnescape(p); err == nil {
				p = dec
			}
			q = `"` + host + p + `"`
		}
		if q != "" && !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	return out
}
