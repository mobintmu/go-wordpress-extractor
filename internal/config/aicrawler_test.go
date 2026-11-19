package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testAccount = "0123456789abcdef0123456789abcdef"

// aiConfig is a valid configuration with the AI crawler on and both
// engines configured, from the defaults buildConfig applies.
func aiConfig(t *testing.T) *Config {
	t.Helper()
	for _, k := range []string{"APP_DEFAULT_ENGINE", "APP_AI_CRAWLER_DEFAULT_RULE", "APP_AI_CRAWLER_API_KEY_FILE",
		"APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE", "APP_AI_CRAWLER_CLOUDFLARE_PROVIDER_API_KEY_FILE"} {
		t.Setenv(k, "")
	}
	cfg := buildConfig(newViper())
	base := valid(t)
	base.AICrawlerEnabled = true
	for _, f := range []func(*Config){
		func(c *Config) {
			c.AICrawlerBaseURL, c.AICrawlerModel, c.AICrawlerAPIKey = cfg.AICrawlerBaseURL, cfg.AICrawlerModel, "sk-avalai"
			c.AICrawlerTimeout, c.AICrawlerMaxInputChars, c.AICrawlerMaxOutputTokens = cfg.AICrawlerTimeout, cfg.AICrawlerMaxInputChars, cfg.AICrawlerMaxOutputTokens
			c.AICrawlerRetries, c.AICrawlerMinIntervalMS, c.AICrawlerResponseFormat = cfg.AICrawlerRetries, cfg.AICrawlerMinIntervalMS, cfg.AICrawlerResponseFormat
			c.AICrawlerDefaultDelayMS, c.AICrawlerLearnPromoteAfter = cfg.AICrawlerDefaultDelayMS, cfg.AICrawlerLearnPromoteAfter
			c.AICrawlerLearnDemoteAfter, c.AICrawlerLearnMaxAttemptsPerDay = cfg.AICrawlerLearnDemoteAfter, cfg.AICrawlerLearnMaxAttemptsPerDay
		},
		func(c *Config) {
			c.CloudflareAccountID, c.CloudflareGatewayID, c.CloudflareEndpoint = testAccount, cfg.CloudflareGatewayID, cfg.CloudflareEndpoint
			c.CloudflareModel, c.CloudflareReasoningEffort, c.CloudflareAPIToken = cfg.CloudflareModel, cfg.CloudflareReasoningEffort, "cf-token"
			c.CloudflareMinIntervalMS, c.CloudflareNoUnifiedBilling = cfg.CloudflareMinIntervalMS, cfg.CloudflareNoUnifiedBilling
		},
	} {
		f(base)
	}
	return base
}

func TestCloudflareDefaults(t *testing.T) {
	for _, k := range []string{"APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID", "APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT", "APP_AI_CRAWLER_CLOUDFLARE_MODEL",
		"APP_AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT", "APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING", "APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG",
		"APP_AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS"} {
		t.Setenv(k, "")
	}
	cfg := buildConfig(newViper())
	if cfg.CloudflareGatewayID != "default" || cfg.CloudflareEndpoint != "rest" || cfg.CloudflareModel != "@cf/deepseek-ai/deepseek-v4.1-flash-1017" ||
		cfg.CloudflareReasoningEffort != "none" || !cfg.CloudflareNoUnifiedBilling || cfg.CloudflareCollectLog || cfg.CloudflareMinIntervalMS != 500 {
		t.Errorf("defaults = %+v", cfg)
	}
}

