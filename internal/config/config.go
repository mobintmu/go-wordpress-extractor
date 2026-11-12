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
		zap.String("proxy", proxy), zap.Bool("browser", cfg.BrowserEnabled))

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
