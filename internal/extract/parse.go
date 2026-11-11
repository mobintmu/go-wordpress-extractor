package extract

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/normalize"
	"go-wordpress/internal/siteconfig"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// parsed is the outcome of the page_check, extract, normalize, and
// validate stages for one HTML document.
type parsed struct {
	product  *Product
	warnings []Warning
	failure  *diagnose.Failure
	trace    []diagnose.TraceStep
	sample   *diagnose.HTMLSample
	fields   map[string]string
}

// PageInput is everything the parser needs from a fetched page.
type PageInput struct {
	Site     *siteconfig.Site
	File     string // site config file name, for suggested actions
	Doc      *goquery.Document
	Body     []byte
	Headers  http.Header
	Status   int
	FinalURL *url.URL
}

// parsePage runs page_check, extract, normalize, and validate.
func parsePage(in PageInput) parsed {
	p := in.Site.Pages.Product
	signals := diagnose.Signals(in.Doc, in.Body, in.Headers)

	// page_check: a challenge header is direct evidence even on HTTP 200.
	if signals.BlockHeader != "" {
		return parsed{failure: diagnose.CheckBlocked(signals, in.Status), sample: bodySample(in.Body)}
	}

	// extract: find the product root.
	root := in.Doc.Find(p.Root)
	if root.Length() == 0 {
		if f := diagnose.ClassifyMissingProduct(signals, in.Status); f != nil {
			return parsed{failure: f, sample: bodySample(in.Body)}
		}
		trace, deepest := diagnose.Trace(in.Doc.Selection, p.Root)
		out := parsed{trace: trace}
		if deepest != nil {
			h, _ := goquery.OuterHtml(deepest)
			out.sample = diagnose.NewSample(diagnose.ReasonDeepestMatch, h)
		} else {
			out.sample = bodySample(in.Body)
		}
		out.failure = diagnose.New(diagnose.SelectorNotFound, diagnose.ConfidenceHigh,
			"The page returned HTTP %d with real content (%d visible characters), but selector '%s' matched 0 elements. The site markup has probably changed.",
			in.Status, signals.TextChars, p.Root).
			WithAction("Update pages.product.root in configs/sites/%s.", in.File)
		return out
	}
	el := root.First()

	// extract: read each configured field.
	values := map[string][]string{}
	samples := map[string]string{}
	for name, f := range p.Fields {
		isPrice := name == siteconfig.FieldPriceCurrent || name == siteconfig.FieldPriceRegular
		vals := readField(el, f, in.FinalURL, name == siteconfig.FieldImageURL, isPrice)
		values[name] = vals
		if len(vals) > 0 {
			samples[name] = diagnose.TruncateText(strings.Join(vals, " | "), 512)
		}
	}
	priceArea := ""
	if p.PriceArea != "" {
		priceArea = normalize.Text(nodesText(el.Find(p.PriceArea), true))
		if priceArea != "" {
			samples["price_area"] = diagnose.TruncateText(priceArea, 512)
		}
	}

	// normalize.
	prod := &Product{Price: Price{Currency: CurrencyIRT, State: PriceUnknown}}
	prod.Title = normalize.Text(pick(values[siteconfig.FieldTitle], p.Fields[siteconfig.FieldTitle], " "))
	if img := pick(values[siteconfig.FieldImageURL], p.Fields[siteconfig.FieldImageURL], " "); img != "" {
		prod.ImageURL = &img
	}
	if df, ok := p.Fields[siteconfig.FieldDescription]; ok {
		var desc string
		if df.Pick == siteconfig.PickJoin {
			desc = normalize.Paragraphs(values[siteconfig.FieldDescription], df.SeparatorOr("\n"))
		} else {
			desc = normalize.Text(pick(values[siteconfig.FieldDescription], df, "\n"))
		}
		if desc != "" {
			prod.Description = &desc
		}
	}
	prod.URL = productURL(in)

	currentText := normalize.Text(pick(values[siteconfig.FieldPriceCurrent], p.Fields[siteconfig.FieldPriceCurrent], " "))
	regularText := normalize.Text(pick(values[siteconfig.FieldPriceRegular], p.Fields[siteconfig.FieldPriceRegular], " "))
	prod.Price.Raw = priceArea
	if prod.Price.Raw == "" {
		prod.Price.Raw = strings.TrimSpace(regularText + " " + currentText)
	}

	callForPrice := normalize.ContainsAny(prod.Price.Raw, in.Site.Locale.CallForPriceWords)
	outOfStock := outOfStock(el, p.Availability)

	out := parsed{product: prod, fields: samples}
	if !callForPrice {
		cur, err := parsePrice(currentText, in.Site.Locale.PriceUnit)
		if err != nil {
			return priceFailure(out, el, siteconfig.FieldPriceCurrent, currentText, p, in.File)
		}
		reg, err := parsePrice(regularText, in.Site.Locale.PriceUnit)
		if err != nil {
			return priceFailure(out, el, siteconfig.FieldPriceRegular, regularText, p, in.File)
		}
		if cur == nil && reg != nil {
			cur, reg = reg, nil
		}
		if cur != nil && reg != nil && *reg == *cur {
			reg = nil
		}
		prod.Price.Current, prod.Price.Regular = cur, reg
	}
	switch {
	case callForPrice:
		prod.Price.State = PriceCallForPrice
	case outOfStock:
		prod.Price.State = PriceOutOfStock
	case prod.Price.Current != nil:
		prod.Price.State = PriceAvailable
	default:
		prod.Price.State = PriceUnknown
	}

	// validate.
	for _, rf := range in.Site.Validation.RequiredFields {
		if fieldPresent(prod, rf) {
			continue
		}
		f := p.Fields[rf]
		rootHTML, _ := goquery.OuterHtml(el)
		steps, _ := diagnose.Trace(el, f.Selector)
		out.trace = append([]diagnose.TraceStep{{Selector: p.Root, Matches: root.Length()}}, steps...)
		out.sample = diagnose.NewSample(diagnose.ReasonProductRoot, rootHTML)
		matched := el.Find(f.Selector).Length()
		detail := fmt.Sprintf("selector '%s' matched 0 elements inside the product root", f.Selector)
		if matched > 0 {
			detail = fmt.Sprintf("selector '%s' matched %d elements inside the product root, but none had a usable value", f.Selector, matched)
		}
		out.failure = diagnose.New(diagnose.RequiredFieldMissing, diagnose.ConfidenceHigh,
			"The product root was found, but the required field '%s' is missing: %s.", rf, detail).
			WithMessage("The required field %s was not found.", rf).
			WithAction("Update pages.product.fields.%s in configs/sites/%s.", rf, in.File)
		return out
	}

	// warnings.
	for _, name := range []string{siteconfig.FieldImageURL, siteconfig.FieldDescription} {
		if _, configured := p.Fields[name]; configured && !fieldPresent(prod, name) {
			out.warnings = append(out.warnings, Warning{Code: WarnFieldMissing, Field: name,
				Message: fmt.Sprintf("The %s field was not found on this product page.", name)})
		}
	}
	if prod.Price.State == PriceUnknown {
		out.warnings = append(out.warnings, Warning{Code: WarnPriceMissing, Field: "price",
			Message: "No price, call-for-price text, or stock signal was found on this product page."})
	}
	return out
}

