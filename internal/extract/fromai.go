package extract

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"go-wordpress/internal/aicrawler"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/normalize"
	"go-wordpress/internal/siteconfig"
)

// Limits on what the AI crawler may return.
const (
	aiMaxTitle       = 300
	aiMaxDescription = 5000
	aiMaxSpecs       = 200
	aiMaxFeatures    = 50
	aiMaxItemChars   = 500
	// aiMinDescOverlap is the share of the description's words that must
	// be on the page.
	aiMinDescOverlap = 0.6
)

// fromAI builds the product from the AI crawler's answer. Every value is
// checked against the page first (grounding): a value that is not on the
// page is dropped and listed with reason AI_NOT_GROUNDED, so the model can
// not invent a name, price or image. Prices are parsed by the Go price
// parser from the text the model copied, never computed by the model.
func fromAI(site *siteconfig.Site, file string, page *fetch.Page, target *url.URL) parsed {
	ai := page.AI
	p, clean := ai.Product, ai.Clean
	final := page.FinalURL
	if final == nil {
		final = target
	}
	out := parsed{fields: map[string]string{}, dropped: map[string]bool{}}
	if !p.IsProduct {
		out.failure = diagnose.New(diagnose.NotProductURL, diagnose.ConfidenceMedium,
			"The AI crawler (model %s) read %s and found that it is not a single product page.", ai.Model, final.String()).
			WithAction("Send only product page URLs.")
		return out
	}

	prod := &Product{URL: aiProductURL(site, final, aicrawler.Str(p.URL)), Price: Price{State: PriceUnknown}}
	out.product = prod
	sample := func(field, v string) {
		if v != "" {
			out.fields[field] = diagnose.TruncateText(v, 512)
		}
	}

	// title
	if name := normalize.Text(aicrawler.Str(p.Name)); name != "" {
		sample(siteconfig.FieldTitle, name)
		if utf8.RuneCountInString(name) <= aiMaxTitle && clean.ContainsText(name) {
			prod.Title = name
		} else {
			out.dropped[siteconfig.FieldTitle] = true
		}
	}

	// image_url
	if img := strings.TrimSpace(aicrawler.Str(p.Image)); img != "" {
		sample(siteconfig.FieldImageURL, img)
		if abs := normalize.ResolveURL(final, img); abs != "" && clean.HasURL(abs) {
			prod.ImageURL = &abs
		} else {
			out.dropped[siteconfig.FieldImageURL] = true
		}
	}

	// description
	if desc := strings.TrimSpace(aicrawler.Str(p.Description)); desc != "" {
		sample(siteconfig.FieldDescription, desc)
		desc = normalizeParagraphs(desc)
		if utf8.RuneCountInString(desc) > aiMaxDescription {
			desc = strings.TrimSpace(string([]rune(desc)[:aiMaxDescription]))
		}
		if clean.WordOverlap(desc) >= aiMinDescOverlap {
			prod.Description = &desc
		} else {
			out.dropped[siteconfig.FieldDescription] = true
		}
	}

	// price
	aiPrice(site, p, clean, prod, &out)

	// specs
	for _, sp := range p.Specs {
		if len(prod.Specs) >= aiMaxSpecs {
			break
		}
		name, value := specName(normalize.Text(sp.Name)), normalize.Text(sp.Value)
		if name == "" || value == "" || len(name) > aiMaxItemChars || len(value) > aiMaxItemChars {
			continue
		}
		if !clean.ContainsText(name) || !clean.ContainsText(value) {
			out.dropped[FieldSpecs] = true
			continue
		}
		s := Spec{Name: name, Value: value}
		if g := normalize.Text(aicrawler.Str(sp.Group)); g != "" && clean.ContainsText(g) {
			s.Group = g
		}
		prod.Specs = append(prod.Specs, s)
	}
	if len(prod.Specs) > 0 {
		delete(out.dropped, FieldSpecs)
	}

	// features
	for _, f := range p.Features {
		if len(prod.Features) >= aiMaxFeatures {
			break
		}
		f = normalize.Text(f)
		if f == "" || len(f) > aiMaxItemChars || slices.Contains(prod.Features, f) {
			continue
		}
		if clean.ContainsText(f) {
			prod.Features = append(prod.Features, f)
		}
	}

	// validate
	for _, rf := range site.Validation.RequiredFields {
		if fieldPresent(prod, rf) {
			continue
		}
		cause := fmt.Sprintf("The AI crawler (model %s) found no %s on the page.", ai.Model, rf)
		if out.dropped[rf] {
			cause = fmt.Sprintf("The AI crawler (model %s) returned a %s that does not appear on the page, so it was dropped.", ai.Model, rf)
		}
		out.failure = diagnose.New(diagnose.RequiredFieldMissing, diagnose.ConfidenceMedium, "%s", cause).
			WithMessage("The required field %s was not found.", rf).
			WithAction("Check the page in a browser. If the field is there, add a site config with selectors under configs/sites/ for this host.")
		return out
	}

	// warnings
	out.warnings = append(out.warnings, Warning{Code: WarnAIExtracted,
		Message: fmt.Sprintf("Values were extracted by an LLM (%s) and checked against the page.", ai.Model)})
	for _, f := range []string{siteconfig.FieldTitle, siteconfig.FieldImageURL, siteconfig.FieldDescription, FieldPrice, FieldSpecs} {
		if out.dropped[f] {
			out.warnings = append(out.warnings, Warning{Code: WarnAINotGrounded, Field: f,
				Message: fmt.Sprintf("The AI crawler returned a %s that is not on the page, so it was left out.", unavailableName(f))})
		}
	}
	if prod.Price.State == PriceUnknown {
		out.warnings = append(out.warnings, Warning{Code: WarnPriceMissing, Field: FieldPrice,
			Message: "No price, call-for-price text, or stock signal was found on this product page."})
	}
	return out
}

