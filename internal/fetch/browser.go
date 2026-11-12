package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/siteconfig"

	"github.com/gocolly/colly/v2"
	"github.com/playwright-community/playwright-go"
	"github.com/temoto/robotstxt"
	"go.uber.org/zap"
)

// BrowserOptions tune the browser engine.
type BrowserOptions struct {
	// Channel picks the browser build: "" or "chromium" for Playwright's
	// bundled Chromium, "chrome" for the installed Google Chrome.
	Channel string
	// Headless runs the browser without a window. Servers need true;
	// a visible window needs a display (or Xvfb).
	Headless bool
	// MaxPages caps the pages open at once on this instance. Each costs
	// roughly 100–300 MB.
	MaxPages int
	// DriverDirectory is where the Playwright driver is installed. Empty
	// uses PLAYWRIGHT_DRIVER_PATH or the user cache directory.
	DriverDirectory string
}

// DefaultBrowserOptions returns production settings.
func DefaultBrowserOptions() BrowserOptions {
	return BrowserOptions{Headless: true, MaxPages: 3}
}

// robotsTTL is how long a site's robots.txt is cached.
const robotsTTL = time.Hour

// BrowserFetcher is the "browser" strategy: it loads the page in a real
// Chrome driven by Playwright, so JavaScript runs and anti-bot challenges
// that only need a real browser are passed. One browser runs per instance;
// every fetch gets a fresh, isolated browser context. All of Chrome's
// traffic goes through the egress gate, which applies the same SSRF rules
// and outbound proxy as the HTTP engine.
type BrowserFetcher struct {
	opts    Options
	bopts   BrowserOptions
	limiter *limiter.Limiter
	dialer  *SafeDialer
	log     *zap.Logger
	slots   chan struct{}

	mu      sync.Mutex
	pw      *playwright.Playwright
	browser playwright.Browser
	gate    *gate
	closed  bool

	// Cookies and local storage per site (for example a cf_clearance
	// cookie), reused by the next fetch of the same site, never another's.
	statesMu sync.Mutex
	states   map[string]*playwright.OptionalStorageState

	robotsMu sync.Mutex
	robots   map[string]robotsEntry
}

type robotsEntry struct {
	data *robotstxt.RobotsData
	at   time.Time
}

// NewBrowserFetcher builds the browser engine. The browser starts on Start
// or on the first fetch.
func NewBrowserFetcher(opts Options, bopts BrowserOptions, lim *limiter.Limiter, log *zap.Logger) *BrowserFetcher {
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = DefaultOptions().MaxBodyBytes
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = DefaultOptions().MaxRedirects
	}
	if bopts.MaxPages <= 0 {
		bopts.MaxPages = DefaultBrowserOptions().MaxPages
	}
	if bopts.Channel == "chromium" {
		bopts.Channel = ""
	}
	return &BrowserFetcher{
		opts:    opts,
		bopts:   bopts,
		limiter: lim,
		dialer:  NewSafeDialer(opts.Resolver, opts.AllowedNets),
		log:     log,
		slots:   make(chan struct{}, bopts.MaxPages),
		states:  make(map[string]*playwright.OptionalStorageState),
		robots:  make(map[string]robotsEntry),
	}
}

// Name makes this the "browser" strategy.
func (b *BrowserFetcher) Name() string { return EngineBrowser }

// Start launches the browser now, so a missing driver or browser fails at
// startup instead of on the first request.
func (b *BrowserFetcher) Start(context.Context) error {
	_, err := b.ensure()
	return err
}

// Stop closes the browser, the Playwright driver and the egress gate.
func (b *BrowserFetcher) Stop(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return b.shutdownLocked()
}

func (b *BrowserFetcher) shutdownLocked() error {
	var errs []error
	if b.browser != nil {
		errs = append(errs, b.browser.Close())
		b.browser = nil
	}
	if b.pw != nil {
		errs = append(errs, b.pw.Stop())
		b.pw = nil
	}
	if b.gate != nil {
		errs = append(errs, b.gate.Close())
		b.gate = nil
	}
	return errors.Join(errs...)
}

