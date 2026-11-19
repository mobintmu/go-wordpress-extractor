package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"regexp"
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

// ValidateAICrawler checks APP_DEFAULT_ENGINE and the AI engines'
// settings, and loads the secrets from their _FILE variables when the
// variables themselves are empty. An engine is configured when the AI
// crawler is on and its own settings are complete (AvalAI: a key;
// Cloudflare: an account id and a token). The app does not start when
// APP_DEFAULT_ENGINE names an engine that is not configured, or when the
// AI crawler is on and no engine is. Secrets never appear in an error.
func ValidateAICrawler(cfg *Config) error {
	cfg.AICrawlerAPIKey = strings.TrimSpace(cfg.AICrawlerAPIKey)
	cfg.CloudflareAPIToken = strings.TrimSpace(cfg.CloudflareAPIToken)
	cfg.CloudflareProviderAPIKey = strings.TrimSpace(cfg.CloudflareProviderAPIKey)
	cfg.avalaiConfigured, cfg.cloudflareConfigured = false, false

	if strings.TrimSpace(cfg.legacyDefaultRule) != "" {
		return fmt.Errorf("APP_AI_CRAWLER_DEFAULT_RULE was replaced by APP_DEFAULT_ENGINE: remove it and set " +
			"APP_DEFAULT_ENGINE=ai-crawler-avalai (was true), ai-crawler-cloudflare, or none (was false)")
	}
	explicit := strings.ToLower(strings.TrimSpace(cfg.DefaultEngine))
	cfg.DefaultEngine = explicit
	if cfg.DefaultEngine == "" {
		cfg.DefaultEngine = EngineNone
		if cfg.AICrawlerEnabled {
			cfg.DefaultEngine = EngineAvalAI
		}
	}
	switch cfg.DefaultEngine {
	case EngineNone, EngineAvalAI, EngineAICloudflare:
	case "http", "browser", "duckduckgo", "brave", "mojeek", "yahoo", "yandex":
		return fmt.Errorf("APP_DEFAULT_ENGINE %q cannot serve websites without a rule: that engine needs a site config. "+
			"Expected ai-crawler-avalai, ai-crawler-cloudflare or none", cfg.DefaultEngine)
	default:
		return fmt.Errorf("invalid APP_DEFAULT_ENGINE: %q. Expected ai-crawler-avalai, ai-crawler-cloudflare or none", cfg.DefaultEngine)
	}

	if !cfg.AICrawlerEnabled {
		if explicit != "" && cfg.DefaultEngine != EngineNone {
			return fmt.Errorf("APP_DEFAULT_ENGINE is %s, but APP_AI_CRAWLER_ENABLED is false. "+
				"Set APP_DEFAULT_ENGINE=none, or turn the AI crawler on", cfg.DefaultEngine)
		}
		return nil
	}
	if err := validateAIShared(cfg); err != nil {
		return err
	}
	avalaiMissing, err := validateAvalAI(cfg)
	if err != nil {
		return err
	}
	cfMissing, err := validateCloudflare(cfg)
	if err != nil {
		return err
	}

	switch {
	case cfg.DefaultEngine == EngineAvalAI && !cfg.avalaiConfigured:
		return fmt.Errorf("APP_DEFAULT_ENGINE is ai-crawler-avalai, but that engine is not configured: %s", avalaiMissing)
	case cfg.DefaultEngine == EngineAICloudflare && !cfg.cloudflareConfigured:
		return fmt.Errorf("APP_DEFAULT_ENGINE is ai-crawler-cloudflare, but that engine is not configured: %s", cfMissing)
	case !cfg.avalaiConfigured && !cfg.cloudflareConfigured:
		return fmt.Errorf("APP_AI_CRAWLER_ENABLED is true, but no AI engine is configured. AvalAI: %s. Cloudflare: %s", avalaiMissing, cfMissing)
	}
	return nil
}

