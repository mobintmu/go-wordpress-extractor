package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"

	"go.uber.org/zap"
)

// Config holds every setting the service reads from the environment.
// All variables use the APP_ prefix, for example APP_HTTP_PORT.
type Config struct {
	ENV         string
	HTTPPort    int
	HTTPAddress string

	SiteConfigDir string
	APIKeysFile   string
	APIKeyPepper  string

	OpsUser         string
	OpsPasswordHash string

	TLSCertFile string
	TLSKeyFile  string

	MaxRequestDuration    time.Duration
	SiteWaitTimeout       time.Duration
	MaxConcurrentRequests int
	MaxConcurrentPerSite  int

	// ProxyHost and ProxyPort name an HTTP proxy that every outbound page
	// fetch goes through. When ProxyHost is empty, pages are fetched directly.
	ProxyHost string
	ProxyPort int

	// ReloadInterval is how often site configs and the API key file are
	// checked for changes.
	ReloadInterval time.Duration

	// BrowserEnabled turns on the "browser" fetch engine (real Chrome via
	// Playwright) for sites whose config sets fetch.engine to "browser".
	BrowserEnabled bool
	// BrowserChannel is "chromium" (Playwright's bundled build) or "chrome"
	// (the installed Google Chrome).
	BrowserChannel string
	// BrowserHeadless runs Chrome without a window.
	BrowserHeadless bool
	// BrowserMaxPages caps the browser pages open at once on this instance.
	BrowserMaxPages int

	// Search engines ("duckduckgo", "brave"). They never contact the
	// product's site; they build the product from search results.

	// SearchUserAgent is the honest client name sent to search providers.
	SearchUserAgent string
	// SearchCacheTTL is how long a matched search result is kept in memory.
	SearchCacheTTL time.Duration
	// SearchNoResultCacheTTL is how long a SEARCH_NO_RESULT answer is kept.
	SearchNoResultCacheTTL time.Duration

	// DuckDuckGoEnabled turns on the "duckduckgo" engine. It needs no key.
	DuckDuckGoEnabled bool
	// DuckDuckGoMinIntervalMS is the minimum gap between two DuckDuckGo
	// queries from this instance.
	DuckDuckGoMinIntervalMS int

	// BraveEnabled turns on the "brave" engine. It needs an API key.
	BraveEnabled bool
	// BraveAPIKey is the Brave Search API key. Secret: never logged.
	BraveAPIKey string
	// BraveAPIKeyFile holds only the key; read when BraveAPIKey is empty.
	BraveAPIKeyFile string
	// BraveMinIntervalMS is the minimum gap between two Brave queries.
	BraveMinIntervalMS int

	// MojeekEnabled, YahooEnabled and YandexEnabled turn on the "mojeek",
	// "yahoo" and "yandex" engines, which read that search engine's results
	// page through github.com/jcalvert/metawebsearch. They need no key.
	MojeekEnabled bool
	// MojeekMinIntervalMS is the minimum gap between two Mojeek queries.
	MojeekMinIntervalMS int
	YahooEnabled        bool
	YahooMinIntervalMS  int
	YandexEnabled       bool
	YandexMinIntervalMS int

	// DefaultEngine is the engine for any website without a rule of its
	// own (no site config and no active learned config):
	// ai-crawler-avalai, ai-crawler-cloudflare, or none (SITE_NOT_CONFIGURED).
	// Read from APP_DEFAULT_ENGINE; when that is empty it is
	// ai-crawler-avalai with the AI crawler on and none with it off.
	DefaultEngine string
	// legacyDefaultRule is APP_AI_CRAWLER_DEFAULT_RULE, which was replaced
	// by APP_DEFAULT_ENGINE: the server refuses to start while it is set.
	legacyDefaultRule string

	// AI crawler: the AI engines extract products with an LLM over an
	// OpenAI-compatible API. "ai-crawler-avalai" calls AvalAI and
	// "ai-crawler-cloudflare" calls Cloudflare AI Gateway; the limits, cache,
	// format and learning settings below are shared.

	// AICrawlerEnabled turns the AI engines on. Each engine is on when its
	// own settings are complete: AvalAI needs a key, Cloudflare an account
	// id and a token.
	AICrawlerEnabled bool
	// AICrawlerBaseURL is AvalAI's OpenAI-compatible API base, ending in /v1.
	AICrawlerBaseURL string
	// AICrawlerModel is the only AvalAI model used; there is no fallback model.
	AICrawlerModel string
	// AICrawlerAPIKey is AvalAI's API key. Secret: never logged.
	AICrawlerAPIKey string
	// AICrawlerAPIKeyFile holds only the key; read when AICrawlerAPIKey is empty.
	AICrawlerAPIKeyFile string
	// AICrawlerSearchModel is the AvalAI model that searches the web
	// itself, used by sites with fetch.ai_fetch "web_search". "none" (or
	// empty after validation) turns that mode off.
	AICrawlerSearchModel string
	// AICrawlerTimeout bounds one LLM call (each retry gets its own).
	AICrawlerTimeout time.Duration
	// AICrawlerMaxInputChars caps the cleaned page sent to the model.
	AICrawlerMaxInputChars int
	// AICrawlerMaxOutputTokens caps the model's answer.
	AICrawlerMaxOutputTokens int
	// AICrawlerTemperature is the sampling temperature, 0 for stable answers.
	AICrawlerTemperature float64
	// AICrawlerRetries is how often a 408, 409, 429 or 5xx answer is retried.
	AICrawlerRetries int
	// AICrawlerMinIntervalMS is the minimum gap between two AvalAI calls.
	AICrawlerMinIntervalMS int
	// AICrawlerCacheTTL keeps an extraction for the same page and model.
	AICrawlerCacheTTL time.Duration
	// AICrawlerResponseFormat is "json_object" or "json_schema".
	AICrawlerResponseFormat string
	// AICrawlerDefaultDelayMS is the gap between two fetches of the same
	// host under the default rule.
	AICrawlerDefaultDelayMS int

	// AICrawlerLearnSelectors turns on selector learning.
	AICrawlerLearnSelectors bool
	// AICrawlerLearnDir holds learned configs. Empty means <SITE_CONFIG_DIR>/learned.
	AICrawlerLearnDir string
	// AICrawlerLearnPromoteAfter is how many different URLs must match in
	// shadow before a learned config becomes active.
	AICrawlerLearnPromoteAfter int
	// AICrawlerLearnDemoteAfter is how many failures in a row demote an
	// active learned config.
	AICrawlerLearnDemoteAfter int
	// AICrawlerLearnMaxAttemptsPerDay caps learning calls per host per day.
	AICrawlerLearnMaxAttemptsPerDay int

	// Engine "ai-crawler-cloudflare" (Cloudflare AI Gateway). It is
	// configured when the AI crawler is on and the account id is set.

	// CloudflareAccountID is the Cloudflare account id (32 hex characters).
	CloudflareAccountID string
	// CloudflareGatewayID names the gateway; "default" is created on first use.
	CloudflareGatewayID string
	// CloudflareEndpoint is "rest" (recommended) or "compat".
	CloudflareEndpoint string
	// CloudflareBaseURL overrides the URL built from the endpoint. Optional.
	CloudflareBaseURL string
	// CloudflareModel is the model id from Cloudflare's catalog.
	CloudflareModel string
	// CloudflareReasoningEffort is sent as reasoning_effort: none, low,
	// high or max.
	CloudflareReasoningEffort string
	// CloudflareAPIToken is the Cloudflare API token (rest) or the gateway
	// token (compat). Secret: never logged.
	CloudflareAPIToken string
	// CloudflareAPITokenFile holds only the token; read when the token is empty.
	CloudflareAPITokenFile string
	// CloudflareProviderAPIKey is the provider's own key, compat only. Secret.
	CloudflareProviderAPIKey string
	// CloudflareProviderAPIKeyFile holds only that key.
	CloudflareProviderAPIKeyFile string
	// CloudflareNoUnifiedBilling sends cf-aig-no-wholesale, so a call
	// without a usable key fails instead of spending credits.
	CloudflareNoUnifiedBilling bool
	// CloudflareCollectLog false asks the gateway not to log prompts and answers.
	CloudflareCollectLog bool
	// CloudflareMinIntervalMS is the minimum gap between two Cloudflare calls.
	CloudflareMinIntervalMS int

	// Set by ValidateAICrawler: which AI engines have complete settings.
	avalaiConfigured     bool
	cloudflareConfigured bool
}