func TestDefaultEngine(t *testing.T) {
	cfg := aiConfig(t)
	if err := ValidateAICrawler(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultEngine != EngineAvalAI || cfg.DefaultAIEngine() != EngineAvalAI || !cfg.AvalAIConfigured() || !cfg.CloudflareConfigured() {
		t.Errorf("empty APP_DEFAULT_ENGINE with AI on: default %q, avalai %v, cloudflare %v", cfg.DefaultEngine, cfg.AvalAIConfigured(), cfg.CloudflareConfigured())
	}
	if cfg.AIModel(EngineAICloudflare) != DefaultCloudflareModel {
		t.Errorf("cloudflare model = %q", cfg.AIModel(EngineAICloudflare))
	}

	cfg = aiConfig(t)
	cfg.DefaultEngine = "AI-Crawler-Cloudflare "
	if err := ValidateAICrawler(cfg); err != nil || cfg.DefaultAIEngine() != EngineAICloudflare {
		t.Errorf("cloudflare default: %v, %q", err, cfg.DefaultAIEngine())
	}

	cfg = aiConfig(t)
	cfg.DefaultEngine = "none"
	if err := ValidateAICrawler(cfg); err != nil || cfg.DefaultAIEngine() != "" {
		t.Errorf("none: %v, %q", err, cfg.DefaultAIEngine())
	}

	// AI off: an empty value means none; an AI engine is refused.
	off := valid(t)
	if err := ValidateAICrawler(off); err != nil || off.DefaultEngine != EngineNone || off.DefaultAIEngine() != "" {
		t.Errorf("AI off: %v, %q", err, off.DefaultEngine)
	}
	off.DefaultEngine = EngineAvalAI
	if err := ValidateAICrawler(off); err == nil || !strings.Contains(err.Error(), "APP_DEFAULT_ENGINE") || !strings.Contains(err.Error(), "APP_AI_CRAWLER_ENABLED") {
		t.Errorf("AI engine with AI off: %v", err)
	}
}

func TestDefaultEngineRefused(t *testing.T) {
	cases := map[string]struct {
		mut  func(*Config)
		want string
	}{
		"unknown":       {func(c *Config) { c.DefaultEngine = "gpt" }, "invalid APP_DEFAULT_ENGINE"},
		"needs config":  {func(c *Config) { c.DefaultEngine = "http" }, "cannot serve websites without a rule"},
		"search engine": {func(c *Config) { c.DefaultEngine = "brave" }, "cannot serve websites without a rule"},
		"legacy var":    {func(c *Config) { c.legacyDefaultRule = "true" }, "APP_AI_CRAWLER_DEFAULT_RULE was replaced by APP_DEFAULT_ENGINE"},
		"avalai unconfigured": {func(c *Config) { c.AICrawlerAPIKey = "" },
			"APP_DEFAULT_ENGINE is ai-crawler-avalai, but that engine is not configured: set APP_AI_CRAWLER_API_KEY"},
		"cloudflare unconfigured": {func(c *Config) { c.DefaultEngine, c.CloudflareAccountID = EngineAICloudflare, "" },
			"APP_DEFAULT_ENGINE is ai-crawler-cloudflare, but that engine is not configured"},
		"nothing configured": {func(c *Config) { c.DefaultEngine, c.AICrawlerAPIKey, c.CloudflareAccountID = "none", "", "" },
			"no AI engine is configured"},
	}
	for name, c := range cases {
		cfg := aiConfig(t)
		c.mut(cfg)
		if err := ValidateAICrawler(cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}
}

func TestCloudflareValidation(t *testing.T) {
	cases := map[string]struct {
		mut  func(*Config)
		want string
	}{
		"account":    {func(c *Config) { c.CloudflareAccountID = "abc" }, "AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID"},
		"gateway":    {func(c *Config) { c.CloudflareGatewayID = strings.Repeat("g", 65) }, "AI_CRAWLER_CLOUDFLARE_GATEWAY_ID"},
		"no gateway": {func(c *Config) { c.CloudflareGatewayID = "" }, "AI_CRAWLER_CLOUDFLARE_GATEWAY_ID"},
		"endpoint":   {func(c *Config) { c.CloudflareEndpoint = "v2" }, "AI_CRAWLER_CLOUDFLARE_ENDPOINT"},
		"http url":   {func(c *Config) { c.CloudflareBaseURL = "http://gateway.example/v1" }, "must use https"},
		"bad url":    {func(c *Config) { c.CloudflareBaseURL = "gateway" }, "AI_CRAWLER_CLOUDFLARE_BASE_URL"},
		"model":      {func(c *Config) { c.CloudflareModel = "deep seek" }, "AI_CRAWLER_CLOUDFLARE_MODEL"},
		"no model":   {func(c *Config) { c.CloudflareModel = "" }, "AI_CRAWLER_CLOUDFLARE_MODEL"},
		"reasoning":  {func(c *Config) { c.CloudflareReasoningEffort = "extreme" }, "REASONING_EFFORT"},
		"no token":   {func(c *Config) { c.CloudflareAPIToken = "" }, "the token is missing"},
		"interval":   {func(c *Config) { c.CloudflareMinIntervalMS = -1 }, "AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS"},
		"key on rest": {func(c *Config) { c.CloudflareProviderAPIKey = "sk-openai" },
			"cannot be used with APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT=rest"},
	}
	for name, c := range cases {
		cfg := aiConfig(t)
		c.mut(cfg)
		err := ValidateAICrawler(cfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), "cf-token") {
			t.Errorf("%s: the token appears in the error", name)
		}
	}

	// Accepted: localhost over http, compat with a provider key.
	cfg := aiConfig(t)
	cfg.CloudflareBaseURL, cfg.CloudflareEndpoint, cfg.CloudflareProviderAPIKey = "http://127.0.0.1:9/v1", "compat", "sk-openai"
	if err := ValidateAICrawler(cfg); err != nil || !cfg.CloudflareConfigured() {
		t.Errorf("compat with provider key: %v", err)
	}
}

func TestCloudflareSecretsFromFiles(t *testing.T) {
	dir := t.TempDir()
	tok := filepath.Join(dir, "cloudflare_api_token")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(tok, []byte("cf-file-token\n"), 0o600))
	cfg := aiConfig(t)
	cfg.CloudflareAPIToken, cfg.CloudflareAPITokenFile = "", tok
	if err := ValidateAICrawler(cfg); err != nil || cfg.CloudflareAPIToken != "cf-file-token" {
		t.Errorf("token from file: %v, %q", err, cfg.CloudflareAPIToken)
	}

	// A missing secret file leaves the engine unconfigured; it is not an
	// error while another engine serves.
	cfg = aiConfig(t)
	cfg.AICrawlerAPIKey, cfg.AICrawlerAPIKeyFile, cfg.DefaultEngine = "", filepath.Join(dir, "missing"), EngineAICloudflare
	if err := ValidateAICrawler(cfg); err != nil || cfg.AvalAIConfigured() || !cfg.CloudflareConfigured() {
		t.Errorf("missing avalai key file: %v, avalai %v", err, cfg.AvalAIConfigured())
	}
	// ... but it is named when that engine is the default.
	cfg.DefaultEngine = EngineAvalAI
	if err := ValidateAICrawler(cfg); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing avalai key file as default: %v", err)
	}
}