// ensure returns a connected browser, launching it (again) when needed.
func (b *BrowserFetcher) ensure() (playwright.Browser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("the browser engine is stopped")
	}
	if b.browser != nil && b.browser.IsConnected() {
		return b.browser, nil
	}
	if b.browser != nil {
		b.log.Warn("browser disconnected; relaunching")
	}
	_ = b.shutdownLocked()

	g, err := startGate(b.dialer, b.opts.Proxy, b.log)
	if err != nil {
		return nil, err
	}
	pw, err := playwright.Run(&playwright.RunOptions{
		DriverDirectory:     b.bopts.DriverDirectory,
		SkipInstallBrowsers: true,
		Verbose:             false,
	})
	if err != nil {
		_ = g.Close()
		return nil, fmt.Errorf("start the Playwright driver: %w", err)
	}
	launch := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(b.bopts.Headless),
		// Playwright also proxies loopback (it adds <-loopback> to the
		// bypass list), so nothing reaches the network around the gate.
		Proxy: &playwright.Proxy{Server: g.URL()},
		Args: []string{
			// WebRTC would otherwise send UDP around the proxy.
			"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		},
	}
	if b.bopts.Channel != "" {
		launch.Channel = playwright.String(b.bopts.Channel)
	}
	browser, err := pw.Chromium.Launch(launch)
	if err != nil {
		_ = pw.Stop()
		_ = g.Close()
		return nil, fmt.Errorf("launch the browser (channel %q): %w", b.channelName(), err)
	}
	b.pw, b.browser, b.gate = pw, browser, g
	b.log.Info("browser engine started", zap.String("channel", b.channelName()), zap.String("version", browser.Version()),
		zap.Bool("headless", b.bopts.Headless), zap.Int("max_pages", b.bopts.MaxPages))
	return browser, nil
}

func (b *BrowserFetcher) channelName() string {
	if b.bopts.Channel == "" {
		return "chromium"
	}
	return b.bopts.Channel
}

func (b *BrowserFetcher) currentGate() *gate {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gate
}

// Fetch loads target in a fresh browser context, retrying transient
// failures like the HTTP engine does.
func (b *BrowserFetcher) Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure) {
	page := &Page{RequestedURL: target.String()}
	if p, _ := strconv.Atoi(target.Port()); target.Port() != "" {
		if err := CheckPort(target.Hostname(), p); err != nil {
			return page, classifyError(ctx, err, page)
		}
	}

	// One page slot per fetch, across retries.
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	case <-ctx.Done():
		return page, classifyError(ctx, ctx.Err(), page)
	}

	browser, err := b.ensure()
	if err != nil {
		return page, diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceHigh,
			"The browser engine could not start: %s.", strings.TrimSuffix(err.Error(), "."))
	}

	if site.RespectRobots() {
		if f := b.checkRobots(ctx, site, target, page); f != nil {
			return page, f
		}
	}

	maxAttempts := site.Retries() + 1
	for n := 1; ; n++ {
		page.Attempts = n
		fail := b.attempt(ctx, browser, site, target, page)
		if fail == nil {
			return page, nil
		}
		if !retryable(fail.Code) || n >= maxAttempts {
			return page, fail
		}
		wait := b.opts.BackoffBase << (n - 1)
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < wait+site.Timeout() {
			fail.RootCause += fmt.Sprintf(" Retrying was skipped because the next attempt (after %s) would pass the request deadline.", wait.Round(time.Millisecond))
			return page, fail
		}
		b.log.Debug("retrying browser fetch", zap.String("url", page.RequestedURL), zap.Int("attempt", n), zap.String("code", fail.Code))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return page, classifyError(ctx, ctx.Err(), page)
		case <-t.C:
		}
	}
}

