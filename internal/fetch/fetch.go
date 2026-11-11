// Package fetch retrieves one product page with Colly, under the request's
// deadline, with retries, per-site spacing, SSRF controls, and evidence.
package fetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"github.com/gocolly/colly/v2"
	"go.uber.org/zap"
)

// Options tune the fetcher. Production uses DefaultOptions; tests override
// the resolver, the allowed networks, and the timings.
type Options struct {
	MaxBodyBytes int64
	MaxRedirects int
	BackoffBase  time.Duration
	Resolver     Resolver
	AllowedNets  []*net.IPNet
	TLSConfig    *tls.Config
}

// DefaultOptions returns production settings.
func DefaultOptions() Options {
	return Options{
		MaxBodyBytes: 10 * 1024 * 1024,
		MaxRedirects: 10,
		BackoffBase:  time.Second,
		Resolver:     SystemResolver,
	}
}

// Page is what one fetch saw. It is filled as far as the fetch got, so it
// can serve as evidence when the fetch fails.
type Page struct {
	RequestedURL string
	FinalURL     *url.URL
	Redirects    []string
	Status       int
	Attempts     int
	Headers      http.Header
	Body         []byte
	BodyLength   int64
}

// ContentType returns the response Content-Type header.
func (p *Page) ContentType() string {
	if p.Headers == nil {
		return ""
	}
	return p.Headers.Get("Content-Type")
}

// Fetcher fetches pages.
type Fetcher struct {
	opts    Options
	limiter *limiter.Limiter
	dialer  *SafeDialer
	log     *zap.Logger
}

// New builds a fetcher.
func New(opts Options, lim *limiter.Limiter, log *zap.Logger) *Fetcher {
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = DefaultOptions().MaxBodyBytes
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = DefaultOptions().MaxRedirects
	}
	return &Fetcher{opts: opts, limiter: lim, dialer: NewSafeDialer(opts.Resolver, opts.AllowedNets), log: log}
}

type redirectError struct{ target string }

func (e *redirectError) Error() string {
	return "redirect to " + e.target + " leaves the site's domains"
}

var errTooManyRedirects = errors.New("too many redirects")

// attempt holds the state of one try.
type attempt struct {
	status   int
	headers  http.Header
	body     []byte
	final    *url.URL
	tooLarge bool
	declared int64
}

// Fetch retrieves target for site. It retries transient failures with
// backoff, honoring Retry-After, within ctx's deadline.
func (f *Fetcher) Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure) {
	page := &Page{RequestedURL: target.String()}
	if p, _ := strconv.Atoi(target.Port()); target.Port() != "" {
		if err := CheckPort(target.Hostname(), p); err != nil {
			return page, classifyError(ctx, err, page)
		}
	}

	var cur *attempt
	c, transport := f.collector(ctx, site, page, &cur)
	defer transport.CloseIdleConnections()

	maxAttempts := site.Retries() + 1
	for n := 1; ; n++ {
		page.Attempts = n
		page.Redirects = page.Redirects[:0]
		a := &attempt{}
		cur = a
		err := c.Visit(target.String())
		f.copyAttempt(page, a)

		var fail *diagnose.Failure
		switch {
		case a.tooLarge:
			size := "more than the limit"
			if a.declared > 0 {
				size = fmt.Sprintf("%d bytes declared", a.declared)
			}
			fail = diagnose.New(diagnose.ResponseTooLarge, diagnose.ConfidenceHigh,
				"The response body was %s; the limit is %d bytes.", size, f.opts.MaxBodyBytes)
		case err != nil:
			fail = classifyError(ctx, err, page)
		default:
			fail = classifyStatus(page)
		}
		if fail == nil {
			return page, nil
		}
		if !retryable(fail.Code) || n >= maxAttempts {
			return page, fail
		}
		wait := f.opts.BackoffBase << (n - 1)
		if ra := retryAfter(page.Headers); ra > wait {
			wait = ra
		}
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < wait+time.Second {
			fail.RootCause += fmt.Sprintf(" Retrying was skipped because the next attempt (after %s) would pass the request deadline.", wait.Round(time.Millisecond))
			return page, fail
		}
		f.log.Debug("retrying fetch", zap.String("url", page.RequestedURL), zap.Int("attempt", n), zap.String("code", fail.Code), zap.Duration("wait", wait))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return page, classifyError(ctx, ctx.Err(), page)
		case <-t.C:
		}
	}
}

func (f *Fetcher) copyAttempt(page *Page, a *attempt) {
	page.Status = a.status
	page.Headers = a.headers
	page.Body = a.body
	page.BodyLength = int64(len(a.body))
	if a.declared > 0 && a.tooLarge {
		page.BodyLength = a.declared
	}
	if a.final != nil {
		page.FinalURL = a.final
	}
}

