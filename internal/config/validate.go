package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
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
		validateProxy,
		validateBrowser,
		ValidateSearch,
		ValidateAICrawler,
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

func validateProxy(cfg *Config) error {
	if cfg.ProxyHost == "" {
		if cfg.ProxyPort != 0 {
			return fmt.Errorf("APP_PROXY_PORT is set but APP_PROXY_HOST is empty. Set both, or neither to fetch directly")
		}
		return nil
	}
	if strings.ContainsAny(cfg.ProxyHost, "/@ ") || (strings.Contains(cfg.ProxyHost, ":") && net.ParseIP(cfg.ProxyHost) == nil) {
		return fmt.Errorf("invalid PROXY_HOST: %q. Expected a host name or IP address only, for example 127.0.0.1", cfg.ProxyHost)
	}
	if cfg.ProxyPort <= 0 || cfg.ProxyPort > 65535 {
		return fmt.Errorf("invalid PROXY_PORT: %d. Expected value between 1 and 65535. Set APP_PROXY_PORT", cfg.ProxyPort)
	}
	return nil
}

func validateWarnings(cfg *Config) {
	if (cfg.OpsUser == "") != (cfg.OpsPasswordHash == "") {
		zap.L().Warn("set both APP_OPS_USER and APP_OPS_PASSWORD_HASH; Swagger UI stays off")
	}
	if cfg.IsProduction() && !cfg.TLSEnabled() {
		zap.L().Warn("serving plain HTTP in production; API keys travel in a header, use this only on a private network")
	}
	if cfg.IsProduction() {
		if st, err := os.Stat(cfg.APIKeysFile); err == nil && st.Mode().Perm()&0o077 != 0 {
			zap.L().Warn("API keys file permissions too open; expected 0600", zap.String("file", cfg.APIKeysFile), zap.String("perm", fmt.Sprintf("%o", st.Mode().Perm())))
		}
	}
}

func validateBrowser(cfg *Config) error {
	if !cfg.BrowserEnabled {
		return nil
	}
	switch cfg.BrowserChannel {
	case "chromium", "chrome", "chrome-beta", "msedge":
	default:
		return fmt.Errorf("invalid BROWSER_CHANNEL: %q. Expected chromium or chrome. Set APP_BROWSER_CHANNEL", cfg.BrowserChannel)
	}
	if cfg.BrowserMaxPages < 1 || cfg.BrowserMaxPages > 50 {
		return fmt.Errorf("invalid BROWSER_MAX_PAGES: %d. Expected value between 1 and 50. Set APP_BROWSER_MAX_PAGES", cfg.BrowserMaxPages)
	}
	return nil
}

// ValidateSearch checks the search engine settings (DuckDuckGo, Brave,
// Mojeek, Yahoo, Yandex) and loads the Brave API
// key from APP_BRAVE_API_KEY_FILE when APP_BRAVE_API_KEY is empty. With
// APP_BRAVE_ENABLED=true and no key the app does not start, so a missing key
// shows up at deploy time and not as failed requests. The key is never put
// in an error message.
func ValidateSearch(cfg *Config) error {
	if strings.TrimSpace(cfg.SearchUserAgent) == "" {
		cfg.SearchUserAgent = DefaultSearchUserAgent
	}
	if cfg.SearchCacheTTL < 0 || cfg.SearchNoResultCacheTTL < 0 {
		return fmt.Errorf("APP_SEARCH_CACHE_TTL and APP_SEARCH_NO_RESULT_CACHE_TTL must not be negative (0 turns the cache off)")
	}
	if cfg.DuckDuckGoMinIntervalMS < 0 || cfg.DuckDuckGoMinIntervalMS > 60000 {
		return fmt.Errorf("invalid DUCKDUCKGO_MIN_INTERVAL_MS: %d. Expected value between 0 and 60000. Set APP_DUCKDUCKGO_MIN_INTERVAL_MS", cfg.DuckDuckGoMinIntervalMS)
	}
	if cfg.BraveMinIntervalMS < 0 || cfg.BraveMinIntervalMS > 60000 {
		return fmt.Errorf("invalid BRAVE_MIN_INTERVAL_MS: %d. Expected value between 0 and 60000. Set APP_BRAVE_MIN_INTERVAL_MS", cfg.BraveMinIntervalMS)
	}
	for name, ms := range map[string]int{"MOJEEK": cfg.MojeekMinIntervalMS, "YAHOO": cfg.YahooMinIntervalMS, "YANDEX": cfg.YandexMinIntervalMS} {
		if ms < 0 || ms > 60000 {
			return fmt.Errorf("invalid %s_MIN_INTERVAL_MS: %d. Expected value between 0 and 60000. Set APP_%s_MIN_INTERVAL_MS", name, ms, name)
		}
	}
	cfg.BraveAPIKey = strings.TrimSpace(cfg.BraveAPIKey)
	if !cfg.BraveEnabled {
		return nil
	}
	if cfg.BraveAPIKey == "" && cfg.BraveAPIKeyFile != "" {
		b, err := os.ReadFile(cfg.BraveAPIKeyFile)
		if err != nil {
			return fmt.Errorf("APP_BRAVE_API_KEY_FILE %q cannot be read: %w", cfg.BraveAPIKeyFile, err)
		}
		cfg.BraveAPIKey = strings.TrimSpace(string(b))
	}
	if cfg.BraveAPIKey == "" {
		return fmt.Errorf("APP_BRAVE_API_KEY or APP_BRAVE_API_KEY_FILE is required when APP_BRAVE_ENABLED is true. " +
			"Create a key in the Brave Search API dashboard; keep it out of committed files such as .env.development")
	}
	if strings.ContainsAny(cfg.BraveAPIKey, " \t\r\n") {
		return fmt.Errorf("the Brave API key contains white space; check APP_BRAVE_API_KEY or APP_BRAVE_API_KEY_FILE")
	}
	return nil
}

