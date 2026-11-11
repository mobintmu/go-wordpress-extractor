package extract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// Fetcher retrieves one page. *fetch.Fetcher implements it.
type Fetcher interface {
	Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*fetch.Page, *diagnose.Failure)
}

// Snapshotter returns the current site config snapshot.
type Snapshotter interface {
	Snapshot() *siteconfig.Snapshot
}

// Service runs the extraction pipeline.
type Service struct {
	sites   Snapshotter
	fetcher Fetcher
	limiter *limiter.Limiter
	log     *zap.Logger
	now     func() time.Time
}

// NewService builds the pipeline.
func NewService(reg *siteconfig.Registry, f *fetch.Fetcher, lim *limiter.Limiter, log *zap.Logger) *Service {
	return &Service{sites: reg, fetcher: f, limiter: lim, log: log, now: time.Now}
}

// NewServiceWith builds a pipeline from interfaces, for tests and tools.
func NewServiceWith(sites Snapshotter, f Fetcher, lim *limiter.Limiter, log *zap.Logger) *Service {
	return &Service{sites: sites, fetcher: f, limiter: lim, log: log, now: time.Now}
}

// ErrSiteBusy is returned when the site had no free slot in time; the
// handler answers HTTP 429.
var ErrSiteBusy = limiter.ErrSiteBusy

// Run processes one request. target must already be a valid absolute
// http(s) URL. It returns ErrSiteBusy instead of a result when the site
// slot could not be acquired.
func (s *Service) Run(ctx context.Context, requestID string, target *url.URL) (resp *Response, err error) {
	start := s.now().UTC()
	resp = &Response{RequestID: requestID, URL: target.String(), StartedAt: start}
	stage := diagnose.StageConfig

	defer func() {
		if r := recover(); r != nil {
			s.log.Error("panic in extraction", zap.Any("panic", r), zap.String("request_id", requestID), zap.ByteString("stack", debug.Stack()))
			s.fail(resp, diagnose.New(diagnose.InternalError, diagnose.ConfidenceHigh, "The service hit an unexpected error: %v.", r).At(stage), nil)
			err = nil
		}
		if resp == nil {
			return // site busy: the handler answers 429
		}
		resp.FinishedAt = s.now().UTC()
		resp.DurationMS = resp.FinishedAt.Sub(start).Milliseconds()
	}()

	// config
	entry := s.sites.Snapshot().Lookup(target.Hostname())
	if entry == nil {
		s.fail(resp, diagnose.New(diagnose.SiteNotConfigured, diagnose.ConfidenceHigh,
			"No file in configs/sites/ lists the host %s in its domains.", target.Hostname()), nil)
		return resp, nil
	}
	resp.SiteID, resp.ConfigVersion = entry.SiteID, entry.Version
	file := filepath.Base(entry.File)
	if entry.Err != nil {
		s.fail(resp, diagnose.New(diagnose.ConfigInvalid, diagnose.ConfidenceHigh,
			"The site config %s failed validation: %s.", file, strings.TrimSuffix(entry.Err.Error(), ".")).
			WithAction("Fix configs/sites/%s and run sitectl validate.", file), nil)
		return resp, nil
	}
	site := entry.Site
	if !site.IsEnabled() {
		s.fail(resp, diagnose.New(diagnose.SiteDisabled, diagnose.ConfidenceHigh,
			"The site config %s has enabled set to false.", file), nil)
		return resp, nil
	}
	if !site.MatchesProductPath(target.Path) {
		s.fail(resp, notProduct(target, site, ""), nil)
		return resp, nil
	}

	// Wait for this site's turn.
	release, aerr := s.limiter.AcquireSite(ctx, site.SiteID)
	if errors.Is(aerr, limiter.ErrSiteBusy) {
		return nil, ErrSiteBusy
	}
	if aerr != nil {
		s.fail(resp, diagnose.New(diagnose.DeadlineExceeded, diagnose.ConfidenceHigh,
			"The request's time limit was reached while waiting for the site's turn.").At(diagnose.StageFetch), nil)
		return resp, nil
	}
	defer release()

	// fetch
	stage = diagnose.StageFetch
	page, fail := s.fetcher.Fetch(ctx, site, target)
	if fail != nil {
		s.fail(resp, fail, evidenceFrom(page))
		return resp, nil
	}
	final := page.FinalURL
	if final == nil {
		final = target
	}
	if final.String() != target.String() && !site.MatchesProductPath(final.Path) {
		s.fail(resp, notProduct(final, site, target.String()), evidenceFrom(page))
		return resp, nil
	}

	// page_check, extract, normalize, validate
	stage = diagnose.StagePageCheck
	doc, perr := goquery.NewDocumentFromReader(bytes.NewReader(page.Body))
	if perr != nil {
		s.fail(resp, diagnose.New(diagnose.InternalError, diagnose.ConfidenceHigh, "The HTML could not be parsed: %v.", perr).At(stage), evidenceFrom(page))
		return resp, nil
	}
	out := parsePage(PageInput{Site: site, File: file, Doc: doc, Body: page.Body, Headers: page.Headers, Status: page.Status, FinalURL: final})
	if ctx.Err() != nil {
		s.fail(resp, diagnose.New(diagnose.DeadlineExceeded, diagnose.ConfidenceHigh,
			"The request's time limit was reached while parsing the page.").At(diagnose.StageExtract), evidenceFrom(page))
		return resp, nil
	}
	if out.failure != nil {
		ev := evidenceFrom(page)
		ev.SelectorTrace = out.trace
		if out.sample != nil {
			ev.HTMLSample = out.sample
		}
		if out.failure.Code == diagnose.RequiredFieldMissing || out.failure.Code == diagnose.PriceParseFailed {
			ev.FieldSamples = out.fields
		}
		s.fail(resp, out.failure, ev)
		if out.failure.Code == diagnose.RequiredFieldMissing || out.failure.Code == diagnose.PriceParseFailed {
			resp.Partial.Product = out.product
		}
		return resp, nil
	}
	resp.Status = StatusSuccess
	resp.Product = out.product
	resp.Warnings = out.warnings
	return resp, nil
}