// validateAIShared checks the settings both AI engines use.
func validateAIShared(cfg *Config) error {
	checks := []struct {
		name      string
		v, lo, hi int
	}{
		{"AI_CRAWLER_MAX_INPUT_CHARS", cfg.AICrawlerMaxInputChars, 1000, 1000000},
		{"AI_CRAWLER_MAX_OUTPUT_TOKENS", cfg.AICrawlerMaxOutputTokens, 100, 100000},
		{"AI_CRAWLER_RETRIES", cfg.AICrawlerRetries, 0, 5},
		{"AI_CRAWLER_MIN_INTERVAL_MS", cfg.AICrawlerMinIntervalMS, 0, 60000},
		{"AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS", cfg.CloudflareMinIntervalMS, 0, 60000},
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

// readSecret returns value, or the trimmed content of file when value is
// empty. A file that does not exist gives "" and no error, so an engine
// that is not used does not need its secret file.
func readSecret(value, file, fileVar string) (string, error) {
	if value != "" || file == "" {
		return value, nil
	}
	b, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s %q cannot be read: %w", fileVar, file, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// validateAvalAI checks the ai-crawler-avalai settings. Without a key the
// engine is simply not configured; missing says what to set.
func validateAvalAI(cfg *Config) (missing string, err error) {
	missing = "set APP_AI_CRAWLER_API_KEY, or APP_AI_CRAWLER_API_KEY_FILE to a file holding only the key " +
		"(secrets/avalai_api_key); keep it out of committed files such as .env"
	if cfg.AICrawlerAPIKey, err = readSecret(cfg.AICrawlerAPIKey, cfg.AICrawlerAPIKeyFile, "APP_AI_CRAWLER_API_KEY_FILE"); err != nil {
		return missing, err
	}
	if cfg.AICrawlerAPIKey == "" {
		if cfg.AICrawlerAPIKeyFile != "" {
			missing += fmt.Sprintf(" (APP_AI_CRAWLER_API_KEY_FILE %q does not exist or is empty)", cfg.AICrawlerAPIKeyFile)
		}
		return missing, nil
	}
	if strings.ContainsAny(cfg.AICrawlerAPIKey, " \t\r\n") {
		return missing, fmt.Errorf("the AI crawler API key contains white space; check APP_AI_CRAWLER_API_KEY or APP_AI_CRAWLER_API_KEY_FILE")
	}
	if err := checkBaseURL("AI_CRAWLER_BASE_URL", cfg.AICrawlerBaseURL, DefaultAICrawlerBaseURL); err != nil {
		return missing, err
	}
	if strings.TrimSpace(cfg.AICrawlerModel) == "" {
		return missing, fmt.Errorf("AI_CRAWLER_MODEL is empty. Set APP_AI_CRAWLER_MODEL, for example %s", DefaultAICrawlerModel)
	}
	cfg.AICrawlerSearchModel = strings.TrimSpace(cfg.AICrawlerSearchModel)
	if strings.EqualFold(cfg.AICrawlerSearchModel, "none") {
		cfg.AICrawlerSearchModel = ""
	}
	if strings.ContainsAny(cfg.AICrawlerSearchModel, " \t\r\n") {
		return missing, fmt.Errorf("invalid AI_CRAWLER_SEARCH_MODEL: %q. Expected a model id without white space, for example %s, or none to turn ai_fetch web_search off", cfg.AICrawlerSearchModel, DefaultAICrawlerSearchModel)
	}
	cfg.avalaiConfigured = true
	return "", nil
}

var hexAccountID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// validateCloudflare checks the ai-crawler-cloudflare settings. Without an
// account id the engine is not configured; with one, every other setting
// must be right.
func validateCloudflare(cfg *Config) (missing string, err error) {
	missing = "set APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID and the token (APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN, " +
		"or APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE holding only the token, such as secrets/cloudflare_api_token)"
	cfg.CloudflareAccountID = strings.ToLower(strings.TrimSpace(cfg.CloudflareAccountID))
	if cfg.CloudflareAccountID == "" {
		return missing, nil
	}
	if !hexAccountID.MatchString(cfg.CloudflareAccountID) {
		return missing, fmt.Errorf("invalid AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID: %q. Expected 32 hex characters, as shown on the Cloudflare dashboard's account home", cfg.CloudflareAccountID)
	}
	cfg.CloudflareGatewayID = strings.TrimSpace(cfg.CloudflareGatewayID)
	if cfg.CloudflareGatewayID == "" || len(cfg.CloudflareGatewayID) > 64 || strings.ContainsAny(cfg.CloudflareGatewayID, "/ \t?#") {
		return missing, fmt.Errorf("invalid AI_CRAWLER_CLOUDFLARE_GATEWAY_ID: %q. Expected a gateway id of at most 64 characters, such as %s", cfg.CloudflareGatewayID, DefaultCloudflareGatewayID)
	}
	cfg.CloudflareEndpoint = strings.ToLower(strings.TrimSpace(cfg.CloudflareEndpoint))
	switch cfg.CloudflareEndpoint {
	case "rest", "compat":
	default:
		return missing, fmt.Errorf("invalid AI_CRAWLER_CLOUDFLARE_ENDPOINT: %q. Expected rest or compat", cfg.CloudflareEndpoint)
	}
	if cfg.CloudflareBaseURL != "" {
		if err := checkBaseURL("AI_CRAWLER_CLOUDFLARE_BASE_URL", cfg.CloudflareBaseURL, "https://api.cloudflare.com/client/v4/accounts/<id>/ai/v1"); err != nil {
			return missing, err
		}
	}
	cfg.CloudflareModel = strings.TrimSpace(cfg.CloudflareModel)
	if cfg.CloudflareModel == "" || strings.ContainsAny(cfg.CloudflareModel, " \t\r\n") {
		return missing, fmt.Errorf("invalid AI_CRAWLER_CLOUDFLARE_MODEL: %q. Expected a model id from the Cloudflare catalog without white space, for example %s", cfg.CloudflareModel, DefaultCloudflareModel)
	}
	cfg.CloudflareReasoningEffort = strings.ToLower(strings.TrimSpace(cfg.CloudflareReasoningEffort))
	switch cfg.CloudflareReasoningEffort {
	case "", "none", "low", "high", "max":
	default:
		return missing, fmt.Errorf("invalid AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT: %q. Expected none, low, high or max", cfg.CloudflareReasoningEffort)
	}
	if cfg.CloudflareAPIToken, err = readSecret(cfg.CloudflareAPIToken, cfg.CloudflareAPITokenFile, "APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE"); err != nil {
		return missing, err
	}
	if cfg.CloudflareProviderAPIKey, err = readSecret(cfg.CloudflareProviderAPIKey, cfg.CloudflareProviderAPIKeyFile, "APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY_FILE"); err != nil {
		return missing, err
	}
	if cfg.CloudflareAPIToken == "" {
		return missing, fmt.Errorf("APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID is set, but the token is missing: set APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN, " +
			"or put the token alone in the file named by APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE (secrets/cloudflare_api_token)")
	}
	if strings.ContainsAny(cfg.CloudflareAPIToken, " \t\r\n") {
		return missing, fmt.Errorf("the Cloudflare API token contains white space; check APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN or APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE")
	}
	if strings.ContainsAny(cfg.CloudflareProviderAPIKey, " \t\r\n") {
		return missing, fmt.Errorf("the Cloudflare provider API key contains white space; check APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY or its _FILE")
	}
	if cfg.CloudflareProviderAPIKey != "" && cfg.CloudflareEndpoint == "rest" {
		return missing, fmt.Errorf("APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY cannot be used with APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT=rest: " +
			"Authorization carries the Cloudflare token there. Store the provider's key in the gateway (BYOK), or use ENDPOINT=compat")
	}
	cfg.cloudflareConfigured = true
	return "", nil
}

// checkBaseURL accepts an absolute https URL, or plain http on localhost.
func checkBaseURL(name, raw, example string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("invalid %s: %q. Expected an absolute URL such as %s. Set APP_%s", name, raw, example, name)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("%s %q must use https: the API key would travel in clear text. Plain http is only accepted for localhost", name, raw)
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
