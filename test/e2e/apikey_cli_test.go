package e2e

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-wordpress/internal/apikey"
)

func runCLI(t *testing.T, env map[string]string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := apikey.CLI{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Getenv: func(k string) string { return env[k] },
		Now:    time.Now,
	}.Run(args)
	return code, out.String(), errOut.String()
}

// TestAPIKeyLifecycle follows the production runbook: generate keys in
// bulk into a .env file, start the service on that file, call it with a
// key from the CSV, then revoke the key and see it rejected.
func TestAPIKeyLifecycle(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, ".env.apikeys")
	csvFile := filepath.Join(dir, "handover.csv")
	env := map[string]string{"APP_API_KEY_PEPPER": testPepper, "APP_API_KEYS_FILE": keyFile}

	code, out, stderr := runCLI(t, env, "", "generate-bulk", "--count", "3", "--name-prefix", "app", "--expires-in", "8760h", "--plain-output", csvFile)
	if code != 0 {
		t.Fatalf("generate-bulk: %d %s", code, stderr)
	}
	if strings.Contains(out, apikey.Prefix) {
		t.Fatalf("full keys must not reach stdout when --plain-output is given:\n%s", out)
	}

	st, err := os.Stat(csvFile)
	must(t, err)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("CSV permission = %o, want 600", st.Mode().Perm())
	}
	st, err = os.Stat(keyFile)
	must(t, err)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("key file permission = %o, want 600", st.Mode().Perm())
	}

	f, err := os.Open(csvFile)
	must(t, err)
	rows, err := csv.NewReader(f).ReadAll()
	f.Close()
	must(t, err)
	if len(rows) != 4 || strings.Join(rows[0], ",") != "id,name,key,expires_at" || rows[1][1] != "app-001" || rows[3][1] != "app-003" {
		t.Fatalf("csv rows: %v", rows)
	}
	key := rows[1][2]

	raw, err := os.ReadFile(keyFile)
	must(t, err)
	if strings.Contains(string(raw), key) || strings.Contains(string(raw), apikey.Prefix) {
		t.Fatalf("key file must hold hashes only:\n%s", raw)
	}
	if !strings.HasPrefix(string(raw), "# Managed by the apikey command") || strings.Count(string(raw), "APIKEY_") != 3 {
		t.Fatalf("unexpected .env content:\n%s", raw)
	}

	// The service reads the .env file directly.
	e := Start(t, Options{Env: map[string]string{"APP_API_KEYS_FILE": keyFile}})
	e.Post("/api/v1/extract", key, map[string]any{"url": e.Shop.URL(hostShop, "/product/ok/")}).Success(t)

	code, out, _ = runCLI(t, env, "", "list", "--json")
	if code != 0 || strings.Contains(out, "hash") {
		t.Fatalf("list must not show hashes: %s", out)
	}
	var listed []map[string]any
	must(t, json.Unmarshal([]byte(out), &listed))
	if len(listed) != 3 {
		t.Fatalf("list: %s", out)
	}

	if code, _, stderr = runCLI(t, env, "", "revoke", "--id", rows[1][0]); code != 0 {
		t.Fatalf("revoke: %s", stderr)
	}
	eventually(t, func() bool {
		return e.Post("/api/v1/extract", key, map[string]any{"url": e.Shop.URL(hostShop, "/product/ok/")}).Status == 401
	})
	e.Post("/api/v1/extract", rows[2][2], map[string]any{"url": e.Shop.URL(hostShop, "/product/ok/")}).Success(t)
}

