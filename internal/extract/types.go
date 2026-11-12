// Package extract runs the stateless pipeline for one product page:
// config, fetch, page_check, extract, normalize, validate.
package extract

import (
	"encoding/json"
	"time"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/siteconfig"
)

// Result statuses.
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// Price states.
const (
	PriceAvailable    = "available"
	PriceCallForPrice = "call_for_price"
	PriceOutOfStock   = "out_of_stock"
	PriceUnknown      = "unknown"
)

// Output currencies. IRT amounts are toman; USD and EUR amounts are cents.
const (
	CurrencyIRT = siteconfig.CurrencyIRT
	CurrencyUSD = siteconfig.CurrencyUSD
	CurrencyEUR = siteconfig.CurrencyEUR
)

// Warning codes.
const (
	WarnFieldMissing = "FIELD_MISSING"
	WarnPriceMissing = "PRICE_MISSING"
)

// Request is the body of POST /api/v1/extract.
type Request struct {
	// One product page, on a domain with a site config.
	URL string `json:"url" binding:"required" example:"https://badomjip.com/product/example/" format:"uri"`
	// Echoed in the response and logs. Generated if missing.
	RequestID string `json:"request_id,omitempty" example:"client-generated-id-123" maxLength:"128"`
} //@name ExtractRequest

// Response is the single object returned for both results.
//
// status is always present. product and warnings are set on success;
// error, evidence, and partial are set on failed.
type Response struct {
	Status    string `json:"status" binding:"required" enums:"success,failed" example:"success"`
	RequestID string `json:"request_id" binding:"required" example:"client-generated-id-123"`
	URL       string `json:"url" binding:"required" format:"uri" example:"https://badomjip.com/product/example/"`
	// Missing when no site config matched.
	SiteID        string    `json:"site_id,omitempty" example:"badomjip"`
	ConfigVersion string    `json:"config_version,omitempty" example:"2026-11-08.1"`
	StartedAt     time.Time `json:"started_at" example:"2026-11-08T10:00:00Z"`
	FinishedAt    time.Time `json:"finished_at" example:"2026-11-08T10:00:03Z"`
	DurationMS    int64     `json:"duration_ms" binding:"required" example:"3120"`
	// Set on success.
	Product *Product `json:"product,omitempty"`
	// Set on success; always an array.
	Warnings []Warning `json:"warnings,omitempty"`
	// Set on failed.
	Error *diagnose.Failure `json:"error,omitempty"`
	// Set on failed.
	Evidence *diagnose.Evidence `json:"evidence,omitempty"`
	// Set on failed.
	Partial *Partial `json:"partial,omitempty"`
} //@name ExtractResponse

// MarshalJSON always writes "warnings" as an array on success, and omits
// it on failure.
func (r Response) MarshalJSON() ([]byte, error) {
	type alias Response
	w := struct {
		alias
		Warnings *[]Warning `json:"warnings,omitempty"`
	}{alias: alias(r)}
	if r.Status == StatusSuccess {
		ws := r.Warnings
		if ws == nil {
			ws = []Warning{}
		}
		w.Warnings = &ws
	}
	return json.Marshal(w)
}

// Product is the extracted product.
type Product struct {
	Title string `json:"title" example:"Example product"`
	// Canonical URL when it is on the site's domains, otherwise the final URL.
	URL         string  `json:"url" binding:"required" format:"uri" example:"https://badomjip.com/product/example/"`
	ImageURL    *string `json:"image_url" format:"uri" extensions:"x-nullable" example:"https://badomjip.com/wp-content/uploads/example.jpg"`
	Description *string `json:"description" extensions:"x-nullable" example:"First paragraph.\nSecond paragraph."`
	Price       Price   `json:"price"`
	// Name/value pairs from the page's specifications, in page order. Only
	// set when the site config defines pages.product.specs and the page has
	// at least one pair.
	Specs []Spec `json:"specs,omitempty"`
} //@name Product

// Spec is one name/value pair from the product's specifications.
type Spec struct {
	// The heading of the group the pair is in, when the page groups them.
	Group string `json:"group,omitempty" example:"Details"`
	Name  string `json:"name" binding:"required" example:"Finish"`
	Value string `json:"value" binding:"required" example:"Honed"`
} //@name Spec

// Price holds prices as integers in the unit named by Currency.
type Price struct {
	// Price in toman (IRT), US cents (USD), or euro cents (EUR). The lowest price when the page shows a range. Null when no price is available, never 0.
	Current *int64 `json:"current" extensions:"x-nullable" example:"400000"`
	// Regular price before any sale, in the same unit as current. The lowest value when the page shows a range.
	Regular *int64 `json:"regular" extensions:"x-nullable" example:"500000"`
	// IRT means the amounts are toman; USD and EUR mean the amounts are cents.
	Currency string `json:"currency" enums:"IRT,USD,EUR" example:"IRT"`
	// Text as it appeared on the page.
	Raw string `json:"raw" example:"500,000 تومان 400,000 تومان"`
	// out_of_stock means the product cannot be ordered.
	State string `json:"state" binding:"required" enums:"available,call_for_price,out_of_stock,unknown" example:"available"`
} //@name Price

// Warning notes a non-fatal problem in a successful result.
type Warning struct {
	Code    string `json:"code" binding:"required" example:"FIELD_MISSING"`
	Field   string `json:"field,omitempty" example:"image_url"`
	Message string `json:"message" binding:"required" example:"The image_url field was not found on this product page."`
} //@name Warning

// Partial carries what was found when extraction failed.
type Partial struct {
	Product *Product `json:"product" binding:"required" extensions:"x-nullable"`
} //@name Partial
