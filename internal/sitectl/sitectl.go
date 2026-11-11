// Package sitectl implements the sitectl command: validate site configs,
// run live checks, record fixtures, and run fixtures offline.
package sitectl

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/extract"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"go.uber.org/zap"
)

const usage = `Usage: sitectl <command> [flags]

Commands:
  validate                 Check schema and references for all files
  check    --url <url>     Run a real extraction on one product page
  fixture  --url <url> --name <name>
                           Save the page's HTML and expected output for testing
  test                     Run all fixtures offline
  coverage                 List sites with status, version, template, fixtures

Common flags: --dir configs  --fixtures testdata/sites  --json
`

// CLI runs sitectl. Fetcher is used by check and fixture; nil means the
// production fetcher.
type CLI struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Fetcher extract.Fetcher
}

// Fixture is one saved test case: <name>.html plus <name>.json.
type Fixture struct {
	URL      string    `json:"url"`
	FinalURL string    `json:"final_url,omitempty"`
	Expect   Expected  `json:"expect"`
	Saved    time.Time `json:"saved_at,omitempty"`
}

// Expected is the part of a response a fixture pins down.
type Expected struct {
	Status    string            `json:"status"`
	ErrorCode string            `json:"error_code,omitempty"`
	Product   *extract.Product  `json:"product,omitempty"`
	Warnings  []extract.Warning `json:"warnings,omitempty"`
}

