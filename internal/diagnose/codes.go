// Package diagnose turns fetch and parse problems into deterministic,
// structured failure reports with evidence.
package diagnose

import "fmt"

// Stage names where a failure can happen.
type Stage string

const (
	StageConfig    Stage = "config"
	StageFetch     Stage = "fetch"
	StagePageCheck Stage = "page_check"
	StageExtract   Stage = "extract"
	StageNormalize Stage = "normalize"
	StageValidate  Stage = "validate"
)

// Owner says who has to act on a failure.
type Owner string

const (
	OwnerClient     Owner = "client"
	OwnerSiteConfig Owner = "site_config"
	OwnerSourceSite Owner = "source_site"
	OwnerService    Owner = "service"
)

// Confidence levels.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
)

// Failure codes returned in failed results.
const (
	SiteNotConfigured    = "SITE_NOT_CONFIGURED"
	SiteDisabled         = "SITE_DISABLED"
	ConfigInvalid        = "CONFIG_INVALID"
	NotProductURL        = "NOT_PRODUCT_URL"
	URLNotAllowed        = "URL_NOT_ALLOWED"
	DNSFailed            = "DNS_FAILED"
	ConnectionFailed     = "CONNECTION_FAILED"
	TLSFailed            = "TLS_FAILED"
	FetchTimeout         = "FETCH_TIMEOUT"
	HTTPNotFound         = "HTTP_NOT_FOUND"
	AccessDenied         = "ACCESS_DENIED"
	RateLimited          = "RATE_LIMITED"
	SourceServerError    = "SOURCE_SERVER_ERROR"
	HTTPError            = "HTTP_ERROR"
	RobotsDisallowed     = "ROBOTS_DISALLOWED"
	RedirectNotAllowed   = "REDIRECT_NOT_ALLOWED"
	TooManyRedirects     = "TOO_MANY_REDIRECTS"
	UnsupportedContent   = "UNSUPPORTED_CONTENT"
	ResponseTooLarge     = "RESPONSE_TOO_LARGE"
	BotProtection        = "BOT_PROTECTION"
	JSRenderedPage       = "JS_RENDERED_PAGE"
	EmptyPage            = "EMPTY_PAGE"
	SelectorNotFound     = "SELECTOR_NOT_FOUND"
	RequiredFieldMissing = "REQUIRED_FIELD_MISSING"
	PriceParseFailed     = "PRICE_PARSE_FAILED"
	DeadlineExceeded     = "DEADLINE_EXCEEDED"
	InternalError        = "INTERNAL_ERROR"
	EngineUnavailable    = "ENGINE_UNAVAILABLE"

	// Search engines (duckduckgo, brave, mojeek, yahoo, yandex).
	SearchNoResult           = "SEARCH_NO_RESULT"
	SearchProviderBlocked    = "SEARCH_PROVIDER_BLOCKED"
	SearchQuotaExceeded      = "SEARCH_QUOTA_EXCEEDED"
	SearchProviderAuthFailed = "SEARCH_PROVIDER_AUTH_FAILED"
	SearchProviderError      = "SEARCH_PROVIDER_ERROR"

	// The AI engines ai-crawler-avalai and ai-crawler-cloudflare (an LLM
	// over an OpenAI-compatible API).
	AIUnavailable      = "AI_UNAVAILABLE"
	AIRateLimited      = "AI_RATE_LIMITED"
	AIQuotaExceeded    = "AI_QUOTA_EXCEEDED"
	AIAuthFailed       = "AI_AUTH_FAILED"
	AIInputTooLarge    = "AI_INPUT_TOO_LARGE"
	AIBadOutput        = "AI_BAD_OUTPUT"
	AIModelUnavailable = "AI_MODEL_UNAVAILABLE"
)

type codeInfo struct {
	stage     Stage
	owner     Owner
	retryable bool
	message   string
	action    string
}

