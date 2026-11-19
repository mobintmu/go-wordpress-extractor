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

	"go-wordpress/internal/aicrawler"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// Fetcher retrieves one page. *fetch.Router implements it by delegating to
// the fetch engine (strategy) the site config selects; each engine
// (*fetch.Fetcher, *fetch.BrowserFetcher) implements it too.
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
	// learner learns selectors from AI results. Nil when the AI crawler or
	// selector learning is off.
	learner *Learner
}

// NewService builds the pipeline. learner may be nil.
func NewService(reg *siteconfig.Registry, f Fetcher, lim *limiter.Limiter, log *zap.Logger, learner *Learner) *Service {
	return &Service{sites: reg, fetcher: f, limiter: lim, log: log, now: time.Now, learner: learner}
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

	// config: a site config, then an active learned config, then the
	// built-in AI rule for any other host (when it is on).
	snap := s.sites.Snapshot()
	entry := snap.Lookup(target.Hostname())
	if entry == nil {
		entry = snap.Default
	}
	if entry == nil {
		s.fail(resp, diagnose.New(diagnose.SiteNotConfigured, diagnose.ConfidenceHigh,
			"No file in configs/sites/ lists the host %s in its domains.", target.Hostname()), nil)
		return resp, nil
	}
	resp.SiteID, resp.ConfigVersion, resp.ConfigOrigin = entry.SiteID, entry.Version, entry.Origin
	file := filepath.Base(entry.File)
	if entry.Err != nil {
		s.fail(resp, diagnose.New(diagnose.ConfigInvalid, diagnose.ConfidenceHigh,
			"The site config %s failed validation: %s.", file, strings.TrimSuffix(entry.Err.Error(), ".")).
			WithAction("Fix configs/sites/%s and run sitectl validate.", file), nil)
		return resp, nil
	}
	site := entry.Site
	if entry.Origin == siteconfig.OriginDefault {
		site = site.ForHost(target.Hostname())
	}
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
	release, aerr := s.limiter.AcquireSite(ctx, site.LimitKey())
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
	resp.Source = sourceOf(page)
	if fail != nil {
		s.fail(resp, fail, evidenceFrom(page))
		return resp, nil
	}
	if page.Search != nil {
		return s.finishSearch(resp, site, file, page), nil
	}
	if page.AI != nil {
		return s.finishAI(resp, entry, site, file, page, target), nil
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
	if out.failure != nil && entry.Origin == siteconfig.OriginLearned && selectorFailure(out.failure.Code) {
		// The learned selectors failed: count it, and let the AI crawler
		// answer this request so the client still gets a product.
		s.learner.Result(entry, false)
		return s.learnedFallback(ctx, resp, entry, site, file, target), nil
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
			resp.UnavailableFields = unavailableFields(site, out.product, false)
		}
		return resp, nil
	}
	s.succeed(resp, site, out, false)
	if entry.Origin == siteconfig.OriginLearned {
		resp.Warnings = append(resp.Warnings, Warning{Code: WarnLearnedConfig,
			Message: "Selectors were learned by the AI crawler and not reviewed by a person."})
		s.learner.Result(entry, true)
	}
	return resp, nil
}

// selectorFailure reports whether code means the selectors did not fit
// the page, as opposed to the site or the network failing.
func selectorFailure(code string) bool {
	switch code {
	case diagnose.SelectorNotFound, diagnose.RequiredFieldMissing, diagnose.PriceParseFailed:
		return true
	}
	return false
}

// learnedFallback answers a request whose learned selectors failed with an
// AI engine instead: the default engine (APP_DEFAULT_ENGINE), or the engine
// that learned the selectors when the default is none.
func (s *Service) learnedFallback(ctx context.Context, resp *Response, entry *siteconfig.Entry, site *siteconfig.Site, file string, target *url.URL) *Response {
	engine := entry.LearnedEngine()
	if d := s.sites.Snapshot().Default; d != nil {
		engine = d.Site.Fetch.Engine
	}
	aiSite := site.WithEngine(engine)
	page, fail := s.fetcher.Fetch(ctx, aiSite, target)
	resp.Source = sourceOf(page)
	if fail != nil {
		s.fail(resp, fail, evidenceFrom(page))
		return resp
	}
	if page.AI == nil {
		s.fail(resp, diagnose.New(diagnose.InternalError, diagnose.ConfidenceHigh, "The AI crawler fallback returned no AI result.").At(diagnose.StageExtract), evidenceFrom(page))
		return resp
	}
	s.finishAI(resp, entry, aiSite, file, page, target)
	if resp.Status == StatusSuccess {
		resp.Warnings = append([]Warning{{Code: WarnLearnedConfigFailed,
			Message: "The learned selectors failed on this page; the AI crawler extracted it instead."}}, resp.Warnings...)
	}
	return resp
}