// Run executes args and returns the exit code.
func (c CLI) Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(c.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "validate":
		err = c.validate(args[1:])
	case "check":
		err = c.check(args[1:])
	case "fixture":
		err = c.fixture(args[1:])
	case "test":
		err = c.test(args[1:])
	case "coverage":
		err = c.coverage(args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(c.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(c.Stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if !errors.Is(err, errReported) {
			fmt.Fprintf(c.Stderr, "error: %v\n", err)
		}
		return 1
	}
	return 0
}

// errReported means the command already printed why it failed.
var errReported = errors.New("reported")

type common struct {
	dir      *string
	fixtures *string
	asJSON   *bool
}

func (c CLI) flags(name string) (*flag.FlagSet, common) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	return fs, common{
		dir:      fs.String("dir", envOr("APP_SITE_CONFIG_DIR", "configs"), "site config root"),
		fixtures: fs.String("fixtures", "testdata/sites", "fixture root"),
		asJSON:   fs.Bool("json", false, "print JSON"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c CLI) printJSON(v any) error {
	enc := json.NewEncoder(c.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// ---- validate ----

type siteRow struct {
	File     string   `json:"file"`
	SiteID   string   `json:"site_id"`
	Version  string   `json:"version"`
	Template string   `json:"template,omitempty"`
	Status   string   `json:"status"`
	Domains  []string `json:"domains"`
	Error    string   `json:"error,omitempty"`
	Fixtures int      `json:"fixtures"`
}

func rows(entries []*siteconfig.Entry, fixtureRoot string) []siteRow {
	out := make([]siteRow, 0, len(entries))
	for _, e := range entries {
		r := siteRow{File: filepath.Base(e.File), SiteID: e.SiteID, Version: e.Version, Template: e.Template, Status: e.Status(), Domains: e.Domains}
		if e.Err != nil {
			r.Error = e.Err.Error()
		}
		if fixtureRoot != "" {
			files, _ := filepath.Glob(filepath.Join(fixtureRoot, e.SiteID, "*.json"))
			r.Fixtures = len(files)
		}
		out = append(out, r)
	}
	return out
}

func (c CLI) validate(args []string) error {
	fs, cm := c.flags("validate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entries, err := siteconfig.LoadDir(*cm.dir)
	if err != nil {
		return err
	}
	rs := rows(entries, "")
	invalid := 0
	for _, r := range rs {
		if r.Status == "invalid" {
			invalid++
		}
	}
	if *cm.asJSON {
		if err := c.printJSON(map[string]any{"valid": invalid == 0, "sites": rs}); err != nil {
			return err
		}
	} else {
		for _, r := range rs {
			if r.Error != "" {
				fmt.Fprintf(c.Stdout, "INVALID  %s: %s\n", r.File, r.Error)
			} else {
				fmt.Fprintf(c.Stdout, "ok       %s (%s, version %s, %s)\n", r.File, r.SiteID, r.Version, r.Status)
			}
		}
		fmt.Fprintf(c.Stdout, "%d sites, %d invalid\n", len(rs), invalid)
	}
	if invalid > 0 {
		return errReported
	}
	return nil
}

// ---- coverage ----

func (c CLI) coverage(args []string) error {
	fs, cm := c.flags("coverage")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entries, err := siteconfig.LoadDir(*cm.dir)
	if err != nil {
		return err
	}
	rs := rows(entries, *cm.fixtures)
	if *cm.asJSON {
		return c.printJSON(rs)
	}
	tw := tabwriter.NewWriter(c.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SITE\tSTATUS\tVERSION\tTEMPLATE\tFIXTURES\tDOMAINS")
	for _, r := range rs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", r.SiteID, r.Status, r.Version, r.Template, r.Fixtures, strings.Join(r.Domains, ","))
	}
	return tw.Flush()
}

// ---- check and fixture ----

type staticSnapshot struct{ s *siteconfig.Snapshot }

func (s staticSnapshot) Snapshot() *siteconfig.Snapshot { return s.s }

func newLimiter() *limiter.Limiter {
	return limiter.New(&config.Config{MaxConcurrentRequests: 4, MaxConcurrentPerSite: 1, SiteWaitTimeout: time.Minute})
}

// recorder keeps the last fetched page so fixture can save it.
type recorder struct {
	next extract.Fetcher
	page *fetch.Page
}

func (r *recorder) Fetch(ctx context.Context, site *siteconfig.Site, u *url.URL) (*fetch.Page, *diagnose.Failure) {
	p, f := r.next.Fetch(ctx, site, u)
	r.page = p
	return p, f
}

func (c CLI) live(dir, rawURL string) (*extract.Response, *fetch.Page, error) {
	target, err := extract.ValidateURL(rawURL)
	if err != nil {
		return nil, nil, err
	}
	entries, err := siteconfig.LoadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	lim := newLimiter()
	f := c.Fetcher
	if f == nil {
		f = fetch.New(fetch.DefaultOptions(), lim, zap.NewNop())
	}
	rec := &recorder{next: f}
	svc := extract.NewServiceWith(staticSnapshot{siteconfig.NewSnapshot(entries)}, rec, lim, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultMaxRequestDuration)
	defer cancel()
	resp, err := svc.Run(ctx, "sitectl", target)
	if err != nil {
		return nil, nil, err
	}
	return resp, rec.page, nil
}

func (c CLI) check(args []string) error {
	fs, cm := c.flags("check")
	u := fs.String("url", "", "product page URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *u == "" {
		return errors.New("--url is required")
	}
	resp, _, err := c.live(*cm.dir, *u)
	if err != nil {
		return err
	}
	if !*cm.asJSON {
		fmt.Fprintf(c.Stdout, "%s: %s\n", strings.ToUpper(resp.Status), resp.URL)
		if resp.Error != nil {
			fmt.Fprintf(c.Stdout, "code:   %s (stage %s, owner %s)\ncause:  %s\naction: %s\n",
				resp.Error.Code, resp.Error.Stage, resp.Error.Owner, resp.Error.RootCause, resp.Error.SuggestedAction)
		}
	}
	if err := c.printJSON(resp); err != nil {
		return err
	}
	if resp.Status != extract.StatusSuccess {
		return errReported
	}
	return nil
}

func (c CLI) fixture(args []string) error {
	fs, cm := c.flags("fixture")
	u := fs.String("url", "", "product page URL")
	name := fs.String("name", "", "fixture name, for example product-sale")
	force := fs.Bool("force", false, "overwrite an existing fixture")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *u == "" || *name == "" {
		return errors.New("--url and --name are required")
	}
	if strings.ContainsAny(*name, `/\.`) {
		return errors.New("--name must not contain '/', '\\' or '.'")
	}
	resp, page, err := c.live(*cm.dir, *u)
	if err != nil {
		return err
	}
	if resp.SiteID == "" || page == nil || len(page.Body) == 0 {
		return fmt.Errorf("no page was fetched: %s", resp)
	}
	dir := filepath.Join(*cm.fixtures, resp.SiteID)
	htmlPath := filepath.Join(dir, *name+".html")
	jsonPath := filepath.Join(dir, *name+".json")
	if !*force && (siteconfig.Exists(htmlPath) || siteconfig.Exists(jsonPath)) {
		return fmt.Errorf("%s already exists; use --force to overwrite", jsonPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fx := Fixture{URL: resp.URL, Expect: expectedOf(resp), Saved: time.Now().UTC().Truncate(time.Second)}
	if page.FinalURL != nil && page.FinalURL.String() != resp.URL {
		fx.FinalURL = page.FinalURL.String()
	}
	b, err := json.MarshalIndent(fx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(htmlPath, page.Body, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(c.Stdout, "saved %s and %s (status %s). Review the expected output before committing.\n", htmlPath, jsonPath, resp.Status)
	return nil
}

func expectedOf(r *extract.Response) Expected {
	e := Expected{Status: r.Status, Product: r.Product, Warnings: r.Warnings}
	if r.Error != nil {
		e.ErrorCode = r.Error.Code
		if r.Partial != nil {
			e.Product = r.Partial.Product
		}
	}
	return e
}

// ---- test ----

// fileFetcher serves fixture HTML instead of the network.
type fileFetcher struct {
	body  []byte
	final *url.URL
}

func (f fileFetcher) Fetch(_ context.Context, _ *siteconfig.Site, u *url.URL) (*fetch.Page, *diagnose.Failure) {
	final := u
	if f.final != nil {
		final = f.final
	}
	return &fetch.Page{
		RequestedURL: u.String(),
		FinalURL:     final,
		Redirects:    []string{},
		Status:       http.StatusOK,
		Attempts:     1,
		Headers:      http.Header{"Content-Type": {"text/html; charset=UTF-8"}},
		Body:         f.body,
		BodyLength:   int64(len(f.body)),
	}, nil
}

// TestResult is the outcome of one fixture.
type TestResult struct {
	Site    string `json:"site"`
	Fixture string `json:"fixture"`
	Pass    bool   `json:"pass"`
	Diff    string `json:"diff,omitempty"`
}

// RunFixtures runs every fixture under root against the configs in dir.
// Enabled sites without fixtures are reported as failures.
func RunFixtures(dir, root string) ([]TestResult, error) {
	entries, err := siteconfig.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	snap := staticSnapshot{siteconfig.NewSnapshot(entries)}
	var results []TestResult
	for _, e := range entries {
		files, _ := filepath.Glob(filepath.Join(root, e.SiteID, "*.json"))
		sort.Strings(files)
		if len(files) == 0 && e.Err == nil && e.Site.IsEnabled() {
			results = append(results, TestResult{Site: e.SiteID, Fixture: "-", Diff: "site has no fixtures; add one with sitectl fixture"})
		}
		for _, jf := range files {
			results = append(results, runFixture(snap, e.SiteID, jf))
		}
	}
	return results, nil
}

func runFixture(snap staticSnapshot, site, jsonPath string) TestResult {
	name := strings.TrimSuffix(filepath.Base(jsonPath), ".json")
	res := TestResult{Site: site, Fixture: name}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		res.Diff = err.Error()
		return res
	}
	var fx Fixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		res.Diff = "invalid fixture JSON: " + err.Error()
		return res
	}
	body, err := os.ReadFile(strings.TrimSuffix(jsonPath, ".json") + ".html")
	if err != nil {
		res.Diff = err.Error()
		return res
	}
	target, err := extract.ValidateURL(fx.URL)
	if err != nil {
		res.Diff = "fixture url: " + err.Error()
		return res
	}
	ff := fileFetcher{body: body}
	if fx.FinalURL != "" {
		ff.final, _ = url.Parse(fx.FinalURL)
	}
	svc := extract.NewServiceWith(snap, ff, newLimiter(), zap.NewNop())
	resp, err := svc.Run(context.Background(), "fixture", target)
	if err != nil {
		res.Diff = err.Error()
		return res
	}
	got, _ := json.MarshalIndent(expectedOf(resp), "", "  ")
	want, _ := json.MarshalIndent(fx.Expect, "", "  ")
	if string(got) != string(want) {
		res.Diff = "expected:\n" + string(want) + "\ngot:\n" + string(got)
		return res
	}
	res.Pass = true
	return res
}

func (c CLI) test(args []string) error {
	fs, cm := c.flags("test")
	if err := fs.Parse(args); err != nil {
		return err
	}
	results, err := RunFixtures(*cm.dir, *cm.fixtures)
	if err != nil {
		return err
	}
	failed := 0
	for _, r := range results {
		if !r.Pass {
			failed++
		}
	}
	if *cm.asJSON {
		if err := c.printJSON(map[string]any{"passed": len(results) - failed, "failed": failed, "results": results}); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			if r.Pass {
				fmt.Fprintf(c.Stdout, "PASS  %s/%s\n", r.Site, r.Fixture)
			} else {
				fmt.Fprintf(c.Stdout, "FAIL  %s/%s\n%s\n", r.Site, r.Fixture, r.Diff)
			}
		}
		fmt.Fprintf(c.Stdout, "%d passed, %d failed\n", len(results)-failed, failed)
	}
	if failed > 0 {
		return errReported
	}
	return nil
}