// Engine names accepted by APP_DEFAULT_ENGINE. They match siteconfig's
// engine names; config does not import siteconfig.
const (
	EngineNone         = "none"
	EngineAvalAI       = "ai-crawler-avalai"
	EngineAICloudflare = "ai-crawler-cloudflare"
)

// AvalAIConfigured reports whether the ai-crawler-avalai engine is on.
func (cfg *Config) AvalAIConfigured() bool { return cfg.AICrawlerEnabled && cfg.avalaiConfigured }

// CloudflareConfigured reports whether the ai-crawler-cloudflare engine is on.
func (cfg *Config) CloudflareConfigured() bool {
	return cfg.AICrawlerEnabled && cfg.cloudflareConfigured
}

// DefaultAIEngine is the engine of the default rule, or "" when websites
// without a rule are not served (APP_DEFAULT_ENGINE=none).
func (cfg *Config) DefaultAIEngine() string {
	if cfg.DefaultEngine == EngineNone || !cfg.AICrawlerEnabled {
		return ""
	}
	return cfg.DefaultEngine
}

// AIModel is the model the AI engine uses, or "" when it is not on.
func (cfg *Config) AIModel(engine string) string {
	switch {
	case engine == EngineAvalAI && cfg.AvalAIConfigured():
		return cfg.AICrawlerModel
	case engine == EngineAICloudflare && cfg.CloudflareConfigured():
		return cfg.CloudflareModel
	}
	return ""
}

