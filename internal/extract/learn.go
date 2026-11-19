package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go-wordpress/internal/aicrawler"
	"go-wordpress/internal/normalize"
	"go-wordpress/internal/siteconfig"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// LearnSettings configure selector learning.
type LearnSettings struct {
	// Dir holds the learned configs (APP_AI_CRAWLER_LEARN_DIR).
	Dir string
	// PromoteAfter different product URLs must match in shadow before a
	// candidate becomes active.
	PromoteAfter int
	// DemoteAfter failures in a row demote an active learned config.
	DemoteAfter int
	// MaxAttemptsPerDay caps learning calls per host per day.
	MaxAttemptsPerDay int
	// DelayMS is fetch.delay_ms in learned configs.
	DelayMS int
}

// Learner turns successful AI extractions into CSS selector configs
// (proposal section 6.2): learn, check on the same page, run in shadow next
// to the AI crawler, and activate after enough matches. Learning and shadow
// checks run in the background, after the response is sent.
type Learner struct {
	client *aicrawler.Client
	s      LearnSettings
	log    *zap.Logger
	now    func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	locks    map[string]*sync.Mutex
	attempts map[string][]time.Time
}

// NewLearner builds a learner. It returns nil when client is nil.
func NewLearner(client *aicrawler.Client, s LearnSettings, log *zap.Logger) *Learner {
	if client == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Learner{client: client, s: s, log: log, now: time.Now, ctx: ctx, cancel: cancel,
		locks: map[string]*sync.Mutex{}, attempts: map[string][]time.Time{}}
}

// Stop cancels running learning calls and waits for them.
func (l *Learner) Stop(context.Context) error {
	if l == nil {
		return nil
	}
	l.cancel()
	l.wg.Wait()
	return nil
}

// Wait blocks until every background job started so far is done. For tests.
func (l *Learner) Wait() {
	if l != nil {
		l.wg.Wait()
	}
}

func (l *Learner) lock(host string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.locks[host]
	if !ok {
		m = &sync.Mutex{}
		l.locks[host] = m
	}
	return m
}

// allow reports whether host may be learned once more today, and counts it.
func (l *Learner) allow(host string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := l.now().Add(-24 * time.Hour)
	kept := l.attempts[host][:0]
	for _, t := range l.attempts[host] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.s.MaxAttemptsPerDay {
		l.attempts[host] = kept
		return false
	}
	l.attempts[host] = append(kept, l.now())
	return true
}

// observation is what one successful AI extraction teaches.
type observation struct {
	host        string
	finalURL    *url.URL
	body        []byte
	product     *Product
	unit        string
	fetchedWith string
}

// Observe is called after a successful AI extraction for a host without a
// site config. A candidate config for the host is checked in shadow; with
// no config (or a demoted one) the host is learned.
func (l *Learner) Observe(o observation) {
	if l == nil || o.unit == "" || len(o.body) == 0 {
		return
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		ctx, cancel := context.WithTimeout(l.ctx, 3*time.Minute)
		defer cancel()
		mu := l.lock(o.host)
		mu.Lock()
		defer mu.Unlock()
		l.observe(ctx, o)
	}()
}

func (l *Learner) observe(ctx context.Context, o observation) {
	path := siteconfig.LearnedFile(l.s.Dir, o.host)
	log := l.log.With(zap.String("host", o.host), zap.String("file", filepath.Base(path)))
	raw, err := siteconfig.ReadLearnedRaw(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		log.Error("learned config unreadable; not learning", zap.Error(err))
		return
	default:
		block, _ := raw["learned"].(map[string]any)
		switch status, _ := block["status"].(string); status {
		case siteconfig.LearnedRejected, siteconfig.LearnedActive:
			return
		case siteconfig.LearnedCandidate:
			site, err := siteconfig.ParseSite(raw)
			if err == nil {
				ok, why := shadowMatch(site, o)
				if ok {
					l.recordMatch(path, raw, block, o.finalURL.String(), log)
					return
				}
				log.Info("learned selectors disagree with the AI crawler; relearning", zap.String("url", o.finalURL.String()), zap.String("why", why))
			} else {
				log.Warn("candidate config invalid; relearning", zap.Error(err))
			}
		}
	}
	l.learn(ctx, path, o, log)
}

