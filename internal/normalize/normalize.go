// Package normalize cleans extracted text: titles, prices, and URLs.
package normalize

import (
	"errors"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Digits converts Persian (۰-۹) and Arabic-Indic (٠-٩) digits to ASCII.
func Digits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// Text strips stray HTML tags and collapses whitespace, including
// non-breaking spaces. Zero-width spaces and direction marks are removed;
// the zero-width non-joiner (U+200C) is kept because Persian words use it.
func Text(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\u00a0', '\u202f':
			return ' '
		case '\u200b', '\ufeff', '\u200e', '\u200f':
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// Paragraphs cleans each part and joins the non-empty ones with sep.
func Paragraphs(parts []string, sep string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := Text(p); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, sep)
}

// numberRe matches a number with optional thousands separators: comma,
// Arabic thousands separator, Arabic comma, dot groups, or apostrophes.
var numberRe = regexp.MustCompile(`\d+(?:[,٬،.'’]\d{3})*`)

// ErrNoPrice means price text had no number in it.
var ErrNoPrice = errors.New("no number in price text")

// Prices returns every number found in text, in order. Persian and Arabic
// digits are converted first, because \d only matches ASCII digits.
func Prices(text string) ([]int64, error) {
	text = Digits(text)
	matches := numberRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil, ErrNoPrice
	}
	out := make([]int64, 0, len(matches))
	for _, m := range matches {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, m)
		v, err := strconv.ParseInt(digits, 10, 64)
		if err != nil || v > math.MaxInt64/10 {
			return nil, errors.New("price number out of range: " + m)
		}
		out = append(out, v)
	}
	return out, nil
}

// usdRe matches a US dollar amount: optional comma thousands groups and
// an optional fraction of one or two digits ("1,234.50", "19.9", "420").
var usdRe = regexp.MustCompile(`\d{1,3}(?:,\d{3})+(?:\.\d{1,2})?|\d+(?:\.\d{1,2})?`)

// usdCents returns every dollar amount in text, in cents.
func usdCents(text string) ([]int64, error) {
	matches := usdRe.FindAllString(Digits(text), -1)
	if len(matches) == 0 {
		return nil, ErrNoPrice
	}
	out := make([]int64, 0, len(matches))
	for _, m := range matches {
		whole, frac, _ := strings.Cut(strings.ReplaceAll(m, ",", ""), ".")
		frac = (frac + "00")[:2]
		w, err := strconv.ParseInt(whole, 10, 64)
		if err != nil || w > (math.MaxInt64-99)/100 {
			return nil, errors.New("price number out of range: " + m)
		}
		f, _ := strconv.ParseInt(frac, 10, 64)
		out = append(out, w*100+f)
	}
	return out, nil
}

// LowestPrice returns the lowest positive amount in text, in the output
// unit for the site's price unit: toman for "toman" and "rial" (rial is
// divided by 10), and cents for "usd". A text with only zeros yields
// (nil, nil): zero is never a price.
func LowestPrice(text, unit string) (*int64, error) {
	var nums []int64
	var err error
	if unit == "usd" {
		nums, err = usdCents(text)
	} else {
		nums, err = Prices(text)
	}
	if err != nil {
		return nil, err
	}
	var lowest *int64
	for _, n := range nums {
		if unit == "rial" {
			n /= 10
		}
		if n <= 0 {
			continue
		}
		if lowest == nil || n < *lowest {
			v := n
			lowest = &v
		}
	}
	return lowest, nil
}

// ContainsAny reports whether text contains any of words, ignoring case
// and extra whitespace.
func ContainsAny(text string, words []string) bool {
	t := strings.ToLower(Text(text))
	for _, w := range words {
		if w = strings.ToLower(Text(w)); w != "" && strings.Contains(t, w) {
			return true
		}
	}
	return false
}

// ResolveURL resolves ref against base and returns an absolute http(s)
// URL, or "" if ref is empty, a data: URI, or not http(s).
func ResolveURL(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(strings.ToLower(ref), "data:") || strings.HasPrefix(strings.ToLower(ref), "javascript:") {
		return ""
	}
	// srcset-like values: take the first candidate.
	if strings.Contains(ref, ", ") || strings.HasSuffix(ref, "w") && strings.Contains(ref, " ") {
		ref = strings.Fields(ref)[0]
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}