// attempt runs one navigation and fills page with what it saw.
func (b *BrowserFetcher) attempt(ctx context.Context, browser playwright.Browser, site *siteconfig.Site, target *url.URL, page *Page) *diagnose.Failure {
	timeout := site.Timeout()
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl); left < timeout {
			timeout = left
		}
	}
	if timeout <= 0 {
		return classifyError(ctx, context.DeadlineExceeded, page)
	}
	if err := b.limiter.Throttle(ctx, site.SiteID, site.Delay()); err != nil {
		return classifyError(ctx, err, page)
	}
	start := time.Now()

	opts := playwright.BrowserNewContextOptions{
		AcceptDownloads: playwright.Bool(false),
		ServiceWorkers:  playwright.ServiceWorkerPolicyBlock, // their requests would skip the route checks below
		Locale:          playwright.String(browserLocale(site)),
		StorageState:    b.storageState(site.SiteID),
	}
	// Chrome's own user agent matches its TLS fingerprint and client hints.
	// Only an explicit user_agents list in the site config overrides it.
	if len(site.Fetch.UserAgents) > 0 {
		opts.UserAgent = playwright.String(site.PickUserAgent())
	}
	bctx, err := browser.NewContext(opts)
	if err != nil {
		return diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceHigh,
			"The browser could not open a new context: %s.", rootMessage(err))
	}
	defer bctx.Close()
	// Closing the context aborts the navigation when the request is cancelled.
	stop := context.AfterFunc(ctx, func() { _ = bctx.Close() })
	defer stop()

	// Main-frame navigations stay on the site's domains; only http(s) leaves
	// the browser. The gate checks every connection's destination.
	var navMu sync.Mutex
	var navErr error
	_ = bctx.Route("**/*", func(r playwright.Route) {
		req := r.Request()
		u, err := url.Parse(req.URL())
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			_ = r.Abort("blockedbyclient")
			return
		}
		if req.IsNavigationRequest() && isMainFrame(req) && !site.HasDomain(u.Hostname()) {
			navMu.Lock()
			navErr = &redirectError{target: u.String()}
			navMu.Unlock()
			_ = r.Abort("blockedbyclient")
			return
		}
		_ = r.Continue()
	})

	p, err := bctx.NewPage()
	if err != nil {
		return diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceHigh,
			"The browser could not open a page: %s.", rootMessage(err))
	}
	ms := float64(timeout.Milliseconds())
	p.SetDefaultTimeout(ms)

	// The latest main-frame document response wins: after an anti-bot
	// challenge reloads the page, that is the real page, not the challenge.
	var docMu sync.Mutex
	var doc playwright.Response
	p.OnResponse(func(r playwright.Response) {
		req := r.Request()
		if req.IsNavigationRequest() && isMainFrame(req) {
			docMu.Lock()
			doc = r
			docMu.Unlock()
		}
	})

	resp, gerr := p.Goto(target.String(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(ms),
	})
	if resp != nil {
		page.Redirects = redirectChain(resp.Request())
	}
	if gerr != nil {
		navMu.Lock()
		blocked := navErr
		navMu.Unlock()
		return b.navigationFailure(ctx, gerr, blocked, target, page)
	}

	// For plain http:// a refusal by the egress gate arrives as a page.
	if resp != nil {
		if v, _ := resp.HeaderValue(gateRefusedHeader); v != "" {
			var refused error = &NotAllowedError{Host: target.Hostname(), Reason: "refused by the egress rules"}
			if g := b.currentGate(); g != nil {
				if gerr := g.lastFailure(target.Hostname()); gerr != nil {
					refused = gerr
				}
			}
			page.Status = resp.Status()
			return classifyError(ctx, refused, page)
		}
	}

	// Wait for the product root when the response may still turn into the
	// product page (a challenge reloads itself; a shell renders with JS).
	// A 404 or similar is final, so it is not waited on.
	if resp == nil || waitable(resp.Status()) {
		if root := site.Pages.Product.Root; root != "" {
			left := timeout - time.Since(start)
			if left > 0 {
				_ = p.Locator(root).First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateAttached,
					Timeout: playwright.Float(float64(left.Milliseconds())),
				})
			}
		}
	}

	html, err := p.Content()
	if err != nil {
		if ctx.Err() != nil {
			return classifyError(ctx, ctx.Err(), page)
		}
		return diagnose.New(diagnose.ConnectionFailed, diagnose.ConfidenceMedium,
			"The browser could not read the page: %s.", rootMessage(err))
	}
	final, _ := url.Parse(p.URL())
	page.FinalURL = final
	docMu.Lock()
	last := doc
	docMu.Unlock()
	if last == nil {
		last = resp
	}
	if last != nil {
		page.Status = last.Status()
		page.Headers = responseHeaders(last)
	} else {
		page.Status = http.StatusOK
		page.Headers = http.Header{}
	}
	if page.Headers.Get("Content-Type") == "" {
		page.Headers.Set("Content-Type", "text/html; charset=utf-8")
	}
	page.Body = []byte(html)
	page.BodyLength = int64(len(page.Body))

	if final != nil && !site.HasDomain(final.Hostname()) {
		return classifyError(ctx, &redirectError{target: final.String()}, page)
	}
	if page.BodyLength > b.opts.MaxBodyBytes {
		page.Body = page.Body[:b.opts.MaxBodyBytes]
		return diagnose.New(diagnose.ResponseTooLarge, diagnose.ConfidenceHigh,
			"The rendered page was %d bytes; the limit is %d bytes.", page.BodyLength, b.opts.MaxBodyBytes)
	}
	if fail := classifyStatus(page); fail != nil {
		return fail
	}
	if st, err := bctx.StorageState(); err == nil && st != nil {
		b.saveStorageState(site.SiteID, st.ToOptionalStorageState())
	}
	return nil
}