// codes is the single table behind every failure. Stage "" means the
// stage is set by the caller (DEADLINE_EXCEEDED, INTERNAL_ERROR).
var codes = map[string]codeInfo{
	SiteNotConfigured:        {StageConfig, OwnerSiteConfig, false, "No site config exists for this host.", "Add a site config under configs/sites/ for this host, or send a URL from a configured site."},
	SiteDisabled:             {StageConfig, OwnerSiteConfig, false, "The site config for this host is disabled.", "Set enabled to true in the site config when the site should be served again."},
	ConfigInvalid:            {StageConfig, OwnerSiteConfig, false, "The site config failed validation.", "Fix the site config file and run sitectl validate."},
	NotProductURL:            {StageConfig, OwnerClient, false, "The URL is not a product page for this site.", "Send only product page URLs, or update pages.product.match in the site config."},
	URLNotAllowed:            {StageFetch, OwnerClient, false, "The URL points to an address the service may not contact.", "Send a URL on a public host and a standard port."},
	DNSFailed:                {StageFetch, OwnerSourceSite, true, "The host name did not resolve.", "Check the domain; retry later if the site's DNS is temporarily down."},
	ConnectionFailed:         {StageFetch, OwnerSourceSite, true, "The connection to the site failed.", "Retry later; the site may be down."},
	TLSFailed:                {StageFetch, OwnerSourceSite, false, "The TLS handshake with the site failed.", "Check the site's certificate; the site owner must fix it."},
	FetchTimeout:             {StageFetch, OwnerSourceSite, true, "The site did not respond in time.", "Retry later, or raise fetch.timeout_seconds in the site config."},
	HTTPNotFound:             {StageFetch, OwnerClient, false, "The site returned HTTP 404 or 410. The product is gone.", "Mark the product as removed."},
	AccessDenied:             {StageFetch, OwnerSourceSite, false, "The site refused access.", "Ask the site owner for access, or set fetch.engine to a search engine (\"brave\", \"duckduckgo\", \"mojeek\", \"yahoo\" or \"yandex\") in the site config to return the data available from search results."},
	RateLimited:              {StageFetch, OwnerSourceSite, true, "The site rate-limited the request.", "Retry later, or raise fetch.delay_ms in the site config."},
	SourceServerError:        {StageFetch, OwnerSourceSite, true, "The site returned a server error.", "Retry later."},
	HTTPError:                {StageFetch, OwnerSourceSite, false, "The site returned an unexpected HTTP status.", "Open the URL in a browser to see what the site returns."},
	RobotsDisallowed:         {StageFetch, OwnerSiteConfig, false, "The site's robots.txt disallows this URL.", "Respect the site's robots.txt, or set fetch.respect_robots_txt to false only with the site owner's permission."},
	RedirectNotAllowed:       {StageFetch, OwnerSiteConfig, false, "The site redirected outside its configured domains.", "Add the target host to domains in the site config if it belongs to the same site."},
	TooManyRedirects:         {StageFetch, OwnerSourceSite, false, "The site redirected too many times.", "The site has a redirect loop; retry later."},
	UnsupportedContent:       {StageFetch, OwnerSourceSite, false, "The response is not HTML.", "Send a URL that serves an HTML product page."},
	ResponseTooLarge:         {StageFetch, OwnerService, false, "The response exceeded the size limit.", "The page is larger than the service accepts."},
	BotProtection:            {StagePageCheck, OwnerSourceSite, false, "The target site returned an anti-bot challenge.", "Ask the site owner for access, or set fetch.engine to a search engine (\"brave\", \"duckduckgo\", \"mojeek\", \"yahoo\" or \"yandex\") in the site config to return the data available from search results."},
	JSRenderedPage:           {StagePageCheck, OwnerSiteConfig, false, "The page is rendered by JavaScript and has no product markup.", "Set fetch.engine to \"browser\" in the site config so the page is rendered in Chrome."},
	EmptyPage:                {StagePageCheck, OwnerSourceSite, true, "The page is empty.", "Retry later."},
	SelectorNotFound:         {StageExtract, OwnerSiteConfig, false, "The product block was not found on the page.", "Update pages.product.root in the site config."},
	RequiredFieldMissing:     {StageValidate, OwnerSiteConfig, false, "A required field was not found.", "Update the field's selector in the site config."},
	PriceParseFailed:         {StageNormalize, OwnerSiteConfig, false, "Price text was found but could not be parsed.", "Update the price selectors or locale words in the site config."},
	EngineUnavailable:        {StageFetch, OwnerService, true, "The fetch engine the site config selects is not available on this instance.", "Enable the engine on this instance (APP_BROWSER_ENABLED=true for \"browser\", APP_DUCKDUCKGO_ENABLED=true for \"duckduckgo\", APP_BRAVE_ENABLED=true with APP_BRAVE_API_KEY for \"brave\", APP_MOJEEK_ENABLED, APP_YAHOO_ENABLED or APP_YANDEX_ENABLED=true for \"mojeek\", \"yahoo\" or \"yandex\"), or retry on an instance that has it."},
	SearchNoResult:           {StageFetch, OwnerSourceSite, false, "The search engine has no result for this product URL.", "Retry in a few days, when the page may be indexed."},
	SearchProviderBlocked:    {StageFetch, OwnerService, true, "The search provider answered with a rate-limit or challenge page.", "Retry later. Raise the engine's minimum interval (APP_DUCKDUCKGO_MIN_INTERVAL_MS, APP_MOJEEK_MIN_INTERVAL_MS, APP_YAHOO_MIN_INTERVAL_MS or APP_YANDEX_MIN_INTERVAL_MS) if this happens often."},
	SearchQuotaExceeded:      {StageFetch, OwnerService, true, "The search provider's rate limit or quota is used up.", "Retry later, raise APP_BRAVE_MIN_INTERVAL_MS, or move to a larger Brave plan."},
	SearchProviderAuthFailed: {StageFetch, OwnerService, false, "The search provider refused the API key.", "Check APP_BRAVE_API_KEY and the plan in the Brave Search API dashboard."},
	SearchProviderError:      {StageFetch, OwnerService, true, "The search provider failed.", "Retry. If it persists for duckduckgo, mojeek, yahoo or yandex, the results page layout may have changed."},
	AIUnavailable:            {StageExtract, OwnerService, true, "The AI provider failed or did not answer in time.", "Retry later. If it persists, check the provider's status and APP_AI_CRAWLER_BASE_URL (AvalAI) or APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT and APP_AI_CRAWLER_CLOUDFLARE_BASE_URL (Cloudflare)."},
	AIRateLimited:            {StageExtract, OwnerService, true, "The AI provider's rate limit was reached.", "Retry later, raise APP_AI_CRAWLER_MIN_INTERVAL_MS (AvalAI) or APP_AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS (Cloudflare), or move the AI account to a higher tier."},
	AIQuotaExceeded:          {StageExtract, OwnerService, false, "The AI provider account has no quota or credit left.", "Top up the AI provider account (for Cloudflare: the Unified Billing credits, or the provider account behind the BYOK key)."},
	AIAuthFailed:             {StageExtract, OwnerService, false, "The AI provider refused the API key or the model.", "AvalAI: check APP_AI_CRAWLER_API_KEY (or APP_AI_CRAWLER_API_KEY_FILE), APP_AI_CRAWLER_MODEL and the account's tier. Cloudflare: check APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN (it needs the Workers AI Read and AI Gateway Run permissions) and the gateway id; with APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING=true, store the provider's key in the gateway (BYOK)."},
	AIInputTooLarge:          {StageExtract, OwnerService, false, "The page is too large for the AI model.", "Lower APP_AI_CRAWLER_MAX_INPUT_CHARS."},
	AIModelUnavailable:       {StageExtract, OwnerService, false, "The AI provider does not offer the configured model on this account, route or region.", "Copy the exact model id from the provider's catalog into APP_AI_CRAWLER_MODEL (AvalAI) or APP_AI_CRAWLER_CLOUDFLARE_MODEL (Cloudflare), and check that the account may use it."},
	AIBadOutput:              {StageExtract, OwnerService, true, "The AI model did not return usable product JSON.", "Retry; if it persists for this site, add a site config with selectors under configs/sites/."},
	DeadlineExceeded:         {"", OwnerService, true, "The request reached its time limit.", "Retry later."},
	InternalError:            {"", OwnerService, true, "An unexpected error occurred in the service.", "Retry; report the request_id if it persists."},
}