// collector builds one Colly collector for this request. A new collector
// per request keeps URL history and robots.txt caches from leaking
// between requests.
func (f *Fetcher) collector(ctx context.Context, site *siteconfig.Site, page *Page, cur **attempt) (*colly.Collector, *http.Transport) {
	c := colly.NewCollector(
		colly.StdlibContext(ctx),
		colly.UserAgent(site.Fetch.UserAgent),
		colly.AllowURLRevisit(),
		colly.ParseHTTPErrorResponse(),
		colly.MaxBodySize(int(f.opts.MaxBodyBytes+1)),
		colly.DetectCharset(),
	)
	c.IgnoreRobotsTxt = !site.RespectRobots()

	transport := &http.Transport{
		Proxy:                 nil, // never use environment proxies: they would bypass the SSRF checks
		DialContext:           f.dialer.DialContext,
		TLSClientConfig:       f.opts.TLSConfig,
		TLSHandshakeTimeout:   15 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
		ExpectContinueTimeout: time.Second,
	}
	jar, _ := cookiejar.New(nil)
	c.SetClient(&http.Client{
		Transport: &throttled{base: transport, ctx: ctx, limiter: f.limiter, siteID: site.SiteID, delay: site.Delay()},
		Jar:       jar,
		Timeout:   site.Timeout(),
	})
	c.SetRedirectHandler(func(req *http.Request, via []*http.Request) error {
		isRobots := len(via) > 0 && via[0].URL.Path == "/robots.txt"
		if !isRobots {
			page.Redirects = append(page.Redirects, req.URL.String())
		}
		if len(via) >= f.opts.MaxRedirects {
			return errTooManyRedirects
		}
		if (req.URL.Scheme != "http" && req.URL.Scheme != "https") || !site.HasDomain(req.URL.Hostname()) {
			if isRobots {
				return http.ErrUseLastResponse
			}
			return &redirectError{target: req.URL.String()}
		}
		return nil
	})

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
		r.Headers.Set("Accept-Language", "fa-IR,fa;q=0.9,en;q=0.7")
	})
	c.OnResponseHeaders(func(r *colly.Response) {
		a := *cur
		if cl := r.Headers.Get("Content-Length"); cl != "" {
			if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > f.opts.MaxBodyBytes {
				a.tooLarge, a.declared, a.status = true, n, r.StatusCode
				h := r.Headers.Clone()
				a.headers = h
				a.final = r.Request.URL
				r.Request.Abort()
			}
		}
	})
	c.OnResponse(func(r *colly.Response) {
		a := *cur
		a.status = r.StatusCode
		if r.Headers != nil {
			a.headers = r.Headers.Clone()
		}
		a.final = r.Request.URL
		if int64(len(r.Body)) > f.opts.MaxBodyBytes {
			a.tooLarge = true
			a.body = r.Body[:f.opts.MaxBodyBytes]
			return
		}
		a.body = r.Body
	})
	return c, transport
}

// throttled spaces requests to one site and ties every request, including
// the robots.txt fetch, to the request context.
type throttled struct {
	base    http.RoundTripper
	ctx     context.Context
	limiter *limiter.Limiter
	siteID  string
	delay   time.Duration
}

func (t *throttled) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Context().Done() == nil {
		req = req.WithContext(t.ctx)
	}
	if err := t.limiter.Throttle(req.Context(), t.siteID, t.delay); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func retryable(code string) bool {
	switch code {
	case diagnose.ConnectionFailed, diagnose.FetchTimeout, diagnose.RateLimited, diagnose.SourceServerError, diagnose.DNSFailed:
		return true
	}
	return false
}