// navigationFailure maps a failed Goto to a failure code, preferring the
// precise reason the route handler or the egress gate recorded.
func (b *BrowserFetcher) navigationFailure(ctx context.Context, err, blocked error, target *url.URL, page *Page) *diagnose.Failure {
	if ctx.Err() != nil {
		return classifyError(ctx, ctx.Err(), page)
	}
	if blocked != nil {
		return classifyError(ctx, blocked, page)
	}
	if g := b.currentGate(); g != nil {
		if gerr := g.lastFailure(target.Hostname()); gerr != nil {
			return classifyError(ctx, gerr, page)
		}
	}
	host := target.Hostname()
	msg := err.Error()
	switch {
	case errors.Is(err, playwright.ErrTimeout):
		return diagnose.New(diagnose.FetchTimeout, diagnose.ConfidenceHigh,
			"%s did not finish loading in the browser within the site's timeout (attempt %d).", host, max(page.Attempts, 1))
	case strings.Contains(msg, "net::ERR_NAME_NOT_RESOLVED"):
		return diagnose.New(diagnose.DNSFailed, diagnose.ConfidenceHigh, "The host name %s did not resolve.", host)
	case strings.Contains(msg, "net::ERR_CERT_"), strings.Contains(msg, "net::ERR_SSL_"):
		return diagnose.New(diagnose.TLSFailed, diagnose.ConfidenceHigh, "The TLS handshake with %s failed: %s.", host, chromeError(msg))
	case strings.Contains(msg, "net::ERR_TOO_MANY_REDIRECTS"):
		return diagnose.New(diagnose.TooManyRedirects, diagnose.ConfidenceHigh,
			"The site redirected too many times without serving a page; this is probably a redirect loop.")
	case strings.Contains(msg, "Download is starting"), strings.Contains(msg, "net::ERR_ABORTED"):
		return diagnose.New(diagnose.UnsupportedContent, diagnose.ConfidenceMedium,
			"The browser did not render the URL as a page (%s); it is probably a file download.", chromeError(msg))
	case strings.Contains(msg, "Target page, context or browser has been closed"):
		return diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceMedium, "The browser closed during the fetch.")
	}
	return diagnose.New(diagnose.ConnectionFailed, diagnose.ConfidenceMedium, "The browser could not load the page: %s.", chromeError(msg))
}

