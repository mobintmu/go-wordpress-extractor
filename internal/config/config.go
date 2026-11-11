package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
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

	// ReloadInterval is how often site configs and the API key file are
	// checked for changes.
	ReloadInterval time.Duration
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
	log.Printf("✅ Loaded config: env=%s port=%d sites=%s keys=%s tls=%t ops=%t",
		cfg.ENV, cfg.HTTPPort, cfg.SiteConfigDir, cfg.APIKeysFile, cfg.TLSEnabled(), cfg.OpsEnabled())

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

// OpsEnabled reports whether Swagger UI (/swagger/*) and /metrics are served.
func (cfg *Config) OpsEnabled() bool { return cfg.OpsUser != "" && cfg.OpsPasswordHash != "" }

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
	log.Printf("📋 Loaded environment: %s\n", env)

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
		log.Printf("✅ Loaded: %s\n", path)
		return
	}

	log.Printf("⚠️  No .env found\n")
}

func applyEnvFile(values map[string]string) {
	for k, v := range values {
		if os.Getenv(k) == "" {
			os.Setenv(k, v) //nolint:errcheck
		}
	}
}