const (
	DefaultHTTPPort              = 80
	DefaultSiteConfigDir         = "configs"
	DefaultProdAPIKeysFile       = "secrets/.env.apikeys"
	DefaultDevAPIKeysFile        = "secrets/api_keys.json"
	DefaultMaxRequestDuration    = 3 * time.Minute
	DefaultSiteWaitTimeout       = 30 * time.Second
	DefaultMaxConcurrentRequests = 20
	DefaultMaxConcurrentPerSite  = 1
	DefaultReloadInterval        = 30 * time.Second
	DefaultBrowserChannel        = "chromium"
	DefaultBrowserMaxPages       = 3
	DefaultSearchUserAgent       = "go-wordpress-extractor/1.0"
	DefaultSearchCacheTTL        = 6 * time.Hour
	DefaultSearchNoResultTTL     = time.Hour
	DefaultDuckDuckGoIntervalMS  = 2000
	DefaultBraveIntervalMS       = 1000
	// DefaultScrapedIntervalMS spaces queries to Mojeek, Yahoo and Yandex,
	// whose results pages are read without an API.
	DefaultScrapedIntervalMS = 3000

	DefaultAICrawlerBaseURL         = "https://api.avalai.ir/v1"
	DefaultAICrawlerModel           = "deepseek-v4.1-flash"
	DefaultAICrawlerSearchModel     = "sonar"
	DefaultAICrawlerTimeout         = 45 * time.Second
	DefaultAICrawlerMaxInputChars   = 60000
	DefaultAICrawlerMaxOutputTokens = 2000
	DefaultAICrawlerRetries         = 2
	DefaultAICrawlerIntervalMS      = 500
	DefaultAICrawlerCacheTTL        = 6 * time.Hour
	DefaultAICrawlerResponseFormat  = "json_object"
	DefaultAICrawlerDelayMS         = 2000
	DefaultAICrawlerPromoteAfter    = 3
	DefaultAICrawlerDemoteAfter     = 3
	DefaultAICrawlerLearnAttempts   = 3

	DefaultCloudflareGatewayID       = "default"
	DefaultCloudflareEndpoint        = "rest"
	DefaultCloudflareModel           = "@cf/deepseek-ai/deepseek-v4.1-flash-1017"
	DefaultCloudflareReasoningEffort = "none"
	DefaultCloudflareIntervalMS      = 500
)

func newViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("APP")
	v.AutomaticEnv()
	return v
}

func NewConfig() (*Config, error) {
	cfg := buildConfig(newViper())

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	proxy := "direct"
	if u := cfg.ProxyURL(); u != nil {
		proxy = u.String()
	}
	zap.L().Info("config loaded",
		zap.String("env", cfg.ENV), zap.Int("port", cfg.HTTPPort), zap.String("sites", cfg.SiteConfigDir),
		zap.String("keys", cfg.APIKeysFile), zap.Bool("tls", cfg.TLSEnabled()), zap.Bool("ops", cfg.OpsEnabled()),
		zap.String("proxy", proxy), zap.Bool("browser", cfg.BrowserEnabled),
		zap.Bool("duckduckgo", cfg.DuckDuckGoEnabled), zap.Bool("brave", cfg.BraveEnabled),
		zap.Bool("mojeek", cfg.MojeekEnabled), zap.Bool("yahoo", cfg.YahooEnabled), zap.Bool("yandex", cfg.YandexEnabled),
		zap.Bool("ai_crawler", cfg.AICrawlerEnabled),
		zap.Bool("ai_avalai", cfg.AvalAIConfigured()), zap.String("ai_avalai_model", cfg.AIModel(EngineAvalAI)),
		zap.Bool("ai_cloudflare", cfg.CloudflareConfigured()), zap.String("ai_cloudflare_model", cfg.AIModel(EngineAICloudflare)),
		zap.String("ai_cloudflare_endpoint", cfg.CloudflareEndpoint),
		zap.String("default_engine", cfg.DefaultEngine), zap.Bool("ai_learn", cfg.AICrawlerEnabled && cfg.AICrawlerLearnSelectors))

	return cfg, nil
}

