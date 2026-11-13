package extract

import (
	"fmt"
	"strings"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/normalize"
	"go-wordpress/internal/siteconfig"
)

// fromSearch builds the product from a matched search result. Only the
// title, URL and description can come from a search result; price is
// unknown and every other field is listed in unavailable_fields.
func fromSearch(site *siteconfig.Site, file string, sp *fetch.SearchPage) parsed {
	r := sp.Result
	prod := &Product{URL: r.URL, Price: Price{Currency: site.Locale.Currency(), State: PriceUnknown}}
	out := parsed{product: prod, fields: map[string]string{}}

	title, truncated := searchTitle(r.Title, site.SearchSettings().TitleStrip)
	prod.Title = title
	out.fields["title"] = diagnose.TruncateText(r.Title, 512)
	if desc := normalize.Text(r.Snippet); desc != "" {
		prod.Description = &desc
		out.fields["description"] = diagnose.TruncateText(r.Snippet, 512)
	}

	for _, rf := range site.Validation.RequiredFields {
		if fieldPresent(prod, rf) {
			continue
		}
		out.failure = diagnose.New(diagnose.RequiredFieldMissing, diagnose.ConfidenceHigh,
			"The %s result for this product (rank %d of query %q) has no usable %s.", sp.Engine, r.Rank, sp.Query, rf).
			WithMessage("The required field %s was not found.", rf).
			WithAction("A search result only provides title, url and description. Remove %s from validation.required_fields in configs/sites/%s, or use another engine for this site.", rf, file)
		return out
	}

	if truncated {
		out.warnings = append(out.warnings, Warning{Code: WarnSearchTitleTruncated, Field: siteconfig.FieldTitle,
			Message: "The search engine shortened the title. The full product title is longer."})
	}
	if prod.Description != nil {
		out.warnings = append(out.warnings, Warning{Code: WarnSearchSnippet, Field: siteconfig.FieldDescription,
			Message: "The description is the search result snippet. It may be shortened or out of date."})
	}
	out.warnings = append(out.warnings, Warning{Code: WarnPriceMissing, Field: FieldPrice,
		Message: fmt.Sprintf("Search results do not include prices (engine %s).", sp.Engine)})
	return out
}

// searchTitle removes the "..." a search engine adds to a shortened title,
// then the site's title_strip suffixes and prefixes. It reports whether the
// title was shortened.
func searchTitle(raw string, strip []string) (string, bool) {
	t := normalize.Text(raw)
	truncated := false
	for _, dots := range []string{"...", "…"} {
		if strings.HasSuffix(t, dots) {
			t, truncated = strings.TrimSpace(strings.TrimSuffix(t, dots)), true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, s := range strip {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if len(t) < len(s) {
				continue
			}
			switch {
			case strings.EqualFold(t[len(t)-len(s):], s):
				t, changed = strings.TrimSpace(t[:len(t)-len(s)]), true
			case strings.EqualFold(t[:len(s)], s):
				t, changed = strings.TrimSpace(t[len(s):]), true
			}
		}
	}
	return strings.TrimRight(t, " -|–,"), truncated
}