// recordMatch adds url to shadow_matches and activates the config once
// PromoteAfter different URLs matched.
func (l *Learner) recordMatch(path string, raw, block map[string]any, u string, log *zap.Logger) {
	matches := stringList(block["shadow_matches"])
	if slices.Contains(matches, u) {
		return
	}
	matches = append(matches, u)
	block["shadow_matches"] = matches
	if len(matches) >= l.s.PromoteAfter {
		block["status"] = siteconfig.LearnedActive
	}
	raw["learned"] = block
	if err := siteconfig.WriteLearned(path, raw); err != nil {
		log.Error("could not write learned config", zap.Error(err))
		return
	}
	log.Info("learned selectors matched in shadow", zap.Int("matches", len(matches)), zap.Any("status", block["status"]))
}

// learn asks the model for selectors, checks them on the same page and
// writes a candidate config.
func (l *Learner) learn(ctx context.Context, path string, o observation, log *zap.Logger) {
	if !l.allow(o.host) {
		log.Info("selector learning skipped: daily attempts used up", zap.Int("max", l.s.MaxAttemptsPerDay))
		return
	}
	outline, err := aicrawler.Outline(o.body, 40000)
	if err != nil {
		log.Warn("selector learning: outline failed", zap.Error(err))
		return
	}
	pj, _ := json.MarshalIndent(learnView(o.product), "", "  ")
	msgs := []aicrawler.Message{
		{Role: "system", Content: aicrawler.LearnSystemPrompt},
		{Role: "user", Content: aicrawler.LearnUserPrompt(o.finalURL.String(), outline, string(pj))},
	}
	comp, cerr := l.client.Complete(aicrawler.WithCall(ctx, aicrawler.PurposeLearn, o.host), msgs, "selectors", nil)
	if cerr != nil {
		log.Warn("selector learning: model call failed", zap.String("code", cerr.Code), zap.String("error", cerr.Message))
		return
	}
	sel, err := aicrawler.ParseSelectors(comp.Content)
	if err != nil {
		log.Warn("selector learning: unusable answer", zap.Error(err))
		return
	}
	raw, err := l.buildConfig(o, sel)
	if err != nil {
		log.Info("selector learning: selectors rejected", zap.Error(err))
		return
	}
	site, err := siteconfig.ParseSite(raw)
	if err != nil {
		log.Info("selector learning: config invalid", zap.Error(err))
		return
	}
	if ok, why := shadowMatch(site, o); !ok {
		log.Info("selector learning: selectors do not reproduce the AI result", zap.String("why", why))
		return
	}
	block := raw["learned"].(map[string]any)
	if l.s.PromoteAfter <= 1 {
		block["status"] = siteconfig.LearnedActive
	}
	if err := siteconfig.WriteLearned(path, raw); err != nil {
		log.Error("could not write learned config", zap.Error(err))
		return
	}
	log.Info("selectors learned", zap.Any("status", block["status"]), zap.String("from", o.finalURL.String()))
}

// maxSelectorLen bounds every learned selector.
const maxSelectorLen = 500

