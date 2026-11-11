package config

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// MinPepperLength is the minimum length of APP_API_KEY_PEPPER.
const MinPepperLength = 32

// ValidateConfig checks every setting. The app does not start if one fails.
func ValidateConfig(cfg *Config) error {
	checks := []func(*Config) error{
		validateEnvironment,
		validateHTTPPort,
		validateHTTPAddress,
		validateSiteConfigDir,
		validateAPIKeysFile,
		validatePepper,
		validateTLS,
		validateDurations,
		validateConcurrency,
	}
	for _, check := range checks {
		if err := check(cfg); err != nil {
			return err
		}
	}
	validateWarnings(cfg)
	return nil
}

func validateEnvironment(cfg *Config) error {
	switch cfg.ENV {
	case "development", "test", "production":
		return nil
	case "":
		return fmt.Errorf("ENV is empty. Set APP_ENV to one of: development, test, production")
	default:
		return fmt.Errorf("invalid ENV: %q. Expected one of: development, test, production. Set APP_ENV", cfg.ENV)
	}
}

func validateHTTPPort(cfg *Config) error {
	if cfg.HTTPPort <= 0 || cfg.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP_PORT: %d. Expected value between 1 and 65535. Set APP_HTTP_PORT", cfg.HTTPPort)
	}
	return nil
}

func validateHTTPAddress(cfg *Config) error {
	if cfg.HTTPAddress == "" || cfg.HTTPAddress == "localhost" {
		return nil
	}
	if net.ParseIP(cfg.HTTPAddress) == nil {
		return fmt.Errorf("invalid HTTP_ADDRESS: %q. Expected an IP address, 'localhost', or empty for all interfaces", cfg.HTTPAddress)
	}
	return nil
}

func validateSiteConfigDir(cfg *Config) error {
	st, err := os.Stat(cfg.SiteConfigDir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("SITE_CONFIG_DIR %q is not a readable directory. Set APP_SITE_CONFIG_DIR", cfg.SiteConfigDir)
	}
	return nil
}

func validateAPIKeysFile(cfg *Config) error {
	if cfg.APIKeysFile == "" {
		return fmt.Errorf("API_KEYS_FILE is empty. Set APP_API_KEYS_FILE")
	}
	if _, err := os.Stat(cfg.APIKeysFile); err != nil {
		return fmt.Errorf("API_KEYS_FILE %q cannot be read: %w. Create it with: apikey generate --name <client> "+
			"(with Docker Compose: docker compose run --rm apikey generate --name <client>)", cfg.APIKeysFile, err)
	}
	return nil
}

func validatePepper(cfg *Config) error {
	if len(cfg.APIKeyPepper) < MinPepperLength {
		return fmt.Errorf("API_KEY_PEPPER must be at least %d characters. Create one with: apikey generate-pepper", MinPepperLength)
	}
	return nil
}

func validateTLS(cfg *Config) error {
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return fmt.Errorf("APP_TLS_CERT_FILE and APP_TLS_KEY_FILE must be set together")
	}
	for _, f := range []string{cfg.TLSCertFile, cfg.TLSKeyFile} {
		if f == "" {
			continue
		}
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("TLS file %q cannot be read: %w", f, err)
		}
	}
	return nil
}

func validateDurations(cfg *Config) error {
	if cfg.MaxRequestDuration < time.Second || cfg.MaxRequestDuration > 10*time.Minute {
		return fmt.Errorf("invalid MAX_REQUEST_DURATION: %s. Expected between 1s and 10m", cfg.MaxRequestDuration)
	}
	if cfg.SiteWaitTimeout < 0 || cfg.SiteWaitTimeout > cfg.MaxRequestDuration {
		return fmt.Errorf("invalid SITE_WAIT_TIMEOUT: %s. Expected between 0 and MAX_REQUEST_DURATION", cfg.SiteWaitTimeout)
	}
	if cfg.ReloadInterval < 10*time.Millisecond {
		return fmt.Errorf("invalid RELOAD_INTERVAL: %s", cfg.ReloadInterval)
	}
	return nil
}

func validateConcurrency(cfg *Config) error {
	if cfg.MaxConcurrentRequests < 1 {
		return fmt.Errorf("invalid MAX_CONCURRENT_REQUESTS: %d. Expected at least 1", cfg.MaxConcurrentRequests)
	}
	if cfg.MaxConcurrentPerSite < 1 {
		return fmt.Errorf("invalid MAX_CONCURRENT_PER_SITE: %d. Expected at least 1", cfg.MaxConcurrentPerSite)
	}
	return nil
}

func validateWarnings(cfg *Config) {
	if (cfg.OpsUser == "") != (cfg.OpsPasswordHash == "") {
		log.Printf("⚠️  WARNING: set both APP_OPS_USER and APP_OPS_PASSWORD_HASH. Swagger UI and /metrics stay off")
	}
	if cfg.IsProduction() && !cfg.TLSEnabled() {
		log.Printf("⚠️  WARNING: serving plain HTTP in production. API keys travel in a header; use this only on a private network")
	}
	if cfg.IsProduction() {
		if st, err := os.Stat(cfg.APIKeysFile); err == nil && st.Mode().Perm()&0o077 != 0 {
			log.Printf("⚠️  WARNING: %s has permission %o; expected 0600", cfg.APIKeysFile, st.Mode().Perm())
		}
	}
}