func buildConfig(v *viper.Viper) *Config {
	v.SetDefault("HTTP_PORT", DefaultHTTPPort)
	v.SetDefault("SITE_CONFIG_DIR", DefaultSiteConfigDir)
	v.SetDefault("MAX_REQUEST_DURATION", DefaultMaxRequestDuration)
	v.SetDefault("SITE_WAIT_TIMEOUT", DefaultSiteWaitTimeout)
	v.SetDefault("MAX_CONCURRENT_REQUESTS", DefaultMaxConcurrentRequests)
	v.SetDefault("MAX_CONCURRENT_PER_SITE", DefaultMaxConcurrentPerSite)
	v.SetDefault("RELOAD_INTERVAL", DefaultReloadInterval)
	v.SetDefault("BROWSER_ENABLED", false)
	v.SetDefault("BROWSER_CHANNEL", DefaultBrowserChannel)
	v.SetDefault("BROWSER_HEADLESS", true)
	v.SetDefault("BROWSER_MAX_PAGES", DefaultBrowserMaxPages)
	v.SetDefault("SEARCH_USER_AGENT", DefaultSearchUserAgent)
	v.SetDefault("SEARCH_CACHE_TTL", DefaultSearchCacheTTL)
	v.SetDefault("SEARCH_NO_RESULT_CACHE_TTL", DefaultSearchNoResultTTL)
	v.SetDefault("DUCKDUCKGO_ENABLED", false)
	v.SetDefault("DUCKDUCKGO_MIN_INTERVAL_MS", DefaultDuckDuckGoIntervalMS)
	v.SetDefault("BRAVE_ENABLED", false)
	v.SetDefault("BRAVE_MIN_INTERVAL_MS", DefaultBraveIntervalMS)
	v.SetDefault("MOJEEK_ENABLED", false)
	v.SetDefault("MOJEEK_MIN_INTERVAL_MS", DefaultScrapedIntervalMS)
	v.SetDefault("YAHOO_ENABLED", false)
	v.SetDefault("YAHOO_MIN_INTERVAL_MS", DefaultScrapedIntervalMS)
	v.SetDefault("YANDEX_ENABLED", false)
	v.SetDefault("YANDEX_MIN_INTERVAL_MS", DefaultScrapedIntervalMS)
	v.SetDefault("AI_CRAWLER_ENABLED", false)
	v.SetDefault("AI_CRAWLER_BASE_URL", DefaultAICrawlerBaseURL)
	v.SetDefault("AI_CRAWLER_MODEL", DefaultAICrawlerModel)
	v.SetDefault("AI_CRAWLER_SEARCH_MODEL", DefaultAICrawlerSearchModel)
	v.SetDefault("AI_CRAWLER_TIMEOUT", DefaultAICrawlerTimeout)
	v.SetDefault("AI_CRAWLER_MAX_INPUT_CHARS", DefaultAICrawlerMaxInputChars)
	v.SetDefault("AI_CRAWLER_MAX_OUTPUT_TOKENS", DefaultAICrawlerMaxOutputTokens)
	v.SetDefault("AI_CRAWLER_TEMPERATURE", 0)
	v.SetDefault("AI_CRAWLER_RETRIES", DefaultAICrawlerRetries)
	v.SetDefault("AI_CRAWLER_MIN_INTERVAL_MS", DefaultAICrawlerIntervalMS)
	v.SetDefault("AI_CRAWLER_CACHE_TTL", DefaultAICrawlerCacheTTL)
	v.SetDefault("AI_CRAWLER_RESPONSE_FORMAT", DefaultAICrawlerResponseFormat)
	v.SetDefault("AI_CRAWLER_DEFAULT_DELAY_MS", DefaultAICrawlerDelayMS)
	v.SetDefault("AI_CRAWLER_LEARN_SELECTORS", true)
	v.SetDefault("AI_CRAWLER_LEARN_PROMOTE_AFTER", DefaultAICrawlerPromoteAfter)
	v.SetDefault("AI_CRAWLER_LEARN_DEMOTE_AFTER", DefaultAICrawlerDemoteAfter)
	v.SetDefault("AI_CRAWLER_LEARN_MAX_ATTEMPTS_PER_DAY", DefaultAICrawlerLearnAttempts)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_GATEWAY_ID", DefaultCloudflareGatewayID)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_ENDPOINT", DefaultCloudflareEndpoint)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_MODEL", DefaultCloudflareModel)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT", DefaultCloudflareReasoningEffort)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING", true)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_COLLECT_LOG", false)
	v.SetDefault("AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS", DefaultCloudflareIntervalMS)

	cfg := &Config{
		ENV:                   v.GetString("ENV"),
		HTTPPort:              v.GetInt("HTTP_PORT"),
		HTTPAddress:           v.GetString("HTTP_ADDRESS"),
		SiteConfigDir:         v.GetString("SITE_CONFIG_DIR"),
		APIKeysFile:           v.GetString("API_KEYS_FILE"),
		APIKeyPepper:          v.GetString("API_KEY_PEPPER"),
		OpsUser:               v.GetString("OPS_USER"),
		OpsPasswordHash:       v.GetString("OPS_PASSWORD_HASH"),
		TLSCertFile:           v.GetString("TLS_CERT_FILE"),
		TLSKeyFile:            v.GetString("TLS_KEY_FILE"),
		MaxRequestDuration:    v.GetDuration("MAX_REQUEST_DURATION"),
		SiteWaitTimeout:       v.GetDuration("SITE_WAIT_TIMEOUT"),
		MaxConcurrentRequests: v.GetInt("MAX_CONCURRENT_REQUESTS"),
		MaxConcurrentPerSite:  v.GetInt("MAX_CONCURRENT_PER_SITE"),
		ReloadInterval:        v.GetDuration("RELOAD_INTERVAL"),
		ProxyHost:             v.GetString("PROXY_HOST"),
		ProxyPort:             v.GetInt("PROXY_PORT"),
		BrowserEnabled:        v.GetBool("BROWSER_ENABLED"),
		BrowserChannel:        v.GetString("BROWSER_CHANNEL"),
		BrowserHeadless:       v.GetBool("BROWSER_HEADLESS"),
		BrowserMaxPages:       v.GetInt("BROWSER_MAX_PAGES"),

		SearchUserAgent:         v.GetString("SEARCH_USER_AGENT"),
		SearchCacheTTL:          v.GetDuration("SEARCH_CACHE_TTL"),
		SearchNoResultCacheTTL:  v.GetDuration("SEARCH_NO_RESULT_CACHE_TTL"),
		DuckDuckGoEnabled:       v.GetBool("DUCKDUCKGO_ENABLED"),
		DuckDuckGoMinIntervalMS: v.GetInt("DUCKDUCKGO_MIN_INTERVAL_MS"),
		BraveEnabled:            v.GetBool("BRAVE_ENABLED"),
		BraveAPIKey:             v.GetString("BRAVE_API_KEY"),
		BraveAPIKeyFile:         v.GetString("BRAVE_API_KEY_FILE"),
		BraveMinIntervalMS:      v.GetInt("BRAVE_MIN_INTERVAL_MS"),
		MojeekEnabled:           v.GetBool("MOJEEK_ENABLED"),
		MojeekMinIntervalMS:     v.GetInt("MOJEEK_MIN_INTERVAL_MS"),
		YahooEnabled:            v.GetBool("YAHOO_ENABLED"),
		YahooMinIntervalMS:      v.GetInt("YAHOO_MIN_INTERVAL_MS"),
		YandexEnabled:           v.GetBool("YANDEX_ENABLED"),
		YandexMinIntervalMS:     v.GetInt("YANDEX_MIN_INTERVAL_MS"),

		DefaultEngine:     v.GetString("DEFAULT_ENGINE"),
		legacyDefaultRule: v.GetString("AI_CRAWLER_DEFAULT_RULE"),

		AICrawlerEnabled:                v.GetBool("AI_CRAWLER_ENABLED"),
		AICrawlerBaseURL:                v.GetString("AI_CRAWLER_BASE_URL"),
		AICrawlerModel:                  v.GetString("AI_CRAWLER_MODEL"),
		AICrawlerAPIKey:                 v.GetString("AI_CRAWLER_API_KEY"),
		AICrawlerAPIKeyFile:             v.GetString("AI_CRAWLER_API_KEY_FILE"),
		AICrawlerSearchModel:            v.GetString("AI_CRAWLER_SEARCH_MODEL"),
		AICrawlerTimeout:                v.GetDuration("AI_CRAWLER_TIMEOUT"),
		AICrawlerMaxInputChars:          v.GetInt("AI_CRAWLER_MAX_INPUT_CHARS"),
		AICrawlerMaxOutputTokens:        v.GetInt("AI_CRAWLER_MAX_OUTPUT_TOKENS"),
		AICrawlerTemperature:            v.GetFloat64("AI_CRAWLER_TEMPERATURE"),
		AICrawlerRetries:                v.GetInt("AI_CRAWLER_RETRIES"),
		AICrawlerMinIntervalMS:          v.GetInt("AI_CRAWLER_MIN_INTERVAL_MS"),
		AICrawlerCacheTTL:               v.GetDuration("AI_CRAWLER_CACHE_TTL"),
		AICrawlerResponseFormat:         v.GetString("AI_CRAWLER_RESPONSE_FORMAT"),
		AICrawlerDefaultDelayMS:         v.GetInt("AI_CRAWLER_DEFAULT_DELAY_MS"),
		AICrawlerLearnSelectors:         v.GetBool("AI_CRAWLER_LEARN_SELECTORS"),
		AICrawlerLearnDir:               v.GetString("AI_CRAWLER_LEARN_DIR"),
		AICrawlerLearnPromoteAfter:      v.GetInt("AI_CRAWLER_LEARN_PROMOTE_AFTER"),
		AICrawlerLearnDemoteAfter:       v.GetInt("AI_CRAWLER_LEARN_DEMOTE_AFTER"),
		AICrawlerLearnMaxAttemptsPerDay: v.GetInt("AI_CRAWLER_LEARN_MAX_ATTEMPTS_PER_DAY"),

		CloudflareAccountID:          v.GetString("AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID"),
		CloudflareGatewayID:          v.GetString("AI_CRAWLER_CLOUDFLARE_GATEWAY_ID"),
		CloudflareEndpoint:           v.GetString("AI_CRAWLER_CLOUDFLARE_ENDPOINT"),
		CloudflareBaseURL:            v.GetString("AI_CRAWLER_CLOUDFLARE_BASE_URL"),
		CloudflareModel:              v.GetString("AI_CRAWLER_CLOUDFLARE_MODEL"),
		CloudflareReasoningEffort:    v.GetString("AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT"),
		CloudflareAPIToken:           v.GetString("AI_CRAWLER_CLOUDFLARE_API_TOKEN"),
		CloudflareAPITokenFile:       v.GetString("AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE"),
		CloudflareProviderAPIKey:     v.GetString("AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY"),
		CloudflareProviderAPIKeyFile: v.GetString("AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY_FILE"),
		CloudflareNoUnifiedBilling:   v.GetBool("AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING"),
		CloudflareCollectLog:         v.GetBool("AI_CRAWLER_CLOUDFLARE_COLLECT_LOG"),
		CloudflareMinIntervalMS:      v.GetInt("AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS"),
	}
	if cfg.APIKeysFile == "" {
		cfg.APIKeysFile = DefaultAPIKeysFile(cfg.ENV)
	}
	return cfg
}