// buildConfig turns the model's selectors into a learned site config.
func (l *Learner) buildConfig(o observation, sel *aicrawler.Selectors) (map[string]any, error) {
	check := func(name, s string) (string, error) {
		s = strings.TrimSpace(s)
		if len(s) > maxSelectorLen {
			return "", fmt.Errorf("%s is longer than %d characters", name, maxSelectorLen)
		}
		return s, nil
	}
	root, err := check("root", sel.Root)
	if err != nil {
		return nil, err
	}
	product := map[string]any{"match": []string{".*"}, "root": root}
	if s, err := check("price_area", aicrawler.Str(sel.PriceArea)); err != nil {
		return nil, err
	} else if s != "" {
		product["price_area"] = s
	}
	fields := map[string]any{}
	for name, f := range sel.Fields {
		if f == nil || !slices.Contains(siteconfig.KnownFields, name) {
			continue
		}
		s, err := check("fields."+name, aicrawler.Str(f.Selector))
		if err != nil {
			return nil, err
		}
		if s == "" {
			continue
		}
		fm := map[string]any{"selector": s}
		attr := []string(f.Attr)
		if name == siteconfig.FieldImageURL && len(attr) == 0 {
			attr = []string{"src", "data-src", "data-original"}
		}
		if len(attr) > 0 {
			fm["attr"] = attr
		}
		switch pk := aicrawler.Str(f.Pick); pk {
		case siteconfig.PickFirst, siteconfig.PickLast, siteconfig.PickJoin:
			fm["pick"] = pk
		}
		if f.Separator != nil {
			fm["separator"] = *f.Separator
		}
		fields[name] = fm
	}
	if _, ok := fields[siteconfig.FieldTitle]; !ok {
		return nil, errors.New("no title selector")
	}
	product["fields"] = fields
	if sp := sel.Specs; sp != nil && aicrawler.Str(sp.Name) != "" {
		specs := map[string]any{}
		for k, v := range map[string]*string{"layout": sp.Layout, "group": sp.Group, "title": sp.Title, "row": sp.Row, "name": sp.Name, "value": sp.Value} {
			s, err := check("specs."+k, aicrawler.Str(v))
			if err != nil {
				return nil, err
			}
			if s != "" {
				specs[k] = s
			}
		}
		product["specs"] = specs
	}
	if f := sel.Features; f != nil && aicrawler.Str(f.Selector) != "" {
		s, err := check("features.selector", aicrawler.Str(f.Selector))
		if err != nil {
			return nil, err
		}
		product["features"] = map[string]any{"selector": s}
	}
	if a := sel.Availability; a != nil {
		av := map[string]any{}
		for k, v := range map[string]*string{"out_of_stock": a.OutOfStock, "order_button": a.OrderButton} {
			s, err := check("availability."+k, aicrawler.Str(v))
			if err != nil {
				return nil, err
			}
			if s != "" {
				av[k] = s
			}
		}
		if len(av) > 0 {
			product["availability"] = av
		}
	}

	now := l.now().UTC()
	engine := o.fetchedWith
	if engine != siteconfig.EngineBrowser {
		engine = siteconfig.EngineHTTP
	}
	return map[string]any{
		"$comment": fmt.Sprintf("Learned by %s (%s, learn prompt %s) from %s on %s. Not reviewed by a person: check it with sitectl learned show, then adopt or reject it.",
			l.client.Engine(), l.client.Model(), aicrawler.LearnPromptVersion, o.finalURL.String(), now.Format("2006-01-02")),
		"schema_version": 1,
		"site_id":        siteconfig.LearnedSiteID(o.host),
		"version":        "learned-" + now.Format("2006-01-02") + ".1",
		"domains":        siteconfig.LearnedDomains(o.host),
		"learned": map[string]any{
			"status":            siteconfig.LearnedCandidate,
			"engine":            l.client.Engine(),
			"model":             l.client.Model(),
			"prompt_version":    aicrawler.LearnPromptVersion,
			"learned_from":      o.finalURL.String(),
			"learned_at":        now.Format(time.RFC3339),
			"shadow_matches":    []string{o.finalURL.String()},
			"failures_in_a_row": 0,
		},
		"fetch": map[string]any{
			"engine":             engine,
			"respect_robots_txt": true,
			"delay_ms":           l.s.DelayMS,
			"timeout_seconds":    20,
			"retries":            1,
		},
		"locale":     map[string]any{"price_unit": o.unit, "currency_words": currencyWords(o.unit)},
		"pages":      map[string]any{"product": product},
		"validation": map[string]any{"required_fields": []string{siteconfig.FieldTitle}},
	}, nil
}

func currencyWords(unit string) []string {
	switch unit {
	case siteconfig.UnitUSD:
		return []string{"$", "USD"}
	case siteconfig.UnitEUR:
		return []string{"€", "EUR"}
	case siteconfig.UnitRial:
		return []string{"ریال"}
	}
	return []string{"تومان"}
}

// learnView is the product as the learning prompt shows it.
func learnView(p *Product) map[string]any {
	m := map[string]any{"title": p.Title, "price_text": p.Price.Raw}
	if p.ImageURL != nil {
		m["image_url"] = *p.ImageURL
	}
	if p.Description != nil {
		m["description"] = *p.Description
	}
	if len(p.Specs) > 0 {
		m["specs"] = p.Specs
	}
	if len(p.Features) > 0 {
		m["features"] = p.Features
	}
	return m
}