// finishAI builds the result from the AI crawler's answer, after checking
// every value against the page. A success on a host without a site config
// teaches the learner.
func (s *Service) finishAI(resp *Response, entry *siteconfig.Entry, site *siteconfig.Site, file string, page *fetch.Page, target *url.URL) *Response {
	out := fromAI(site, file, page, target)
	if out.failure != nil {
		ev := evidenceFrom(page)
		ev.FieldSamples = out.fields
		s.fail(resp, out.failure, ev)
		if out.product != nil {
			resp.Partial.Product = out.product
			resp.UnavailableFields = aiUnavailable(out.product, out.dropped, page.AI.FetchedWith == siteconfig.AIFetchWebSearch)
		}
		return resp
	}
	resp.Status = StatusSuccess
	resp.Product = out.product
	resp.Warnings = out.warnings
	resp.UnavailableFields = aiUnavailable(out.product, out.dropped, page.AI.FetchedWith == siteconfig.AIFetchWebSearch)
	resp.Completeness = completeness(resp.UnavailableFields)

	// Nothing to learn from without the page (ai_fetch web_search).
	if s.learner != nil && entry.Origin == siteconfig.OriginDefault && page.AI.FetchedWith != siteconfig.AIFetchWebSearch {
		p := page.AI.Product
		final := page.FinalURL
		if final == nil {
			final = target
		}
		unit := ""
		if out.product.Price.Current != nil {
			unit = aiUnit(site.Locale.PriceUnit, aicrawler.Str(p.Price.Currency), aicrawler.Str(p.Price.CurrentText)+" "+aicrawler.Str(p.Price.RegularText))
		}
		s.learner.Observe(observation{host: target.Hostname(), finalURL: final, body: page.Body,
			product: out.product, unit: unit, fetchedWith: page.AI.FetchedWith})
	}
	return resp
}

// succeed fills a success result: whatever was found, and every field that
// was not in unavailable_fields.
func (s *Service) succeed(resp *Response, site *siteconfig.Site, out parsed, fromSearch bool) {
	resp.Status = StatusSuccess
	resp.Product = out.product
	resp.Warnings = out.warnings
	resp.UnavailableFields = unavailableFields(site, out.product, fromSearch)
	resp.Completeness = completeness(resp.UnavailableFields)
}

// finishSearch builds the result from a search engine's match. The site
// was not contacted, so there is no page to check or parse.
func (s *Service) finishSearch(resp *Response, site *siteconfig.Site, file string, page *fetch.Page) *Response {
	out := fromSearch(site, file, page.Search)
	if out.failure != nil {
		ev := evidenceFrom(page)
		ev.FieldSamples = out.fields
		s.fail(resp, out.failure, ev)
		resp.Partial.Product = out.product
		resp.UnavailableFields = unavailableFields(site, out.product, true)
		return resp
	}
	s.succeed(resp, site, out, true)
	return resp
}

// sourceOf says which engine produced the page.
func sourceOf(p *fetch.Page) *Source {
	if p == nil || p.Engine == "" {
		return nil
	}
	src := &Source{Engine: p.Engine, SiteContacted: !siteconfig.IsSearchEngine(p.Engine)}
	if a := p.AI; a != nil {
		src.Model, src.PromptVersion, src.FetchedWith, src.Cached = a.Model, a.PromptVersion, a.FetchedWith, a.Cached
		if a.FetchedWith == siteconfig.AIFetchWebSearch {
			src.SiteContacted, src.Sources = false, a.Sources
		}
		if a.Evidence.InputTokens > 0 || a.Evidence.OutputTokens > 0 {
			src.Tokens = &Tokens{Input: a.Evidence.InputTokens, Output: a.Evidence.OutputTokens}
		}
	}
	if sp := p.Search; sp != nil {
		src.Cached = sp.Cached
		if sp.Result != nil {
			src.Query, src.ResultRank = sp.Query, sp.Result.Rank
		}
	}
	return src
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
	if p.Search != nil {
		// The site was not contacted: only the search is evidence.
		ev.Search = p.Search.Evidence()
		return ev
	}
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
	if p.AI != nil {
		ai := p.AI.Evidence
		ev.AI = &ai
	}
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