func (s *Service) fail(resp *Response, f *diagnose.Failure, ev *diagnose.Evidence) {
	if ev == nil {
		ev = &diagnose.Evidence{FailedURL: resp.URL, Redirects: []string{}}
	}
	if f.Stage == "" {
		f.Stage = diagnose.StageConfig
	}
	resp.Status = StatusFailed
	resp.Error = f
	resp.Evidence = ev
	resp.Partial = &Partial{}
	resp.Product = nil
	resp.Warnings = nil
}

func notProduct(u *url.URL, site *siteconfig.Site, redirectedFrom string) *diagnose.Failure {
	if redirectedFrom != "" {
		return diagnose.New(diagnose.NotProductURL, diagnose.ConfidenceHigh,
			"%s redirected to %s, whose path matches none of the product page patterns %q. This does not mean the product is gone.",
			redirectedFrom, u.String(), site.Pages.Product.Match)
	}
	return diagnose.New(diagnose.NotProductURL, diagnose.ConfidenceHigh,
		"The path %q matches none of the site's product page patterns %q.", u.Path, site.Pages.Product.Match)
}

func evidenceFrom(p *fetch.Page) *diagnose.Evidence {
	ev := &diagnose.Evidence{Redirects: []string{}}
	if p == nil {
		return ev
	}
	ev.FailedURL = p.RequestedURL
	ev.FinalURL = p.RequestedURL
	if p.FinalURL != nil {
		ev.FinalURL = p.FinalURL.String()
	}
	if len(p.Redirects) > 0 {
		ev.Redirects = append(ev.Redirects, p.Redirects...)
	}
	ev.HTTPStatus = p.Status
	ev.Attempts = p.Attempts
	ev.ContentType = p.ContentType()
	ev.BodyBytes = p.BodyLength
	if len(p.Body) > 0 {
		sum := sha256.Sum256(p.Body)
		ev.BodySHA256 = hex.EncodeToString(sum[:])
		ev.HTMLSample = diagnose.NewSample(diagnose.ReasonBodyStart, string(p.Body))
		if doc, err := goquery.NewDocumentFromReader(bytes.NewReader(p.Body)); err == nil && looksHTML(p) {
			ev.PageTitle = pageTitle(doc)
		}
	}
	ev.ResponseHeaders = diagnose.FilterHeaders(p.Headers)
	return ev
}

func looksHTML(p *fetch.Page) bool {
	ct := strings.ToLower(p.ContentType())
	return ct == "" || strings.Contains(ct, "html")
}

func pageTitle(doc *goquery.Document) string {
	return diagnose.TruncateText(strings.Join(strings.Fields(doc.Find("title").First().Text()), " "), 300)
}

// String summarizes a response for logs.
func (r *Response) String() string {
	if r.Error != nil {
		return fmt.Sprintf("%s %s %s", r.Status, r.Error.Code, r.URL)
	}
	return fmt.Sprintf("%s %s", r.Status, r.URL)
}