// shadowMatch runs the selector pipeline with site on the page the AI
// crawler read and compares the two products.
func shadowMatch(site *siteconfig.Site, o observation) (bool, string) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(o.body))
	if err != nil {
		return false, err.Error()
	}
	out := parsePage(PageInput{Site: site, File: "learned", Doc: doc, Body: o.body, Status: 200, FinalURL: o.finalURL})
	if out.failure != nil {
		return false, out.failure.Code + ": " + out.failure.RootCause
	}
	return sameProduct(out.product, o.product)
}

// sameProduct compares a selector result (got) with the AI result (want):
// same title, price and image; the description overlaps by at least 90 %;
// at least 80 % of the specifications and features are found.
func sameProduct(got, want *Product) (bool, string) {
	if !strings.EqualFold(normalize.Text(got.Title), normalize.Text(want.Title)) {
		return false, fmt.Sprintf("title %q != %q", got.Title, want.Title)
	}
	if !samePrice(got.Price.Current, want.Price.Current) {
		return false, "current price differs"
	}
	if want.Price.Regular != nil && !samePrice(got.Price.Regular, want.Price.Regular) {
		return false, "regular price differs"
	}
	if want.ImageURL != nil && (got.ImageURL == nil || *got.ImageURL != *want.ImageURL) {
		return false, "image differs"
	}
	if want.Description != nil {
		if got.Description == nil || overlap(*want.Description, *got.Description) < 0.9 {
			return false, "description differs"
		}
	}
	if n := len(want.Specs); n > 0 {
		have := map[string]bool{}
		for _, s := range got.Specs {
			have[strings.ToLower(s.Name+"\x00"+s.Value)] = true
		}
		found := 0
		for _, s := range want.Specs {
			if have[strings.ToLower(s.Name+"\x00"+s.Value)] {
				found++
			}
		}
		if float64(found) < 0.8*float64(n) {
			return false, fmt.Sprintf("only %d of %d specs found", found, n)
		}
	}
	if n := len(want.Features); n > 0 {
		found := 0
		for _, f := range want.Features {
			if slices.ContainsFunc(got.Features, func(g string) bool { return strings.EqualFold(g, f) }) {
				found++
			}
		}
		if float64(found) < 0.8*float64(n) {
			return false, fmt.Sprintf("only %d of %d features found", found, n)
		}
	}
	return true, ""
}

func samePrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// overlap is the share of a's words found in b.
func overlap(a, b string) float64 {
	aw := strings.Fields(strings.ToLower(normalize.Text(a)))
	if len(aw) == 0 {
		return 1
	}
	bw := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(normalize.Text(b))) {
		bw[w] = true
	}
	n := 0
	for _, w := range aw {
		if bw[w] {
			n++
		}
	}
	return float64(n) / float64(len(aw))
}

func stringList(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// Result records how a request on an active learned config went: a success
// resets failures_in_a_row; DemoteAfter failures in a row demote it, so
// the host goes back to the AI crawler and is learned again.
func (l *Learner) Result(entry *siteconfig.Entry, ok bool) {
	if l == nil || entry == nil || entry.Origin != siteconfig.OriginLearned {
		return
	}
	host := entry.Domains[0]
	mu := l.lock(host)
	mu.Lock()
	defer mu.Unlock()
	path := entry.File
	raw, err := siteconfig.ReadLearnedRaw(path)
	if err != nil {
		return
	}
	block, _ := raw["learned"].(map[string]any)
	if block == nil || block["status"] != siteconfig.LearnedActive {
		return
	}
	failures, _ := block["failures_in_a_row"].(float64)
	switch {
	case ok && failures == 0:
		return
	case ok:
		block["failures_in_a_row"] = 0
	default:
		failures++
		block["failures_in_a_row"] = failures
		if int(failures) >= l.s.DemoteAfter {
			block["status"] = siteconfig.LearnedDemoted
			block["shadow_matches"] = []string{}
			l.log.Warn("learned config demoted after repeated failures", zap.String("host", host), zap.Float64("failures", failures))
		}
	}
	raw["learned"] = block
	if err := siteconfig.WriteLearned(path, raw); err != nil {
		l.log.Error("could not write learned config", zap.String("host", host), zap.Error(err))
	}
}
