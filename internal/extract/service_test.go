package extract

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"go.uber.org/zap"
)

type snap struct{ s *siteconfig.Snapshot }

func (s snap) Snapshot() *siteconfig.Snapshot { return s.s }

type panicFetcher struct{}

func (panicFetcher) Fetch(context.Context, *siteconfig.Site, *url.URL) (*fetch.Page, *diagnose.Failure) {
	panic("boom")
}

func repoService(t *testing.T, f Fetcher) *Service {
	entries, err := siteconfig.LoadDir(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	lim := limiter.New(&config.Config{MaxConcurrentRequests: 2, MaxConcurrentPerSite: 1, SiteWaitTimeout: time.Second})
	return NewServiceWith(snap{siteconfig.NewSnapshot(entries)}, f, lim, zap.NewNop())
}

func TestPanicBecomesInternalError(t *testing.T) {
	u, _ := url.Parse("https://badomjip.com/product/x/")
	resp, err := repoService(t, panicFetcher{}).Run(context.Background(), "r1", u)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusFailed || resp.Error.Code != diagnose.InternalError || resp.Error.Stage != diagnose.StageFetch || !resp.Error.Retryable {
		t.Fatalf("got %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.RootCause, "boom") || resp.FinishedAt.IsZero() {
		t.Fatalf("root cause / timing missing: %+v", resp)
	}
}

func TestValidateURL(t *testing.T) {
	good := map[string]string{
		"https://Shop.Test/product/a/#frag": "https://shop.test/product/a/",
		"HTTP://shop.test:8080/p":           "http://shop.test:8080/p",
	}
	for in, want := range good {
		u, err := ValidateURL(in)
		if err != nil || u.String() != want {
			t.Errorf("ValidateURL(%q) = %v, %v; want %s", in, u, err, want)
		}
	}
	for _, bad := range []string{"", "shop.test/p", "ftp://shop.test/", "mailto:a@b.c", "http://", "http://u:p@shop.test/", "http://shop.test:99999/", "http://shop.test:0/"} {
		if _, err := ValidateURL(bad); err == nil {
			t.Errorf("ValidateURL(%q) should fail", bad)
		}
	}
}

func TestResponseJSONShape(t *testing.T) {
	ok, _ := json.Marshal(Response{Status: StatusSuccess, Product: &Product{}})
	if !strings.Contains(string(ok), `"warnings":[]`) || strings.Contains(string(ok), `"error"`) {
		t.Fatalf("success shape: %s", ok)
	}
	failed, _ := json.Marshal(Response{Status: StatusFailed, Partial: &Partial{}})
	if strings.Contains(string(failed), `"warnings"`) || !strings.Contains(string(failed), `"partial":{"product":null}`) {
		t.Fatalf("failed shape: %s", failed)
	}
}
