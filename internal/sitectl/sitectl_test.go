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
	passed, others, missing := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Site == "badomjip" && r.Pass:
			passed++
		case r.Site != "badomjip":
			others++
			if !r.Pass && r.Fixture == "-" {
				missing++
			}
		}
	}
	if passed != 1 || len(results) != passed+others || others == 0 || missing != others {
		t.Fatalf("want the recorded fixture to pass and every other site reported as missing fixtures: %+v", results)
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

// TestHTMLFileSkipsTheFetcher: --html must use the file, never the network.
func TestHTMLFileSkipsTheFetcher(t *testing.T) {
	html := filepath.Join(repoFixtures, "badomjip", "product-sale.html")
	fixtures := t.TempDir()
	cli := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := CLI{Stdout: &out, Stderr: &errOut, Fetcher: failFetcher{t}}.Run(args)
		return code, out.String(), errOut.String()
	}
	u := "https://badomjip.com/product/premium-almond/"
	if code, out, errOut := cli("check", "--dir", repoConfigs, "--url", u, "--html", html, "--json"); code != 0 || !strings.Contains(out, `"status": "success"`) {
		t.Fatalf("check --html: %d %s %s", code, out, errOut)
	}
	if code, out, errOut := cli("fixture", "--dir", repoConfigs, "--fixtures", fixtures, "--url", u, "--html", html, "--name", "fromfile"); code != 0 {
		t.Fatalf("fixture --html: %s %s", out, errOut)
	}
	want, _ := os.ReadFile(html)
	got, err := os.ReadFile(filepath.Join(fixtures, "badomjip", "fromfile.html"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("saved HTML must equal the input file: %v", err)
	}
	if code, _, errOut := cli("check", "--dir", repoConfigs, "--url", u, "--html", filepath.Join(fixtures, "missing.html")); code != 1 || errOut == "" {
		t.Fatalf("missing --html file must fail: %d %s", code, errOut)
	}
}

type failFetcher struct{ t *testing.T }

func (f failFetcher) Fetch(_ context.Context, _ *siteconfig.Site, u *url.URL) (*fetch.Page, *diagnose.Failure) {
	f.t.Errorf("fetcher called for %s although --html was given", u)
	return nil, nil
}
