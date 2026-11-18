package aicrawler

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"

	"go-wordpress/internal/normalize"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// outlineAttrs are the attributes kept in the outline: the ones stable
// selectors are built from.
var outlineAttrs = map[string]bool{"id": true, "class": true, "itemprop": true, "itemtype": true, "role": true, "name": true, "rel": true, "type": true}

// Outline is a compact view of the page's DOM for selector learning: one
// line per element with its tag, id, classes, itemprop and data-*
// attributes and a short sample of its own text, indented by depth.
// Scripts, styles and hidden elements are left out. At most maxChars
// characters.
func Outline(body []byte, maxChars int) (string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	doc.Find("script, style, svg, noscript, iframe, template, link, meta, canvas").Remove()
	var b strings.Builder
	var walk func(n *html.Node, depth int)
	walk = func(n *html.Node, depth int) {
		if b.Len() > maxChars {
			return
		}
		if n.Type == html.ElementNode {
			line := describe(n)
			if t := ownText(n); t != "" {
				line += ` "` + t + `"`
			}
			b.WriteString(strings.Repeat(" ", min(depth, 40)) + line + "\n")
			depth++
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, depth)
		}
	}
	for _, n := range doc.Find("body").Nodes {
		walk(n, 0)
	}
	out := b.String()
	if utf8.RuneCountInString(out) > maxChars {
		out = string([]rune(out)[:maxChars])
	}
	return out, nil
}

func describe(n *html.Node) string {
	var b strings.Builder
	b.WriteString(n.Data)
	var extra []string
	for _, a := range n.Attr {
		k := a.Key
		v := strings.TrimSpace(a.Val)
		switch {
		case k == "id" && v != "":
			b.WriteString("#" + v)
		case k == "class" && v != "":
			for _, c := range strings.Fields(v) {
				b.WriteString("." + c)
			}
		case outlineAttrs[k] || strings.HasPrefix(k, "data-") || ((k == "src" || k == "content") && n.Data != "a"):
			if k == "id" || k == "class" {
				continue
			}
			if len(v) > 80 {
				v = v[:80] + "…"
			}
			extra = append(extra, k+`="`+v+`"`)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		b.WriteString("[" + strings.Join(extra, " ") + "]")
	}
	return b.String()
}

// ownText is the element's direct text, at most 80 characters.
func ownText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data + " ")
		}
	}
	t := normalize.Text(b.String())
	if utf8.RuneCountInString(t) > 80 {
		t = string([]rune(t)[:80]) + "…"
	}
	return t
}