// Failure is the error part of a failed result.
type Failure struct {
	// Failure code. New codes can be added over time, so treat unknown codes like INTERNAL_ERROR.
	// Known codes: SITE_NOT_CONFIGURED, SITE_DISABLED, CONFIG_INVALID, NOT_PRODUCT_URL, URL_NOT_ALLOWED,
	// DNS_FAILED, CONNECTION_FAILED, TLS_FAILED, FETCH_TIMEOUT, HTTP_NOT_FOUND, ACCESS_DENIED, RATE_LIMITED,
	// SOURCE_SERVER_ERROR, HTTP_ERROR, ROBOTS_DISALLOWED, REDIRECT_NOT_ALLOWED, TOO_MANY_REDIRECTS,
	// UNSUPPORTED_CONTENT, RESPONSE_TOO_LARGE, BOT_PROTECTION, JS_RENDERED_PAGE, EMPTY_PAGE, SELECTOR_NOT_FOUND,
	// REQUIRED_FIELD_MISSING, PRICE_PARSE_FAILED, DEADLINE_EXCEEDED, INTERNAL_ERROR, ENGINE_UNAVAILABLE,
	// SEARCH_NO_RESULT, SEARCH_PROVIDER_BLOCKED, SEARCH_QUOTA_EXCEEDED, SEARCH_PROVIDER_AUTH_FAILED, SEARCH_PROVIDER_ERROR,
	// AI_UNAVAILABLE, AI_RATE_LIMITED, AI_QUOTA_EXCEEDED, AI_AUTH_FAILED, AI_INPUT_TOO_LARGE, AI_BAD_OUTPUT, AI_MODEL_UNAVAILABLE.
	Code            string `json:"code" binding:"required" example:"SELECTOR_NOT_FOUND"`
	Stage           Stage  `json:"stage" binding:"required" enums:"config,fetch,page_check,extract,normalize,validate" swaggertype:"string" example:"extract"`
	Owner           Owner  `json:"owner" binding:"required" enums:"client,site_config,source_site,service" swaggertype:"string" example:"site_config"`
	Retryable       bool   `json:"retryable" binding:"required" example:"false"`
	Confidence      string `json:"confidence" binding:"required" enums:"high,medium" example:"high"`
	Message         string `json:"message" binding:"required" example:"The product block was not found on the page."`
	RootCause       string `json:"root_cause" binding:"required" example:"The page returned HTTP 200 with real content, but selector 'div.product.type-product' matched 0 elements. The site markup has probably changed."`
	SuggestedAction string `json:"suggested_action,omitempty" example:"Update pages.product.root in configs/sites/badomjip.com.json."`
} //@name ExtractError

