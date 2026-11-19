package siteconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Learned config statuses. Only an active learned config serves requests.
const (
	// LearnedCandidate: the selectors reproduced the AI result on the page
	// they were learned from and now run in shadow next to the AI crawler.
	LearnedCandidate = "candidate"
	// LearnedActive: proven on enough pages; the config serves its host.
	LearnedActive = "active"
	// LearnedDemoted: failed too often while active; the host is served by
	// the AI crawler again and relearned.
	LearnedDemoted = "demoted"
	// LearnedRejected: a person rejected it; the host is never relearned.
	LearnedRejected = "rejected"
)

// Learned is the provenance and state of a config written by the AI
// crawler's selector learning.
type Learned struct {
	Status string `json:"status"`
	// Engine is the AI engine that learned the selectors. Optional: files
	// learned before ai-crawler-cloudflare existed have none, which means
	// ai-crawler-avalai.
	Engine        string `json:"engine,omitempty"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
	LearnedFrom   string `json:"learned_from"`
	LearnedAt     string `json:"learned_at"`
	// ShadowMatches lists the different product URLs on which the learned
	// selectors gave the same product as the AI crawler.
	ShadowMatches []string `json:"shadow_matches"`
	// FailuresInARow counts failed requests while active; a success resets it.
	FailuresInARow int `json:"failures_in_a_row"`
}

// DefaultAISiteID is the site_id of the built-in default rule.
const DefaultAISiteID = "ai-crawler-default"

// DefaultAIVersion is the config_version of the built-in default rule
// with the ai-crawler-avalai engine. With another engine the engine's
// provider is appended ("builtin-1+cloudflare"), so config_version changes
// when APP_DEFAULT_ENGINE does.
const DefaultAIVersion = "builtin-1"

// DefaultAIVersionFor is the config_version of the default rule served by
// engine.
func DefaultAIVersionFor(engine string) string {
	if engine == EngineAICrawler {
		return DefaultAIVersion
	}
	return DefaultAIVersion + "+" + strings.TrimPrefix(engine, "ai-crawler-")
}

// LearnedEngine is the AI engine that learned e, ai-crawler-avalai for
// files that do not say.
func (e *Entry) LearnedEngine() string {
	if e.Site != nil && e.Site.Learned != nil && e.Site.Learned.Engine != "" {
		return e.Site.Learned.Engine
	}
	return EngineAICrawler
}

func validateLearned(s *Site, add func(string, ...any)) {
	switch s.Learned.Status {
	case LearnedCandidate, LearnedActive, LearnedDemoted, LearnedRejected:
	default:
		add("learned.status must be %q, %q, %q or %q", LearnedCandidate, LearnedActive, LearnedDemoted, LearnedRejected)
	}
	if e := s.Learned.Engine; e != "" && !IsAIEngine(e) {
		add("learned.engine %q must be %q or %q", e, EngineAICrawler, EngineAICloudflare)
	}
	if s.Fetch.Engine != EngineHTTP && s.Fetch.Engine != EngineBrowser {
		add("a learned config may only use fetch.engine %q or %q", EngineHTTP, EngineBrowser)
	}
	if !*s.Fetch.RespectRobotsTxt {
		add("a learned config must respect robots.txt")
	}
}

// LearnedDomains returns the domains a learned config for host may claim:
// the host without "www." and with it.
func LearnedDomains(host string) []string {
	base := strings.TrimPrefix(NormalizeHost(host), "www.")
	return []string{base, "www." + base}
}

// LearnedFile is the file name of the learned config for host.
func LearnedFile(dir, host string) string {
	return filepath.Join(dir, LearnedDomains(host)[0]+".json")
}

var nonIDChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// LearnedSiteID builds a valid site_id for a learned config of host.
func LearnedSiteID(host string) string {
	id := "learned-" + nonIDChars.ReplaceAllString(LearnedDomains(host)[0], "-")
	if len(id) > 64 {
		id = id[:64]
	}
	return strings.TrimRight(id, "-_")
}

// LoadLearned reads every learned config in dir. A missing directory is not
// an error: nothing has been learned yet. Each entry keeps its status in
// Site.Learned; only active, valid entries should serve requests.
func LoadLearned(dir string) ([]*Entry, error) {
	if dir == "" {
		return nil, nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	sort.Strings(files)
	out := make([]*Entry, 0, len(files))
	for _, f := range files {
		e := loadSite(f, nil)
		e.Origin = OriginLearned
		checkLearnedEntry(e)
		out = append(out, e)
	}
	return out, nil
}

// checkLearnedEntry adds the rules that only apply to learned files: the
// learned block is required, and the config may only claim the host its
// file is named after (and that host with "www.").
func checkLearnedEntry(e *Entry) {
	if e.Err != nil {
		return
	}
	if e.Site.Learned == nil {
		e.Err, e.Site = errors.New("a file in the learned directory must have a learned block"), nil
		return
	}
	allowed := LearnedDomains(strings.TrimSuffix(filepath.Base(e.File), ".json"))
	for _, d := range e.Site.Domains {
		if d != allowed[0] && d != allowed[1] {
			e.Err, e.Site = fmt.Errorf("a learned config may only claim %s and %s, not %q", allowed[0], allowed[1], d), nil
			return
		}
	}
	if e.Site.Extends != "" {
		e.Err, e.Site = errors.New("a learned config may not use extends"), nil
	}
}

// LearnedStatus returns the learned status of e, or "" when e is not a
// valid learned config.
func (e *Entry) LearnedStatus() string {
	if e == nil || e.Site == nil || e.Site.Learned == nil {
		return ""
	}
	return e.Site.Learned.Status
}

// WriteLearned writes a learned config atomically (temp file and rename),
// so the registry never loads half a file.
func WriteLearned(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".learned-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// ReadLearnedRaw reads a learned file as a JSON object, keeping "$comment"
// and every other key, so it can be changed and written back.
func ReadLearnedRaw(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", filepath.Base(path), err)
	}
	return m, nil
}

// DefaultAIEntry builds the built-in default rule: any host without a site
// config or an active learned config is served by engine (from
// APP_DEFAULT_ENGINE). delayMS is the gap between two fetches of the same
// host.
func DefaultAIEntry(engine string, delayMS int) *Entry {
	t := true
	retries := 1
	s := &Site{
		SchemaVersion: 1,
		SiteID:        DefaultAISiteID,
		Version:       DefaultAIVersionFor(engine),
		Fetch: Fetch{
			Engine:           engine,
			UserAgent:        defaultUserAgent,
			DelayMS:          &delayMS,
			TimeoutSeconds:   20,
			Retries:          &retries,
			RespectRobotsTxt: &t,
		},
		Locale:     Locale{PriceUnit: UnitAuto},
		Pages:      Pages{Product: ProductPage{Match: []string{".*"}}},
		Validation: Validation{RequiredFields: []string{FieldTitle}},
		compiledMatch: []*regexp.Regexp{
			regexp.MustCompile(".*"),
		},
	}
	return &Entry{
		File:    "(built-in default rule)",
		SiteID:  DefaultAISiteID,
		Version: s.Version,
		Site:    s,
		Origin:  OriginDefault,
	}
}

// ForHost returns a copy of the site that serves only host: its domains
// are host and its "www." form, and throttling, cookies and the per-site
// limiter are keyed on "ai:<host>". Used for the default AI rule, which has
// no domains of its own. robots.txt can not be turned off.
func (s *Site) ForHost(host string) *Site {
	c := *s
	c.Domains = LearnedDomains(host)
	c.limitKey = "ai:" + c.Domains[0]
	t := true
	c.Fetch.RespectRobotsTxt = &t
	return &c
}

// WithEngine returns a copy of the site that uses engine. A learned site
// falls back to the AI crawler this way when its selectors fail.
func (s *Site) WithEngine(engine string) *Site {
	c := *s
	c.Fetch.Engine = engine
	if IsAIEngine(engine) && c.Locale.PriceUnit == "" {
		c.Locale.PriceUnit = UnitAuto
	}
	return &c
}

// ParseSite decodes and validates a site config given as a JSON object,
// without templates. m is not modified.
func ParseSite(m map[string]any) (*Site, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var cp map[string]any
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, err
	}
	site, err := decodeSite(cp)
	if err != nil {
		return nil, err
	}
	if err := Validate(site); err != nil {
		return nil, err
	}
	return site, nil
}