// checkRobots applies robots.txt as the HTTP engine does. robots.txt is
// fetched through the same SSRF-safe transport and cached for an hour.
func (b *BrowserFetcher) checkRobots(ctx context.Context, site *siteconfig.Site, target *url.URL, page *Page) *diagnose.Failure {
	key := target.Scheme + "://" + target.Host
	b.robotsMu.Lock()
	e, ok := b.robots[key]
	b.robotsMu.Unlock()
	if !ok || time.Since(e.at) > robotsTTL {
		data, err := b.fetchRobots(ctx, site, key)
		if err != nil {
			// Same as Colly: a robots.txt that cannot be read does not block.
			b.log.Debug("robots.txt unavailable", zap.String("site", key), zap.Error(err))
			return nil
		}
		e = robotsEntry{data: data, at: time.Now()}
		b.robotsMu.Lock()
		b.robots[key] = e
		b.robotsMu.Unlock()
	}
	path := target.EscapedPath()
	if target.RawQuery != "" {
		path += "?" + target.RawQuery
	}
	if !e.data.TestAgent(path, site.PickUserAgent()) {
		return classifyError(ctx, colly.ErrRobotsTxtBlocked, page)
	}
	return nil
}

func (b *BrowserFetcher) fetchRobots(ctx context.Context, site *siteconfig.Site, origin string) (*robotstxt.RobotsData, error) {
	transport := &http.Transport{Proxy: nil, DialContext: b.dialer.DialContext, TLSClientConfig: b.opts.TLSConfig}
	if b.opts.Proxy != nil {
		g := &gate{dialer: b.dialer, upstream: b.opts.Proxy}
		transport.Proxy = g.proxyFor
		transport.DialContext = g.dialUpstream
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", site.PickUserAgent())
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}
	return robotstxt.FromStatusAndBytes(resp.StatusCode, body)
}

func (b *BrowserFetcher) storageState(siteID string) *playwright.OptionalStorageState {
	b.statesMu.Lock()
	defer b.statesMu.Unlock()
	return b.states[siteID]
}

func (b *BrowserFetcher) saveStorageState(siteID string, st *playwright.OptionalStorageState) {
	b.statesMu.Lock()
	defer b.statesMu.Unlock()
	b.states[siteID] = st
}

// waitable reports whether a first response may still become the product
// page: success, or a status anti-bot challenges answer with.
func waitable(status int) bool {
	return (status >= 200 && status < 300) || status == http.StatusForbidden ||
		status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

func isMainFrame(req playwright.Request) (main bool) {
	defer func() {
		if recover() != nil { // requests without a frame (workers)
			main = false
		}
	}()
	f := req.Frame()
	return f != nil && f.ParentFrame() == nil
}

func redirectChain(req playwright.Request) []string {
	var chain []string
	for r := req.RedirectedFrom(); r != nil; r = r.RedirectedFrom() {
		chain = append(chain, r.URL())
	}
	// chain runs newest first; Page.Redirects lists the targets in order.
	out := make([]string, 0, len(chain))
	for i := len(chain) - 2; i >= 0; i-- {
		out = append(out, chain[i])
	}
	if len(chain) > 0 {
		out = append(out, req.URL())
	}
	return out
}

func responseHeaders(r playwright.Response) http.Header {
	h := http.Header{}
	all, err := r.AllHeaders()
	if err != nil {
		all = r.Headers()
	}
	for k, v := range all {
		for _, line := range strings.Split(v, "\n") {
			h.Add(k, line)
		}
	}
	return h
}

// browserLocale is the first language the HTTP engine would ask for.
func browserLocale(site *siteconfig.Site) string {
	first, _, _ := strings.Cut(acceptLanguage(site), ",")
	first, _, _ = strings.Cut(first, ";")
	return strings.TrimSpace(first)
}

// chromeError keeps the net::ERR_* code of a Playwright navigation error.
func chromeError(msg string) string {
	if i := strings.Index(msg, "net::ERR_"); i >= 0 {
		code := msg[i:]
		if j := strings.IndexAny(code, " \n"); j > 0 {
			code = code[:j]
		}
		return code
	}
	first, _, _ := strings.Cut(msg, "\n")
	return strings.TrimSuffix(first, ".")
}