// LearnedDir is where learned site configs are kept: APP_AI_CRAWLER_LEARN_DIR,
// or <SITE_CONFIG_DIR>/learned. Empty when the AI crawler is off.
func (cfg *Config) LearnedDir() string {
	if !cfg.AICrawlerEnabled {
		return ""
	}
	if cfg.AICrawlerLearnDir != "" {
		return cfg.AICrawlerLearnDir
	}
	return filepath.Join(cfg.SiteConfigDir, "learned")
}

// DefaultAPIKeysFile returns the key file used when APP_API_KEYS_FILE is unset.
func DefaultAPIKeysFile(env string) string {
	if env == "production" {
		return DefaultProdAPIKeysFile
	}
	return DefaultDevAPIKeysFile
}

func (cfg *Config) IsTest() bool        { return cfg.ENV == "test" }
func (cfg *Config) IsDevelopment() bool { return cfg.ENV == "development" }
func (cfg *Config) IsProduction() bool  { return cfg.ENV == "production" }

// TLSEnabled reports whether the server terminates HTTPS itself.
func (cfg *Config) TLSEnabled() bool { return cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" }

// OpsEnabled reports whether Swagger UI (/swagger/*) is served.
func (cfg *Config) OpsEnabled() bool { return cfg.OpsUser != "" && cfg.OpsPasswordHash != "" }

// ProxyURL returns the outbound HTTP proxy, or nil to fetch directly.
func (cfg *Config) ProxyURL() *url.URL { return ProxyURL(cfg.ProxyHost, cfg.ProxyPort) }

// ProxyURL builds http://host:port, or returns nil when host is empty.
func ProxyURL(host string, port int) *url.URL {
	if host == "" {
		return nil
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port))}
}

