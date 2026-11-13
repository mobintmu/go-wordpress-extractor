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
}

const (
	DefaultHTTPPort              = 4000
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
		zap.Bool("duckduckgo", cfg.DuckDuckGoEnabled), zap.Bool("brave", cfg.BraveEnabled))

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
	}
	if cfg.APIKeysFile == "" {
		cfg.APIKeysFile = DefaultAPIKeysFile(cfg.ENV)
	}
	return cfg
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
