package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"

	"go-wordpress/internal/aicrawler"
	"go-wordpress/internal/config"
	"go-wordpress/internal/extract"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"github.com/openai/openai-go/v3/option"
	"go.uber.org/zap"
)

type diagnosticSnapshot struct{ snapshot *siteconfig.Snapshot }

func (s diagnosticSnapshot) Snapshot() *siteconfig.Snapshot { return s.snapshot }

// DiagnoseGapGPT runs the built-in GapGPT extraction rule for one URL and
// prints the provider exchanges followed by the extraction result.
func DiagnoseGapGPT(ctx context.Context, rawURL string, out io.Writer) error {
	target, err := extract.ValidateURL(rawURL)
	if err != nil {
		return err
	}
	cfg, err := config.NewGapGPTDiagnosticConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.MaxRequestDuration)
	defer cancel()

	log := zap.NewNop()
	lim := limiter.New(cfg)
	opts := fetchOptions(cfg)
	browser := browserFetcher(cfg, opts, lim, log)
	if browser != nil {
		defer browser.Stop(context.Background())
	}
	trace := &gapgptTrace{out: out, key: cfg.GapGPTAPIKey}
	settings := aiSettings(cfg, cfg.AICrawlerMinIntervalMS)
	settings.Middleware = trace.exchange
	client := gapgptClient(cfg, settings)
	engine := fetch.NewAIEngine(fetch.New(opts, lim, log), browser, client, aicrawler.NewCache(0), cfg.AICrawlerMaxInputChars, log)
	snapshot := siteconfig.NewSnapshot(nil)
	snapshot.Default = siteconfig.DefaultAIEntry(config.EngineGapGPT, cfg.AICrawlerDefaultDelayMS)
	svc := extract.NewServiceWith(diagnosticSnapshot{snapshot}, engine, lim, log)

	fmt.Fprintf(out, "GapGPT diagnostic: model %s | per-attempt timeout %s | extraction limit %s\n",
		cfg.GapGPTModel, settings.Timeout, cfg.MaxRequestDuration)
	resp, err := svc.Run(ctx, "gapgpt-diagnostic", target)
	if err != nil {
		return err
	}
	result, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	if trace.attempt == 0 {
		fmt.Fprintln(out, "\nThe extraction ended before calling GapGPT.")
	}
	fmt.Fprintf(out, "\nExtraction result\n%s\n", trace.redact(string(result)))
	if resp.Status == extract.StatusFailed {
		return errors.New("GapGPT diagnostic extraction failed")
	}
	return nil
}

type gapgptTrace struct {
	out     io.Writer
	key     string
	attempt int
}

func (t *gapgptTrace) redact(value string) string {
	if t.key == "" {
		return value
	}
	return strings.ReplaceAll(value, t.key, "[redacted]")
}

func (t *gapgptTrace) printBody(body []byte) {
	value := t.redact(string(body))
	if json.Valid(body) {
		fmt.Fprintln(t.out, value)
	} else {
		fmt.Fprintf(t.out, "%q\n", value)
	}
}

func (t *gapgptTrace) exchange(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	t.attempt++
	endpoint := *req.URL
	endpoint.User = nil
	fmt.Fprintf(t.out, "\nGapGPT request %d: %s %s\nContent-Type: %q\nAuthorization: [redacted]\n",
		t.attempt, req.Method, t.redact(endpoint.String()), req.Header.Get("Content-Type"))
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			var data []byte
			data, err = io.ReadAll(body)
			_ = body.Close()
			if err == nil {
				t.printBody(data)
			}
		}
		if err != nil {
			fmt.Fprintf(t.out, "Cannot copy request body: %q\n", t.redact(err.Error()))
		}
	}

	start := time.Now()
	var connected, written atomic.Bool
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		GotConn:      func(httptrace.GotConnInfo) { connected.Store(true) },
		WroteRequest: func(info httptrace.WroteRequestInfo) { written.Store(info.Err == nil) },
	}))
	res, err := next(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if res == nil {
		fmt.Fprintf(t.out, "\nGapGPT response %d: no HTTP response after %s (connected=%t, request_written=%t)\n",
			t.attempt, elapsed, connected.Load(), written.Load())
	} else {
		fmt.Fprintf(t.out, "\nGapGPT response %d: HTTP %d (headers after %s)\nContent-Type: %q\nX-Request-Id: %q\n",
			t.attempt, res.StatusCode, elapsed, t.redact(res.Header.Get("Content-Type")), t.redact(res.Header.Get("X-Request-Id")))
		if res.Body != nil {
			data, readErr := io.ReadAll(res.Body)
			_ = res.Body.Close()
			res.Body = io.NopCloser(bytes.NewReader(data))
			t.printBody(data)
			if err == nil {
				err = readErr
			}
		}
	}
	if err != nil {
		fmt.Fprintf(t.out, "GapGPT HTTP error: %q\n", t.redact(err.Error()))
	}
	return res, err
}