// ProxyURLFromEnv reads APP_PROXY_HOST and APP_PROXY_PORT directly, for
// tools that do not build a full Config. It returns nil when no proxy is set.
func ProxyURLFromEnv() (*url.URL, error) {
	host := os.Getenv("APP_PROXY_HOST")
	port := 0
	if v := os.Getenv("APP_PROXY_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid APP_PROXY_PORT: %q", v)
		}
		port = p
	}
	if err := validateProxy(&Config{ProxyHost: host, ProxyPort: port}); err != nil {
		return nil, err
	}
	return ProxyURL(host, port), nil
}

// ShutdownTimeout is how long running requests may take to finish on stop.
func (cfg *Config) ShutdownTimeout() time.Duration { return cfg.MaxRequestDuration + 10*time.Second }

// MaxRequestDurationFromEnv reads APP_MAX_REQUEST_DURATION before the
// container is built, so the Fx stop timeout can be sized to match it.
func MaxRequestDurationFromEnv() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("APP_MAX_REQUEST_DURATION")); err == nil && d > 0 {
		return d
	}
	return DefaultMaxRequestDuration
}

// LoadEnv loads the first env file found: .env.<APP_ENV>, then .env,
// in the current directory and then its parent. Non-empty variables already
// in the environment win; variables that are unset or empty are filled from
// the file. (An empty exported variable, for example left behind by
// sourcing an older copy of the file, would otherwise hide the file's value.)
func LoadEnv() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	zap.L().Info("environment selected", zap.String("env", env))

	paths := []string{
		fmt.Sprintf(".env.%s", env),
		".env",
		filepath.Join("..", fmt.Sprintf(".env.%s", env)),
		filepath.Join("..", ".env"),
	}

	for _, path := range paths {
		values, err := godotenv.Read(path)
		if err != nil {
			continue
		}
		applyEnvFile(values)
		zap.L().Info("env file loaded", zap.String("path", path))
		return
	}

	zap.L().Warn("no .env file found")
}