func retryAfter(h http.Header) time.Duration {
	if h == nil {
		return 0
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

// classifyError maps a transport error to a failure code.
func classifyError(ctx context.Context, err error, page *Page) *diagnose.Failure {
	host := ""
	if u, perr := url.Parse(page.RequestedURL); perr == nil {
		host = u.Hostname()
	}
	if ctx.Err() != nil {
		return diagnose.New(diagnose.DeadlineExceeded, diagnose.ConfidenceHigh,
			"The request's time limit was reached while fetching the page (attempt %d).", max(page.Attempts, 1)).At(diagnose.StageFetch)
	}
	var notAllowed *NotAllowedError
	var redir *redirectError
	var dnsErr *net.DNSError
	var certInvalid x509.CertificateInvalidError
	var hostErr x509.HostnameError
	var unknownAuth x509.UnknownAuthorityError
	var verifyErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	var alertErr tls.AlertError
	var netErr net.Error

	switch {
	case errors.Is(err, colly.ErrRobotsTxtBlocked):
		return diagnose.New(diagnose.RobotsDisallowed, diagnose.ConfidenceHigh,
			"The site's robots.txt disallows %s for this user agent.", page.RequestedURL)
	case errors.As(err, &notAllowed):
		return diagnose.New(diagnose.URLNotAllowed, diagnose.ConfidenceHigh, "%s.", capitalize(notAllowed.Error()))
	case errors.As(err, &redir):
		return diagnose.New(diagnose.RedirectNotAllowed, diagnose.ConfidenceHigh,
			"The site redirected to %s, which is not one of the site's configured domains.", redir.target)
	case errors.Is(err, errTooManyRedirects):
		return diagnose.New(diagnose.TooManyRedirects, diagnose.ConfidenceHigh,
			"The site redirected %d times without serving a page; this is probably a redirect loop.", len(page.Redirects))
	case errors.As(err, &dnsErr):
		return diagnose.New(diagnose.DNSFailed, diagnose.ConfidenceHigh, "The host name %s did not resolve: %s.", host, dnsErr.Err)
	case errors.As(err, &certInvalid), errors.As(err, &hostErr), errors.As(err, &unknownAuth),
		errors.As(err, &verifyErr), errors.As(err, &recordErr), errors.As(err, &alertErr),
		strings.Contains(err.Error(), "tls: "), strings.Contains(err.Error(), "x509: "):
		return diagnose.New(diagnose.TLSFailed, diagnose.ConfidenceHigh, "The TLS handshake with %s failed: %s.", host, rootMessage(err))
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return diagnose.New(diagnose.FetchTimeout, diagnose.ConfidenceHigh,
			"%s did not respond within the site's timeout (attempt %d).", host, max(page.Attempts, 1))
	case errors.Is(err, syscall.ECONNREFUSED):
		return diagnose.New(diagnose.ConnectionFailed, diagnose.ConfidenceHigh, "%s refused the connection.", host)
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return diagnose.New(diagnose.ConnectionFailed, diagnose.ConfidenceHigh, "%s closed or reset the connection.", host)
	default:
		return diagnose.New(diagnose.ConnectionFailed, diagnose.ConfidenceMedium, "Fetching the page failed: %s.", rootMessage(err))
	}
}

// classifyStatus maps a completed response to a failure, or nil when the
// response is an HTML page to parse.
func classifyStatus(page *Page) *diagnose.Failure {
	st := page.Status
	switch {
	case st >= 200 && st < 300:
		if !isHTML(page) {
			return diagnose.New(diagnose.UnsupportedContent, diagnose.ConfidenceHigh,
				"The response has Content-Type %q, not HTML.", page.ContentType())
		}
		return nil
	case st == http.StatusNotFound || st == http.StatusGone:
		return diagnose.New(diagnose.HTTPNotFound, diagnose.ConfidenceHigh, "The site returned HTTP %d for this URL.", st)
	}
	signals := diagnose.Signals(nil, page.Body, page.Headers)
	switch {
	case st == http.StatusUnauthorized || st == http.StatusForbidden:
		if f := diagnose.CheckBlocked(signals, st); f != nil {
			return f
		}
		return diagnose.New(diagnose.AccessDenied, diagnose.ConfidenceHigh,
			"The site returned HTTP %d without signs of an anti-bot challenge.", st)
	case st == http.StatusTooManyRequests:
		return diagnose.New(diagnose.RateLimited, diagnose.ConfidenceHigh,
			"The site returned HTTP 429 on all %d attempts.", page.Attempts)
	case st >= 500:
		if f := diagnose.CheckBlocked(signals, st); f != nil {
			return f
		}
		return diagnose.New(diagnose.SourceServerError, diagnose.ConfidenceHigh,
			"The site returned HTTP %d on all %d attempts.", st, page.Attempts)
	default:
		return diagnose.New(diagnose.HTTPError, diagnose.ConfidenceHigh, "The site returned HTTP %d, which the service does not handle.", st)
	}
}

func isHTML(page *Page) bool {
	ct := page.ContentType()
	if ct == "" {
		if len(page.Body) == 0 {
			return true // let the page check report EMPTY_PAGE
		}
		ct = http.DetectContentType(page.Body)
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == "text/html" || mt == "application/xhtml+xml"
}

// rootMessage removes Go's URL-error wrapping for a readable cause.
func rootMessage(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return strings.TrimSuffix(err.Error(), ".")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