func (f *Failure) Error() string { return f.Code + ": " + f.RootCause }

// New builds a failure for code from the code table. The root cause is
// formatted from rootCause and args.
func New(code, confidence, rootCause string, args ...any) *Failure {
	info, ok := codes[code]
	if !ok {
		info = codes[InternalError]
	}
	return &Failure{
		Code:            code,
		Stage:           info.stage,
		Owner:           info.owner,
		Retryable:       info.retryable,
		Confidence:      confidence,
		Message:         info.message,
		RootCause:       fmt.Sprintf(rootCause, args...),
		SuggestedAction: info.action,
	}
}

// At sets the stage for codes whose stage depends on where they happened.
func (f *Failure) At(stage Stage) *Failure {
	if f.Stage == "" {
		f.Stage = stage
	}
	return f
}

// WithAction replaces the suggested action.
func (f *Failure) WithAction(format string, args ...any) *Failure {
	f.SuggestedAction = fmt.Sprintf(format, args...)
	return f
}

// WithMessage replaces the short message.
func (f *Failure) WithMessage(format string, args ...any) *Failure {
	f.Message = fmt.Sprintf(format, args...)
	return f
}

// Codes returns every known code.
func Codes() []string {
	out := make([]string, 0, len(codes))
	for c := range codes {
		out = append(out, c)
	}
	return out
}