// ValidateAICrawler checks the AI crawler settings and loads the API key
// from APP_AI_CRAWLER_API_KEY_FILE when APP_AI_CRAWLER_API_KEY is empty.
// With APP_AI_CRAWLER_ENABLED=true and no key the app does not start. The
// key is never put in an error message.
func ValidateAICrawler(cfg *Config) error {
	cfg.AICrawlerAPIKey = strings.TrimSpace(cfg.AICrawlerAPIKey)
	if !cfg.AICrawlerEnabled {
		return nil
	}
	u, err := url.Parse(cfg.AICrawlerBaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("invalid AI_CRAWLER_BASE_URL: %q. Expected an absolute URL such as %s. Set APP_AI_CRAWLER_BASE_URL", cfg.AICrawlerBaseURL, DefaultAICrawlerBaseURL)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("AI_CRAWLER_BASE_URL %q must use https: the API key would travel in clear text. Plain http is only accepted for localhost", cfg.AICrawlerBaseURL)
	}
	if strings.TrimSpace(cfg.AICrawlerModel) == "" {
		return fmt.Errorf("AI_CRAWLER_MODEL is empty. Set APP_AI_CRAWLER_MODEL, for example %s", DefaultAICrawlerModel)
	}
	if cfg.AICrawlerAPIKey == "" && cfg.AICrawlerAPIKeyFile != "" {
		b, err := os.ReadFile(cfg.AICrawlerAPIKeyFile)
		if err != nil {
			return fmt.Errorf("APP_AI_CRAWLER_API_KEY_FILE %q cannot be read: %w", cfg.AICrawlerAPIKeyFile, err)
		}
		cfg.AICrawlerAPIKey = strings.TrimSpace(string(b))
	}
	if cfg.AICrawlerAPIKey == "" {
		return fmt.Errorf("APP_AI_CRAWLER_API_KEY or APP_AI_CRAWLER_API_KEY_FILE is required when APP_AI_CRAWLER_ENABLED is true. " +
			"Put the key alone in secrets/avalai_api_key; keep it out of committed files such as .env")
	}
	if strings.ContainsAny(cfg.AICrawlerAPIKey, " \t\r\n") {
		return fmt.Errorf("the AI crawler API key contains white space; check APP_AI_CRAWLER_API_KEY or APP_AI_CRAWLER_API_KEY_FILE")
	}
	checks := []struct {
		name      string
		v, lo, hi int
	}{
		{"AI_CRAWLER_MAX_INPUT_CHARS", cfg.AICrawlerMaxInputChars, 1000, 1000000},
		{"AI_CRAWLER_MAX_OUTPUT_TOKENS", cfg.AICrawlerMaxOutputTokens, 100, 100000},
		{"AI_CRAWLER_RETRIES", cfg.AICrawlerRetries, 0, 5},
		{"AI_CRAWLER_MIN_INTERVAL_MS", cfg.AICrawlerMinIntervalMS, 0, 60000},
		{"AI_CRAWLER_DEFAULT_DELAY_MS", cfg.AICrawlerDefaultDelayMS, 0, 60000},
		{"AI_CRAWLER_LEARN_PROMOTE_AFTER", cfg.AICrawlerLearnPromoteAfter, 1, 100},
		{"AI_CRAWLER_LEARN_DEMOTE_AFTER", cfg.AICrawlerLearnDemoteAfter, 1, 100},
		{"AI_CRAWLER_LEARN_MAX_ATTEMPTS_PER_DAY", cfg.AICrawlerLearnMaxAttemptsPerDay, 1, 1000},
	}
	for _, c := range checks {
		if c.v < c.lo || c.v > c.hi {
			return fmt.Errorf("invalid %s: %d. Expected value between %d and %d. Set APP_%s", c.name, c.v, c.lo, c.hi, c.name)
		}
	}
	if cfg.AICrawlerTimeout < time.Second || cfg.AICrawlerTimeout > 10*time.Minute {
		return fmt.Errorf("invalid AI_CRAWLER_TIMEOUT: %s. Expected between 1s and 10m", cfg.AICrawlerTimeout)
	}
	if cfg.AICrawlerTemperature < 0 || cfg.AICrawlerTemperature > 2 {
		return fmt.Errorf("invalid AI_CRAWLER_TEMPERATURE: %v. Expected between 0 and 2", cfg.AICrawlerTemperature)
	}
	if cfg.AICrawlerCacheTTL < 0 {
		return fmt.Errorf("APP_AI_CRAWLER_CACHE_TTL must not be negative (0 turns the cache off)")
	}
	switch cfg.AICrawlerResponseFormat {
	case "json_object", "json_schema":
	default:
		return fmt.Errorf("invalid AI_CRAWLER_RESPONSE_FORMAT: %q. Expected json_object or json_schema", cfg.AICrawlerResponseFormat)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
