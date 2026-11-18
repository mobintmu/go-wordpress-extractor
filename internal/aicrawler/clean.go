package aicrawler

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"go-wordpress/internal/normalize"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// Page is a product page reduced for the model, plus what is needed to
// check the model's answer against the page (grounding).
type Page struct {
	// Text is what the model reads: title, canonical, meta, JSON-LD and
	// the visible content as compact text, at most maxChars characters.
	Text string
	// Truncated reports whether Text was cut to maxChars.
	Truncated bool

	// corpus is every text of the page (visible text, meta values, image
	// alt texts and JSON-LD), normalized and lowercased, never truncated.
	corpus string
	// raw is corpus before lowercasing, for reading amounts.
	raw string
	// words is the set of words in corpus.
	words map[string]bool
	// urls holds every absolute URL found in img src/srcset/data-src,
	// og:image, link hrefs and JSON-LD.
	urls map[string]bool
}

// removed lists elements whose content is never product data.
const removed = "script, style, svg, noscript, iframe, template, canvas, header, nav, footer, [hidden], [aria-hidden=true], [style*='display:none'], [style*='display: none']"

var urlInText = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

// Clean parses body and builds the model input and grounding data. base
// resolves relative URLs.
func Clean(body []byte, base *url.URL, maxChars int) (*Page, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p := &Page{urls: map[string]bool{}, words: map[string]bool{}}
	var head strings.Builder
	var corpus strings.Builder
	addURL := func(ref string) {
		if abs := normalize.ResolveURL(base, ref); abs != "" {
			p.urls[abs] = true
		}
	}

	if t := normalize.Text(doc.Find("title").First().Text()); t != "" {
		head.WriteString("TITLE: " + t + "\n")
		corpus.WriteString(t + "\n")
	}
	if href, ok := doc.Find(`link[rel="canonical"]`).First().Attr("href"); ok {
		if abs := normalize.ResolveURL(base, href); abs != "" {
			head.WriteString("CANONICAL: " + abs + "\n")
		}
	}
	doc.Find("meta[property], meta[name]").Each(func(_ int, m *goquery.Selection) {
		key, _ := m.Attr("property")
		if key == "" {
			key, _ = m.Attr("name")
		}
		k := strings.ToLower(key)
		if !(strings.HasPrefix(k, "og:") || strings.HasPrefix(k, "product:") || k == "description" || strings.HasPrefix(k, "twitter:")) {
			return
		}
		v := normalize.Text(m.AttrOr("content", ""))
		if v == "" {
			return
		}
		if strings.HasSuffix(k, ":image") || strings.HasSuffix(k, ":image:url") || strings.HasSuffix(k, ":image:secure_url") {
			addURL(v)
			if abs := normalize.ResolveURL(base, v); abs != "" {
				v = abs
			}
		}
		head.WriteString("META " + k + ": " + v + "\n")
		corpus.WriteString(v + "\n")
	})
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		j := strings.Join(strings.Fields(s.Text()), " ")
		if j == "" {
			return
		}
		head.WriteString("JSON-LD: " + j + "\n")
		corpus.WriteString(jsonText(j) + "\n")
		// JSON often escapes slashes: "https:\/\/cdn.example\/a.jpg".
		for _, u := range urlInText.FindAllString(strings.ReplaceAll(j, `\/`, "/"), -1) {
			addURL(u)
		}
	})
	doc.Find("img, source").Each(func(_ int, img *goquery.Selection) {
		for _, a := range []string{"src", "data-src", "data-original", "data-lazy-src", "data-zoom-image", "data-large_image"} {
			if v, ok := img.Attr(a); ok {
				addURL(v)
			}
		}
		for _, a := range []string{"srcset", "data-srcset"} {
			if v, ok := img.Attr(a); ok {
				for _, cand := range strings.Split(v, ",") {
					if f := strings.Fields(cand); len(f) > 0 {
						addURL(f[0])
					}
				}
			}
		}
	})
	doc.Find("a[href], link[href]").Each(func(_ int, a *goquery.Selection) {
		if v, ok := a.Attr("href"); ok && looksLikeImage(v) {
			addURL(v)
		}
	})

	// Visible content, as compact text with simple structure markers.
	bodySel := doc.Find("body")
	// The corpus keeps header/nav/footer text too: a price or name may sit there.
	full := normalize.Text(textOf(bodySel))
	corpus.WriteString(full + "\n")
	bodySel.Find("img[alt]").Each(func(_ int, img *goquery.Selection) {
		corpus.WriteString(normalize.Text(img.AttrOr("alt", "")) + "\n")
	})
	bodySel.Find(removed).Remove()
	var content strings.Builder
	for _, n := range bodySel.Nodes {
		render(&content, n, base)
	}

	text := head.String() + "CONTENT:\n" + collapseBlankLines(content.String())
	if utf8.RuneCountInString(text) > maxChars && maxChars > 0 {
		r := []rune(text)
		text = string(r[:maxChars])
		p.Truncated = true
	}
	p.Text = text
	p.raw = normalize.Digits(normalize.Text(corpus.String()))
	p.corpus = strings.ToLower(p.raw)
	for _, w := range words(p.corpus) {
		p.words[w] = true
	}
	return p, nil
}