func priceFailure(out parsed, el *goquery.Selection, field, text string, p siteconfig.ProductPage, file string) parsed {
	rootHTML, _ := goquery.OuterHtml(el)
	out.sample = diagnose.NewSample(diagnose.ReasonProductRoot, rootHTML)
	out.trace = []diagnose.TraceStep{{Selector: p.Root, Matches: 1}}
	steps, _ := diagnose.Trace(el, p.Fields[field].Selector)
	out.trace = append(out.trace, steps...)
	out.failure = diagnose.New(diagnose.PriceParseFailed, diagnose.ConfidenceHigh,
		"The %s selector found the text %q, which contains no number and none of the site's call_for_price_words.", field, diagnose.TruncateText(text, 200)).
		WithAction("Update pages.product.fields.%s or locale.call_for_price_words in configs/sites/%s.", field, file)
	return out
}

func parsePrice(text, unit string) (*int64, error) {
	if text == "" {
		return nil, nil
	}
	return normalize.LowestPrice(text, unit)
}

// outOfStock reports whether the product cannot be ordered: the
// out_of_stock selector matches, or an order button is configured and
// none is found.
func outOfStock(el *goquery.Selection, a siteconfig.Availability) bool {
	if a.OutOfStock != "" && el.Find(a.OutOfStock).Length() > 0 {
		return true
	}
	return a.OrderButton != "" && el.Find(a.OrderButton).Length() == 0
}

