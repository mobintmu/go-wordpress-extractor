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
	// The description is a search result snippet: shortened, maybe out of date.
	WarnSearchSnippet = "SEARCH_SNIPPET"
	// The search engine shortened the title.
	WarnSearchTitleTruncated = "SEARCH_TITLE_TRUNCATED"
)

// Completeness values.
const (
	CompletenessFull    = "full"
	CompletenessPartial = "partial"
)

// Reasons in unavailable_fields.
const (
	// The page was fetched, but the field was not on it (http, browser).
	ReasonNotOnPage = "NOT_ON_PAGE"
	// The search engine cannot provide the field (duckduckgo, brave).
	ReasonNotInSearchResults = "NOT_IN_SEARCH_RESULTS"
	// The site config does not extract the field for this site.
	ReasonNotConfigured = "NOT_CONFIGURED"
)

// Names used in unavailable_fields that are not product JSON fields.
const (
	FieldPrice        = "price"
	FieldAvailability = "availability"
	FieldSpecs        = "specs"
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
// status is always present. product, warnings, completeness and
// unavailable_fields are set on success; error, evidence, and partial are set
// on failed. A failed result that carries partial.product also has
// unavailable_fields. The HTTP status is 200 for both.
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
	// Which fetch engine produced the data. Set once a fetch was attempted.
	Source *Source `json:"source,omitempty"`
	// full when unavailable_fields is empty, partial otherwise. Set on success.
	Completeness string `json:"completeness,omitempty" enums:"full,partial" example:"partial"`
	// Set on success.
	Product *Product `json:"product,omitempty"`
	// Every product field that is not in the response, with the reason. Always an array on success; also set on failed when partial.product is.
	UnavailableFields []UnavailableField `json:"unavailable_fields,omitempty"`
	// Set on success; always an array.
	Warnings []Warning `json:"warnings,omitempty"`
	// Set on failed.
	Error *diagnose.Failure `json:"error,omitempty"`
	// Set on failed.
	Evidence *diagnose.Evidence `json:"evidence,omitempty"`
	// Set on failed.
	Partial *Partial `json:"partial,omitempty"`
} //@name ExtractResponse

// MarshalJSON always writes "warnings" and "unavailable_fields" as arrays
// on success, and omits warnings on failure. On failure unavailable_fields
// is written only when partial.product is set.
func (r Response) MarshalJSON() ([]byte, error) {
	type alias Response
	w := struct {
		alias
		Warnings          *[]Warning          `json:"warnings,omitempty"`
		UnavailableFields *[]UnavailableField `json:"unavailable_fields,omitempty"`
	}{alias: alias(r)}
	uf := r.UnavailableFields
	if uf == nil {
		uf = []UnavailableField{}
	}
	if r.Status == StatusSuccess {
		ws := r.Warnings
		if ws == nil {
			ws = []Warning{}
		}
		w.Warnings = &ws
		w.UnavailableFields = &uf
	} else if r.Partial != nil && r.Partial.Product != nil {
		w.UnavailableFields = &uf
	}
	return json.Marshal(w)
}

// Source says which fetch engine produced the data.
type Source struct {
	Engine string `json:"engine" binding:"required" enums:"http,browser,duckduckgo,brave" example:"duckduckgo"`
	// False for the search engines, which never contact the product's site.
	SiteContacted bool `json:"site_contacted" binding:"required" example:"false"`
	// The search query whose results held the product. Search engines only.
	Query string `json:"query,omitempty" example:"site:fergusonhome.com duravit 030549 0hole"`
	// The product's 1-based position in the search results. Search engines only.
	ResultRank int `json:"result_rank,omitempty" example:"1"`
	// True when the search answer came from the instance's cache.
	Cached bool `json:"cached,omitempty" example:"false"`
} //@name Source

// UnavailableField is one product field that is not in the response.
type UnavailableField struct {
	Field   string `json:"field" binding:"required" enums:"title,image_url,description,price,availability,specs" example:"price"`
	Reason  string `json:"reason" binding:"required" enums:"NOT_ON_PAGE,NOT_IN_SEARCH_RESULTS,NOT_CONFIGURED" example:"NOT_IN_SEARCH_RESULTS"`
	Message string `json:"message" binding:"required" example:"Search results do not include prices."`
} //@name UnavailableField

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