func TestAPIKeyCLIRules(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.json")
	env := map[string]string{"APP_API_KEY_PEPPER": testPepper, "APP_API_KEYS_FILE": keyFile}

	t.Run("pepper is required", func(t *testing.T) {
		code, _, stderr := runCLI(t, map[string]string{"APP_API_KEYS_FILE": keyFile}, "", "generate", "--name", "x")
		if code == 0 || !strings.Contains(stderr, "APP_API_KEY_PEPPER") {
			t.Fatalf("want pepper error, got %d %s", code, stderr)
		}
	})
	t.Run("generate-pepper prints a long random value", func(t *testing.T) {
		_, a, _ := runCLI(t, nil, "", "generate-pepper")
		_, b, _ := runCLI(t, nil, "", "generate-pepper")
		if len(strings.TrimSpace(a)) < 32 || a == b {
			t.Fatalf("pepper: %q %q", a, b)
		}
	})
	t.Run("generate shows the key once and stores only the hash", func(t *testing.T) {
		code, out, stderr := runCLI(t, env, "", "generate", "--name", "catalog-service", "--json")
		if code != 0 {
			t.Fatal(stderr)
		}
		var res struct {
			Keys []struct{ ID, Key string }
		}
		must(t, json.Unmarshal([]byte(out), &res))
		if len(res.Keys) != 1 || !strings.HasPrefix(res.Keys[0].Key, apikey.Prefix+res.Keys[0].ID+"_") {
			t.Fatalf("output: %s", out)
		}
		entries, _, err := apikey.ReadFile(keyFile, false)
		must(t, err)
		if len(entries) != 1 || entries[0].Hash != apikey.Hash(testPepper, res.Keys[0].Key) {
			t.Fatalf("stored entry does not match the key")
		}
	})
	t.Run("duplicate name stops the run", func(t *testing.T) {
		code, _, stderr := runCLI(t, env, "", "generate", "--name", "catalog-service")
		if code == 0 || !strings.Contains(stderr, "already exists") {
			t.Fatalf("want duplicate error: %s", stderr)
		}
	})
	t.Run("bulk run is all or nothing", func(t *testing.T) {
		names := filepath.Join(dir, "names.txt")
		must(t, os.WriteFile(names, []byte("# clients\npricing-service\n\ncatalog-service\n"), 0o600))
		out := filepath.Join(dir, "bulk.csv")
		code, _, stderr := runCLI(t, env, "", "generate-bulk", "--names-file", names, "--plain-output", out)
		if code == 0 || !strings.Contains(stderr, "catalog-service") {
			t.Fatalf("want conflict error, got %d %s", code, stderr)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Errorf("CSV must not be left behind when no keys were created")
		}
		entries, _, _ := apikey.ReadFile(keyFile, false)
		if len(entries) != 1 {
			t.Errorf("key file changed: %d entries", len(entries))
		}
	})
	t.Run("existing CSV is never overwritten", func(t *testing.T) {
		out := filepath.Join(dir, "exists.csv")
		must(t, os.WriteFile(out, []byte("keep"), 0o600))
		code, _, stderr := runCLI(t, env, "", "generate-bulk", "--count", "2", "--name-prefix", "x", "--plain-output", out)
		if code == 0 || !strings.Contains(stderr, "refusing to overwrite") {
			t.Fatalf("got %d %s", code, stderr)
		}
		if b, _ := os.ReadFile(out); string(b) != "keep" {
			t.Errorf("CSV was overwritten")
		}
	})
	t.Run("revoke by name prefix", func(t *testing.T) {
		out := filepath.Join(dir, "team.csv")
		if code, _, stderr := runCLI(t, env, "", "generate-bulk", "--count", "2", "--name-prefix", "team", "--expires-in", "never", "--plain-output", out); code != 0 {
			t.Fatal(stderr)
		}
		code, stdout, _ := runCLI(t, env, "", "revoke", "--name-prefix", "team")
		if code != 0 || !strings.Contains(stdout, "revoked 2 key(s)") {
			t.Fatalf("revoke: %s", stdout)
		}
		if code, _, _ := runCLI(t, env, "", "revoke", "--name-prefix", "team"); code == 0 {
			t.Errorf("revoking nothing should fail")
		}
	})
	t.Run("hash-ops-password", func(t *testing.T) {
		code, out, _ := runCLI(t, env, "a long operator password\n", "hash-ops-password")
		if code != 0 || strings.TrimSpace(out) != apikey.Hash(testPepper, "a long operator password") {
			t.Fatalf("hash: %q", out)
		}
		if code, _, _ := runCLI(t, env, "short\n", "hash-ops-password"); code == 0 {
			t.Errorf("short passwords must be refused")
		}
	})
}

func TestStartupRejectsBadKeyFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, ".env.apikeys")
	must(t, os.WriteFile(bad, []byte("APIKEY_ABC=hmac-sha256,active\n"), 0o600))
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_HTTP_PORT", "1")
	t.Setenv("APP_SITE_CONFIG_DIR", filepath.Join("..", "..", "configs"))
	t.Setenv("APP_API_KEYS_FILE", bad)
	t.Setenv("APP_API_KEY_PEPPER", testPepper)
	t.Setenv("APP_RELOAD_INTERVAL", "1s")
	if err := newAppForStartupTest().Err(); err == nil || !strings.Contains(err.Error(), "6 comma-separated values") {
		t.Fatalf("want startup error for a bad key file, got %v", err)
	}
}