func fieldPresent(p *Product, field string) bool {
	switch field {
	case siteconfig.FieldTitle:
		return p.Title != ""
	case siteconfig.FieldImageURL:
		return p.ImageURL != nil
	case siteconfig.FieldDescription:
		return p.Description != nil
	case siteconfig.FieldPriceCurrent:
		return p.Price.Current != nil
	case siteconfig.FieldPriceRegular:
		return p.Price.Regular != nil
	}
	return false
}

// readField returns the non-empty values of every element the field
// selector matches inside el. With attr, each element's attributes are
// tried in order; for URLs, values that do not resolve are skipped.
func readField(el *goquery.Selection, f siteconfig.Field, base *url.URL, isURL, everyNode bool) []string {
	var out []string
	el.Find(f.Selector).Each(func(_ int, s *goquery.Selection) {
		if len(f.Attr) == 0 {
			if t := nodesText(s, everyNode); strings.TrimSpace(t) != "" {
				out = append(out, t)
			}
			return
		}
		for _, a := range f.Attr {
			v, ok := s.Attr(a)
			if !ok || strings.TrimSpace(v) == "" {
				continue
			}
			if isURL {
				if v = normalize.ResolveURL(base, v); v == "" {
					continue
				}
			}
			out = append(out, strings.TrimSpace(v))
			return
		}
	})
	return out
}

func pick(vals []string, f siteconfig.Field, defaultSep string) string {
	if len(vals) == 0 {
		return ""
	}
	switch f.Pick {
	case siteconfig.PickLast:
		return vals[len(vals)-1]
	case siteconfig.PickJoin:
		return strings.Join(vals, f.SeparatorOr(defaultSep))
	default:
		return vals[0]
	}
}

// blockElements get a space around their text, so "<p>a</p><p>b</p>"
// reads "a b", not "ab".
var blockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "dd": true, "div": true,
	"dl": true, "dt": true, "figcaption": true, "figure": true, "footer": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "header": true, "hr": true, "li": true, "main": true,
	"nav": true, "ol": true, "p": true, "pre": true, "section": true, "table": true, "td": true,
	"th": true, "tr": true, "ul": true,
}

// nodesText returns the text of the selection, skipping scripts and
// styles. Block elements and <br> are separated by spaces. With everyNode,
// every text node is separated, so adjacent inline elements like
// <span>100</span><span>200</span> do not merge into one number.
func nodesText(s *goquery.Selection, everyNode bool) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			if everyNode {
				b.WriteByte(' ')
			}
			return
		case html.ElementNode:
			switch {
			case n.Data == "script" || n.Data == "style" || n.Data == "noscript" || n.Data == "template":
				return
			case n.Data == "br" || blockElements[n.Data]:
				b.WriteByte(' ')
				defer b.WriteByte(' ')
			}
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

// productURL is the canonical URL when it is on the site's domains, and
// otherwise the final URL after redirects.
func productURL(in PageInput) string {
	if href, ok := in.Doc.Find(`link[rel="canonical"]`).First().Attr("href"); ok {
		if abs := normalize.ResolveURL(in.FinalURL, href); abs != "" {
			if u, err := url.Parse(abs); err == nil && in.Site.HasDomain(u.Hostname()) {
				return abs
			}
		}
	}
	if in.FinalURL != nil {
		return in.FinalURL.String()
	}
	return ""
}

func bodySample(body []byte) *diagnose.HTMLSample {
	if len(body) == 0 {
		return nil
	}
	return diagnose.NewSample(diagnose.ReasonBodyStart, string(body))
}
