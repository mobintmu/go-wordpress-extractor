package diagnose

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

// PageSignals summarizes an HTML document for page checks.
type PageSignals struct {
	BodyBytes   int
	TextChars   int
	ScriptBytes int
	AppRoot     string
	BlockMarker string
	BlockHeader string
	Title       string
}

// Markers that identify anti-bot challenge or CAPTCHA pages.
var blockMarkers = []string{
	"cf-browser-verification",
	"cf_chl_opt",
	"challenge-platform",
	"cf-challenge",
	"Attention Required! | Cloudflare",
	"Just a moment...",
	"g-recaptcha",
	"h-captcha",
	"hcaptcha.com",
	"challenges.cloudflare.com/turnstile",
	"__arcdn",
	"arvancloud-challenge",
	"ddos-guard",
	"Checking your browser",
	"_Incapsula_Resource",
	"captcha-delivery.com",
}

// App root selectors used by client-side frameworks.
var appRoots = []string{"#root", "#app", "#__next", "#__nuxt", "app-root", "[data-reactroot]", "[ng-app]", "[data-server-rendered]"}

// Signals inspects the document and headers.
func Signals(doc *goquery.Document, body []byte, headers http.Header) PageSignals {
	s := PageSignals{BodyBytes: len(body)}
	if headers != nil && headers.Get("cf-mitigated") != "" {
		s.BlockHeader = "cf-mitigated: " + headers.Get("cf-mitigated")
	}
	raw := string(body)
	for _, m := range blockMarkers {
		if strings.Contains(raw, m) {
			s.BlockMarker = m
			break
		}
	}
	if doc == nil {
		return s
	}
	s.Title = strings.TrimSpace(doc.Find("title").First().Text())
	doc.Find("script").Each(func(_ int, sc *goquery.Selection) {
		s.ScriptBytes += len(sc.Text())
		if src, ok := sc.Attr("src"); ok && src != "" {
			s.ScriptBytes += 1024 // external bundle; size unknown, count it as substantial
		}
	})
	s.TextChars = VisibleTextLength(doc)
	for _, r := range appRoots {
		if el := doc.Find(r); el.Length() > 0 && utf8.RuneCountInString(strings.TrimSpace(el.Text())) < 200 {
			s.AppRoot = r
			break
		}
	}
	return s
}

// VisibleTextLength counts visible characters in body, ignoring scripts,
// styles, and templates.
func VisibleTextLength(doc *goquery.Document) int {
	body := doc.Find("body").Clone()
	body.Find("script, style, noscript, template").Remove()
	return utf8.RuneCountInString(strings.Join(strings.Fields(body.Text()), " "))
}

// Thresholds for page checks.
const (
	emptyTextChars   = 50
	jsShellTextChars = 500
	jsShellScripts   = 5000
)

// CheckBlocked reports BOT_PROTECTION when the response is a challenge.
// The header is direct evidence; markup is a heuristic.
func CheckBlocked(s PageSignals, status int) *Failure {
	if s.BlockHeader != "" {
		return New(BotProtection, ConfidenceHigh,
			"The response had HTTP %d and a %s header, so the product page was never served.", status, s.BlockHeader)
	}
	if s.BlockMarker != "" {
		return New(BotProtection, ConfidenceMedium,
			"The response had HTTP %d and its HTML contains the challenge marker %q, so the product page was never served.", status, s.BlockMarker)
	}
	return nil
}

// ClassifyMissingProduct explains an HTTP 200 page whose product root did
// not match. The first matching check wins: block page, empty document,
// JavaScript shell, then layout change (nil, meaning SELECTOR_NOT_FOUND).
func ClassifyMissingProduct(s PageSignals, status int) *Failure {
	if f := CheckBlocked(s, status); f != nil {
		return f
	}
	if s.BodyBytes == 0 {
		return New(EmptyPage, ConfidenceHigh, "The page returned HTTP %d with an empty body.", status)
	}
	if s.TextChars < emptyTextChars && s.AppRoot == "" && s.ScriptBytes < jsShellScripts {
		return New(EmptyPage, ConfidenceMedium,
			"The page returned HTTP %d but has only %d visible characters of text and no application shell.", status, s.TextChars)
	}
	if s.TextChars < jsShellTextChars && (s.AppRoot != "" || s.ScriptBytes >= jsShellScripts) {
		detail := "large script payloads"
		if s.AppRoot != "" {
			detail = "an application root element (" + s.AppRoot + ")"
		}
		return New(JSRenderedPage, ConfidenceMedium,
			"The page returned HTTP %d with %s, %d bytes of script, and only %d visible characters. The product is probably rendered by JavaScript.",
			status, detail, s.ScriptBytes, s.TextChars)
	}
	return nil
}