// aiPrice fills prod.Price from the model's price texts. The currency comes
// from the site's price_unit, or for "auto" from the model's currency code
// and the price text. Each amount must appear on the page.
func aiPrice(site *siteconfig.Site, p *aicrawler.Product, clean *aicrawler.Page, prod *Product, out *parsed) {
	curText := normalize.Text(aicrawler.Str(p.Price.CurrentText))
	regText := normalize.Text(aicrawler.Str(p.Price.RegularText))
	if curText != "" || regText != "" {
		out.fields["price_area"] = diagnose.TruncateText(strings.TrimSpace(regText+" "+curText), 512)
	}
	unit := aiUnit(site.Locale.PriceUnit, aicrawler.Str(p.Price.Currency), curText+" "+regText)
	prod.Price.Currency = siteconfig.Locale{PriceUnit: unit}.Currency()
	if unit == "" {
		// No currency could be determined, so no amount can be trusted.
		prod.Price.Currency = siteconfig.CurrencyUSD
		if curText != "" || regText != "" {
			out.dropped[FieldPrice] = true
		}
		curText, regText = "", ""
	}

	amounts := clean.Amounts(unit)
	grounded := func(text string) *int64 {
		if text == "" {
			return nil
		}
		v, err := parsePrice(text, unit)
		if err != nil || v == nil {
			out.dropped[FieldPrice] = true
			return nil
		}
		if !slices.Contains(amounts, *v) {
			out.dropped[FieldPrice] = true
			return nil
		}
		return v
	}
	cur, reg := grounded(curText), grounded(regText)
	if cur == nil && reg != nil {
		cur, reg, curText, regText = reg, nil, regText, ""
	}
	if reg != nil && (*reg == *cur || *reg < *cur) {
		reg, regText = nil, ""
	}
	if cur != nil {
		delete(out.dropped, FieldPrice)
	}
	prod.Price.Current, prod.Price.Regular = cur, reg
	if cur != nil || reg != nil {
		raw := []string{}
		if reg != nil {
			raw = append(raw, regText)
		}
		raw = append(raw, curText)
		prod.Price.Raw = strings.Join(raw, " ")
	}

	switch p.Price.State {
	case PriceCallForPrice:
		if cur == nil {
			prod.Price.State = PriceCallForPrice
			delete(out.dropped, FieldPrice)
		}
	case PriceOutOfStock:
		prod.Price.State = PriceOutOfStock
	}
	if prod.Price.State == PriceUnknown && cur != nil {
		prod.Price.State = PriceAvailable
	}
}

// aiUnit is the site's price unit, or for "auto" the unit the model's
// currency code or the price text names. "" when it can not be told.
func aiUnit(siteUnit, code, text string) string {
	if siteUnit != "" && siteUnit != siteconfig.UnitAuto {
		return siteUnit
	}
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "USD":
		return siteconfig.UnitUSD
	case "EUR":
		return siteconfig.UnitEUR
	case "IRR":
		return siteconfig.UnitRial
	case "IRT", "TOMAN":
		return siteconfig.UnitToman
	}
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "$") || strings.Contains(t, "usd"):
		return siteconfig.UnitUSD
	case strings.Contains(t, "€") || strings.Contains(t, "eur"):
		return siteconfig.UnitEUR
	case strings.Contains(t, "تومان") || strings.Contains(t, "toman"):
		return siteconfig.UnitToman
	case strings.Contains(t, "ریال") || strings.Contains(t, "rial"):
		return siteconfig.UnitRial
	}
	return ""
}

// aiProductURL is the model's URL when it is on the requested host (with or
// without www.), otherwise the final URL after redirects.
func aiProductURL(site *siteconfig.Site, final *url.URL, candidate string) string {
	if abs := normalize.ResolveURL(final, candidate); abs != "" {
		if u, err := url.Parse(abs); err == nil {
			h := siteconfig.NormalizeHost(u.Hostname())
			if site.HasDomain(h) || slices.Contains(siteconfig.LearnedDomains(final.Hostname()), h) {
				return abs
			}
		}
	}
	return final.String()
}

// normalizeParagraphs cleans each line and keeps line breaks between them.
func normalizeParagraphs(s string) string {
	return normalize.Paragraphs(strings.Split(s, "\n"), "\n")
}

// aiUnavailable lists the fields missing from an AI result. A field the
// model returned but that is not on the page is AI_NOT_GROUNDED; a field
// the model did not find is NOT_ON_PAGE.
func aiUnavailable(prod *Product, dropped map[string]bool) []UnavailableField {
	out := []UnavailableField{}
	if prod == nil {
		return out
	}
	add := func(field string) {
		reason := ReasonNotOnPage
		if dropped[field] {
			reason = ReasonAINotGrounded
		}
		out = append(out, UnavailableField{Field: field, Reason: reason, Message: unavailableMessage(field, reason)})
	}
	if prod.Title == "" {
		add(siteconfig.FieldTitle)
	}
	if prod.ImageURL == nil {
		add(siteconfig.FieldImageURL)
	}
	if prod.Description == nil {
		add(siteconfig.FieldDescription)
	}
	if prod.Price.Current == nil && prod.Price.State != PriceCallForPrice {
		add(FieldPrice)
	}
	if prod.Price.State == PriceUnknown {
		add(FieldAvailability)
	}
	if len(prod.Specs) == 0 {
		add(FieldSpecs)
	}
	return out
}
