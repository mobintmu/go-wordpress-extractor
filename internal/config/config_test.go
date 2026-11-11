package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func valid(t *testing.T) *Config {
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys.json")
	os.WriteFile(keys, []byte(`{"version":1,"keys":[]}`), 0o600)
	return &Config{
		ENV: "test", HTTPPort: 4000, SiteConfigDir: dir, APIKeysFile: keys,
		APIKeyPepper: strings.Repeat("p", 32), MaxRequestDuration: 3 * time.Minute,
		SiteWaitTimeout: 30 * time.Second, MaxConcurrentRequests: 20, MaxConcurrentPerSite: 1,
		ReloadInterval: 30 * time.Second,
	}
}

func TestValidateConfig(t *testing.T) {
	if err := ValidateConfig(valid(t)); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := map[string]struct {
		mut  func(*Config)
		want string
	}{
		"env":           {func(c *Config) { c.ENV = "staging" }, "invalid ENV"},
		"port":          {func(c *Config) { c.HTTPPort = 70000 }, "HTTP_PORT"},
		"address":       {func(c *Config) { c.HTTPAddress = "not-an-ip" }, "HTTP_ADDRESS"},
		"site dir":      {func(c *Config) { c.SiteConfigDir = "/nonexistent" }, "SITE_CONFIG_DIR"},
		"key file":      {func(c *Config) { c.APIKeysFile = "/nonexistent/keys" }, "API_KEYS_FILE"},
		"short pepper":  {func(c *Config) { c.APIKeyPepper = "short" }, "API_KEY_PEPPER"},
		"half tls":      {func(c *Config) { c.TLSCertFile = "cert.pem" }, "must be set together"},
		"duration":      {func(c *Config) { c.MaxRequestDuration = time.Hour }, "MAX_REQUEST_DURATION"},
		"site wait":     {func(c *Config) { c.SiteWaitTimeout = 5 * time.Minute }, "SITE_WAIT_TIMEOUT"},
		"capacity":      {func(c *Config) { c.MaxConcurrentRequests = 0 }, "MAX_CONCURRENT_REQUESTS"},
		"site capacity": {func(c *Config) { c.MaxConcurrentPerSite = 0 }, "MAX_CONCURRENT_PER_SITE"},
	}
	for name, c := range cases {
		cfg := valid(t)
		c.mut(cfg)
		if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}
}

func TestDefaultsFromEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_API_KEYS_FILE", "")
	t.Setenv("APP_MAX_REQUEST_DURATION", "")
	cfg := buildConfig(newViper())
	if cfg.APIKeysFile != "secrets/.env.apikeys" || cfg.HTTPPort != 4000 || cfg.MaxRequestDuration != 3*time.Minute ||
		cfg.MaxConcurrentRequests != 20 || cfg.MaxConcurrentPerSite != 1 || cfg.SiteWaitTimeout != 30*time.Second {
		t.Fatalf("defaults: %+v", cfg)
	}
	t.Setenv("APP_ENV", "development")
	if cfg := buildConfig(newViper()); cfg.APIKeysFile != "secrets/api_keys.json" {
		t.Fatalf("dev key file = %s", cfg.APIKeysFile)
	}
}

func TestEnvFileFillsEmptyButKeepsRealValues(t *testing.T) {
	t.Setenv("APP_OPS_USER", "")          // exported but empty
	t.Setenv("APP_HTTP_PORT", "5000")     // real value
	t.Setenv("APP_SITE_WAIT_TIMEOUT", "") // cleared by Setenv, restored after
	os.Unsetenv("APP_SITE_WAIT_TIMEOUT")
	applyEnvFile(map[string]string{"APP_OPS_USER": "admin", "APP_HTTP_PORT": "4000", "APP_SITE_WAIT_TIMEOUT": "10s"})
	if os.Getenv("APP_OPS_USER") != "admin" || os.Getenv("APP_HTTP_PORT") != "5000" || os.Getenv("APP_SITE_WAIT_TIMEOUT") != "10s" {
		t.Fatalf("got user=%q port=%q wait=%q", os.Getenv("APP_OPS_USER"), os.Getenv("APP_HTTP_PORT"), os.Getenv("APP_SITE_WAIT_TIMEOUT"))
	}
}
