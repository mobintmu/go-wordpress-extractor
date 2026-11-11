package sitectl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/siteconfig"
)

var repoConfigs = filepath.Join("..", "..", "configs")
var repoFixtures = filepath.Join("..", "..", "testdata", "sites")

// TestRepositoryFixtures is what CI runs: every site's saved HTML must
// still produce its expected output.
func TestRepositoryFixtures(t *testing.T) {
	results, err := RunFixtures(repoConfigs, repoFixtures)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, r := range results {
		if !r.Pass {
			t.Errorf("%s/%s:\n%s", r.Site, r.Fixture, r.Diff)
		}
	}
}

func run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := CLI{Stdout: &out, Stderr: &errOut, Fetcher: stubFetcher{}}.Run(args)
	return code, out.String(), errOut.String()
}

type stubFetcher struct{}

func (stubFetcher) Fetch(_ context.Context, _ *siteconfig.Site, u *url.URL) (*fetch.Page, *diagnose.Failure) {
	body, _ := os.ReadFile(filepath.Join(repoFixtures, "badomjip", "product-sale.html"))
	return &fetch.Page{RequestedURL: u.String(), FinalURL: u, Status: http.StatusOK, Attempts: 1,
		Headers: http.Header{"Content-Type": {"text/html"}}, Body: body, BodyLength: int64(len(body))}, nil
}

func TestCommands(t *testing.T) {
	if code, out, _ := run("validate", "--dir", repoConfigs, "--json"); code != 0 || !strings.Contains(out, `"valid": true`) {
		t.Fatalf("validate: %d %s", code, out)
	}
	if code, out, _ := run("coverage", "--dir", repoConfigs, "--fixtures", repoFixtures); code != 0 || !strings.Contains(out, "badomjip") {
		t.Fatalf("coverage: %d %s", code, out)
	}
	if code, out, _ := run("test", "--dir", repoConfigs, "--fixtures", repoFixtures); code != 0 || !strings.Contains(out, "0 failed") {
		t.Fatalf("test: %d %s", code, out)
	}
	code, out, _ := run("check", "--dir", repoConfigs, "--url", "https://badomjip.com/product/premium-almond/", "--json")
	var resp map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &resp) != nil || resp["status"] != "success" {
		t.Fatalf("check: %d %s", code, out)
	}
	if code, out, _ := run("check", "--dir", repoConfigs, "--url", "https://badomjip.com/product-category/x/"); code != 1 || !strings.Contains(out, "NOT_PRODUCT_URL") {
		t.Fatalf("check on a category page: %d %s", code, out)
	}
}

func TestFixtureRecordsAndReplays(t *testing.T) {
	fixtures := t.TempDir()
	args := []string{"fixture", "--dir", repoConfigs, "--fixtures", fixtures, "--url", "https://badomjip.com/product/premium-almond/", "--name", "recorded"}
	if code, out, errOut := run(args...); code != 0 {
		t.Fatalf("fixture: %s %s", out, errOut)
	}
	for _, f := range []string{"recorded.html", "recorded.json"} {
		if _, err := os.Stat(filepath.Join(fixtures, "badomjip", f)); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, errOut := run(args...); code == 0 || !strings.Contains(errOut, "already exists") {
		t.Fatalf("fixture must not overwrite without --force: %s", errOut)
	}
	results, err := RunFixtures(repoConfigs, fixtures)
	if err != nil {
		t.Fatal(err)
	}
	passed, missing := 0, 0
	for _, r := range results {
		if r.Pass {
			passed++
		} else if r.Fixture == "-" {
			missing++
		}
	}
	if passed != 1 || missing != 1 {
		t.Fatalf("want the recorded fixture to pass and hosseinibrothers reported as missing fixtures: %+v", results)
	}
}

func TestValidateReportsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sites"), 0o755)
	os.MkdirAll(filepath.Join(dir, "templates"), 0o755)
	os.WriteFile(filepath.Join(dir, "sites", "x.test.json"), []byte(`{"schema_version":2}`), 0o644)
	code, out, _ := run("validate", "--dir", dir)
	if code != 1 || !strings.Contains(out, "INVALID") || !strings.Contains(out, "schema_version must be 1") {
		t.Fatalf("validate: %d %s", code, out)
	}
}
