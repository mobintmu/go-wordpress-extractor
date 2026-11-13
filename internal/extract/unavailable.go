package extract

import (
	"fmt"

	"go-wordpress/internal/siteconfig"
)

// unavailableFields lists every product field that is not in prod, with
// the reason, for any engine. The same fields are checked for every site:
// title (only when missing), image_url, description, price, availability and
// specs. A field the site config does not extract is NOT_CONFIGURED; a
// configured field that is missing is NOT_ON_PAGE for a fetched page and
// NOT_IN_SEARCH_RESULTS for a search result.
func unavailableFields(site *siteconfig.Site, prod *Product, fromSearch bool) []UnavailableField {
	out := []UnavailableField{}
	if prod == nil {
		return out
	}
	p := site.Pages.Product
	_, hasTitle := p.Fields[siteconfig.FieldTitle]
	_, hasImage := p.Fields[siteconfig.FieldImageURL]
	_, hasDesc := p.Fields[siteconfig.FieldDescription]
	_, hasCur := p.Fields[siteconfig.FieldPriceCurrent]
	_, hasReg := p.Fields[siteconfig.FieldPriceRegular]
	hasAvail := p.Availability.OutOfStock != "" || p.Availability.OrderButton != ""

	add := func(field string, configured bool) {
		reason := ReasonNotOnPage
		switch {
		case !configured:
			reason = ReasonNotConfigured
		case fromSearch:
			reason = ReasonNotInSearchResults
		}
		out = append(out, UnavailableField{Field: field, Reason: reason, Message: unavailableMessage(field, reason)})
	}

	if prod.Title == "" {
		add(siteconfig.FieldTitle, hasTitle || fromSearch)
	}
	if prod.ImageURL == nil {
		add(siteconfig.FieldImageURL, hasImage)
	}
	if prod.Description == nil {
		add(siteconfig.FieldDescription, hasDesc || fromSearch)
	}
	if prod.Price.Current == nil && prod.Price.State != PriceCallForPrice {
		add(FieldPrice, hasCur || hasReg)
	}
	// Availability is decided from the page whenever it is configured, so
	// a fetched page never lacks it; a search result never has it.
	if !hasAvail || fromSearch {
		add(FieldAvailability, hasAvail)
	}
	if len(prod.Specs) == 0 {
		add(FieldSpecs, p.Specs != nil)
	}
	return out
}

func unavailableMessage(field, reason string) string {
	name := map[string]string{
		siteconfig.FieldTitle:       "title",
		siteconfig.FieldImageURL:    "product image",
		siteconfig.FieldDescription: "description",
		FieldPrice:                  "price",
		FieldAvailability:           "stock status",
		FieldSpecs:                  "specifications",
	}[field]
	switch reason {
	case ReasonNotConfigured:
		return fmt.Sprintf("The site config does not extract the %s for this site.", name)
	case ReasonNotInSearchResults:
		return fmt.Sprintf("The search result does not include the %s.", name)
	default:
		return fmt.Sprintf("The %s was not found on this product page.", name)
	}
}

// completeness is full when nothing is missing.
func completeness(uf []UnavailableField) string {
	if len(uf) == 0 {
		return CompletenessFull
	}
	return CompletenessPartial
}