func looksLikeImage(u string) bool {
	l := strings.ToLower(u)
	if i := strings.IndexAny(l, "?#"); i >= 0 {
		l = l[:i]
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif"} {
		if strings.HasSuffix(l, ext) {
			return true
		}
	}
	return false
}

// jsonText makes JSON-LD readable for grounding: escaped slashes and
// \uXXXX escapes are undone and punctuation becomes spaces.
func jsonText(j string) string {
	j = strings.ReplaceAll(j, `\/`, "/")
	j = unicodeEscape.ReplaceAllStringFunc(j, func(m string) string {
		var r rune
		for _, c := range m[2:] {
			r <<= 4
			switch {
			case c >= '0' && c <= '9':
				r |= c - '0'
			case c >= 'a' && c <= 'f':
				r |= c - 'a' + 10
			case c >= 'A' && c <= 'F':
				r |= c - 'A' + 10
			}
		}
		return string(r)
	})
	return strings.NewReplacer(`"`, " ", "{", " ", "}", " ", "[", " ", "]", " ").Replace(j)
}

var unicodeEscape = regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)

// textOf is the text of the selection with spaces around block elements.
func textOf(s *goquery.Selection) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			return
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "noscript", "template", "svg":
				return
			}
			b.WriteByte(' ')
			defer b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range s.Nodes {
		walk(n)
	}
	return b.String()
}

var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "main": true, "aside": true, "ul": true, "ol": true,
	"dl": true, "dt": true, "dd": true, "table": true, "tbody": true, "thead": true, "figure": true, "figcaption": true,
	"blockquote": true, "pre": true, "form": true, "fieldset": true, "details": true, "summary": true, "br": true, "hr": true,
}

// render writes n as compact text: "# " before headings, "- " before list
// items, "| a | b |" for table rows and "[image: url alt]" for images.
func render(b *strings.Builder, n *html.Node, base *url.URL) {
	switch n.Type {
	case html.TextNode:
		if t := normalize.Text(n.Data); t != "" {
			b.WriteString(t + " ")
		}
		return
	case html.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			render(b, c, base)
		}
		return
	}
	switch tag := n.Data; {
	case len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6':
		b.WriteString("\n" + strings.Repeat("#", int(tag[1]-'0')) + " " + normalize.Text(nodeText(n)) + "\n")
		return
	case tag == "li":
		b.WriteString("\n- ")
	case tag == "tr":
		var cells []string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
				cells = append(cells, normalize.Text(nodeText(c)))
			}
		}
		b.WriteString("\n| " + strings.Join(cells, " | ") + " |\n")
		return
	case tag == "img":
		src := attr(n, "src")
		for _, a := range []string{"data-src", "data-original", "data-lazy-src"} {
			if v := attr(n, a); v != "" && (src == "" || strings.HasPrefix(src, "data:")) {
				src = v
			}
		}
		if abs := normalize.ResolveURL(base, src); abs != "" {
			b.WriteString("[image: " + abs)
			if alt := normalize.Text(attr(n, "alt")); alt != "" {
				b.WriteString(" alt=\"" + alt + "\"")
			}
			b.WriteString("] ")
		}
		return
	case tag == "button" || tag == "select" || tag == "option":
		b.WriteString("[" + tag + ": " + normalize.Text(nodeText(n)) + "] ")
		return
	case blockTags[tag]:
		b.WriteString("\n")
		defer b.WriteString("\n")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		render(b, c, base)
	}
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data + " ")
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var blankLines = regexp.MustCompile(`[ \t]*\n[ \t\n]*`)

func collapseBlankLines(s string) string {
	return strings.TrimSpace(blankLines.ReplaceAllString(s, "\n"))
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

func words(s string) []string { return wordRe.FindAllString(strings.ToLower(s), -1) }

// ContainsText reports whether s, normalized and lowercased, appears in the
// page. Empty text never matches.
func (p *Page) ContainsText(s string) bool {
	n := strings.ToLower(normalize.Digits(normalize.Text(s)))
	return n != "" && strings.Contains(p.corpus, n)
}

// WordOverlap is the share of s's words that appear on the page.
func (p *Page) WordOverlap(s string) float64 {
	ws := words(normalize.Digits(s))
	if len(ws) == 0 {
		return 0
	}
	n := 0
	for _, w := range ws {
		if p.words[w] {
			n++
		}
	}
	return float64(n) / float64(len(ws))
}

// HasURL reports whether u is one of the page's image or link URLs.
func (p *Page) HasURL(u string) bool { return p.urls[u] }

// Amounts returns every amount on the page in unit's output unit.
func (p *Page) Amounts(unit string) []int64 { return normalize.Amounts(p.raw, unit) }

// Raw is the page's normalized text, for detecting currency words.
func (p *Page) Raw() string { return p.raw }