func applyEnvFile(values map[string]string) {
	for k, v := range values {
		if os.Getenv(k) == "" {
			os.Setenv(k, v) //nolint:errcheck
		}
	}
}

// SearchSettings returns the search engine settings in the form the fetch
// package uses.
func (c *Config) SearchSettings() SearchSettings {
	return SearchSettings{
		UserAgent:             c.SearchUserAgent,
		CacheTTL:              c.SearchCacheTTL,
		NoResultTTL:           c.SearchNoResultCacheTTL,
		DuckDuckGoEnabled:     c.DuckDuckGoEnabled,
		DuckDuckGoMinInterval: time.Duration(c.DuckDuckGoMinIntervalMS) * time.Millisecond,
		BraveEnabled:          c.BraveEnabled,
		BraveAPIKey:           c.BraveAPIKey,
		BraveMinInterval:      time.Duration(c.BraveMinIntervalMS) * time.Millisecond,
		MojeekEnabled:         c.MojeekEnabled,
		MojeekMinInterval:     time.Duration(c.MojeekMinIntervalMS) * time.Millisecond,
		YahooEnabled:          c.YahooEnabled,
		YahooMinInterval:      time.Duration(c.YahooMinIntervalMS) * time.Millisecond,
		YandexEnabled:         c.YandexEnabled,
		YandexMinInterval:     time.Duration(c.YandexMinIntervalMS) * time.Millisecond,
	}
}

// SearchSettings mirrors fetch.SearchSettings; config does not import fetch.
type SearchSettings struct {
	UserAgent             string
	CacheTTL              time.Duration
	NoResultTTL           time.Duration
	DuckDuckGoEnabled     bool
	DuckDuckGoMinInterval time.Duration
	BraveEnabled          bool
	BraveAPIKey           string
	BraveMinInterval      time.Duration
	MojeekEnabled         bool
	MojeekMinInterval     time.Duration
	YahooEnabled          bool
	YahooMinInterval      time.Duration
	YandexEnabled         bool
	YandexMinInterval     time.Duration
}

// SearchFromEnv reads and checks only the search engine settings, for tools
// such as sitectl that do not load the whole server configuration.
func SearchFromEnv() (SearchSettings, error) {
	cfg := buildConfig(newViper())
	if err := ValidateSearch(cfg); err != nil {
		return SearchSettings{}, err
	}
	return cfg.SearchSettings(), nil
}
