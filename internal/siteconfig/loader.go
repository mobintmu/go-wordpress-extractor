package siteconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
)

const (
	TemplatesDir = "templates"
	SitesDir     = "sites"

	defaultUserAgent = "ProductExtractor/1.0"
	defaultDelayMS   = 1000
	defaultTimeout   = 30
	defaultRetries   = 2
)

var (
	siteIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	hostPattern    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9-]{2,63}$`)
	templateNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// Entry is one site file after loading. When the file is invalid, Site is
// nil and Err explains why; the entry still claims its domains so requests
// get CONFIG_INVALID instead of SITE_NOT_CONFIGURED.
type Entry struct {
	File     string
	SiteID   string
	Version  string
	Template string
	Domains  []string
	Site     *Site
	Err      error
}

// Status returns enabled, disabled, or invalid.
func (e *Entry) Status() string {
	switch {
	case e.Err != nil:
		return "invalid"
	case !e.Site.IsEnabled():
		return "disabled"
	default:
		return "enabled"
	}
}

// LoadDir reads templates and sites from dir. Individual bad files become
// invalid entries; an error is returned only when the directory itself
// cannot be read.
func LoadDir(dir string) ([]*Entry, error) {
	templates, err := readTemplates(filepath.Join(dir, TemplatesDir))
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, SitesDir, "*.json"))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, SitesDir)); err != nil {
		return nil, fmt.Errorf("read sites directory: %w", err)
	}
	sort.Strings(files)
	entries := make([]*Entry, 0, len(files))
	for _, f := range files {
		entries = append(entries, loadSite(f, templates))
	}
	markDuplicateDomains(entries)
	return entries, nil
}

type rawTemplate struct {
	data map[string]any
	err  error
}

func readTemplates(dir string) (map[string]rawTemplate, error) {
	out := map[string]rawTemplate{}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		m, err := readObject(f)
		out[name] = rawTemplate{data: m, err: err}
	}
	return out, nil
}

func readObject(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if m == nil {
		return nil, errors.New("file must contain a JSON object")
	}
	return m, nil
}

func loadSite(path string, templates map[string]rawTemplate) *Entry {
	e := &Entry{File: path}
	// Fallback identity from the file name, so a broken file still claims its domain.
	base := strings.TrimSuffix(filepath.Base(path), ".json")
	e.Domains = []string{NormalizeHost(base)}
	e.SiteID = base

	raw, err := readObject(path)
	if err != nil {
		e.Err = err
		return e
	}
	if id, ok := raw["site_id"].(string); ok && id != "" {
		e.SiteID = id
	}
	if v, ok := raw["version"].(string); ok {
		e.Version = v
	}
	if t, ok := raw["extends"].(string); ok {
		e.Template = t
	}
	if ds, ok := raw["domains"].([]any); ok && len(ds) > 0 {
		var domains []string
		for _, d := range ds {
			if s, ok := d.(string); ok {
				domains = append(domains, NormalizeHost(s))
			}
		}
		if len(domains) > 0 {
			e.Domains = domains
		}
	}

	merged, err := resolve(raw, templates, map[string]bool{})
	if err != nil {
		e.Err = err
		return e
	}
	site, err := decodeSite(merged)
	if err != nil {
		e.Err = err
		return e
	}
	if err := Validate(site); err != nil {
		e.Err = err
		return e
	}
	e.Site = site
	e.SiteID, e.Version, e.Domains = site.SiteID, site.Version, site.Domains
	return e
}

// resolve applies the extends chain: each parent is merged under its child.
func resolve(obj map[string]any, templates map[string]rawTemplate, seen map[string]bool) (map[string]any, error) {
	parentName, _ := obj["extends"].(string)
	if parentName == "" {
		return obj, nil
	}
	if !templateNameRe.MatchString(parentName) {
		return nil, fmt.Errorf("extends: invalid template name %q", parentName)
	}
	if seen[parentName] {
		return nil, fmt.Errorf("extends: template cycle at %q", parentName)
	}
	seen[parentName] = true
	t, ok := templates[parentName]
	if !ok {
		return nil, fmt.Errorf("extends: template %q not found in %s/", parentName, TemplatesDir)
	}
	if t.err != nil {
		return nil, fmt.Errorf("template %q: %w", parentName, t.err)
	}
	parent, err := resolve(t.data, templates, seen)
	if err != nil {
		return nil, err
	}
	merged := Merge(parent, obj)
	merged["extends"] = obj["extends"]
	return merged, nil
}

// Merge deep-merges override onto base. Objects merge key by key; arrays
// and scalars in override replace base. Neither input is modified.
func Merge(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if bm, ok := out[k].(map[string]any); ok {
			if om, ok := v.(map[string]any); ok {
				out[k] = Merge(bm, om)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func decodeSite(m map[string]any) (*Site, error) {
	stripMeta(m)
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var s Site
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &s, nil
}

// stripMeta removes "$schema", "$comment", and other "$" keys at any depth.
func stripMeta(m map[string]any) {
	for k, v := range m {
		if strings.HasPrefix(k, "$") {
			delete(m, k)
			continue
		}
		if child, ok := v.(map[string]any); ok {
			stripMeta(child)
		}
	}
}

// Validate applies defaults and checks a decoded site. It mirrors
// configs/schema/site-config.v1.json and adds checks JSON Schema cannot
// express, such as compiling selectors and regular expressions.
func Validate(s *Site) error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }

	if s.SchemaVersion != 1 {
		add("schema_version must be 1")
	}
	if !siteIDPattern.MatchString(s.SiteID) {
		add("site_id %q must be lowercase letters, digits, '-' or '_'", s.SiteID)
	}
	if strings.TrimSpace(s.Version) == "" {
		add("version is required")
	}
	if len(s.Domains) == 0 {
		add("domains must list at least one hostname")
	}
	for i, d := range s.Domains {
		n := NormalizeHost(d)
		if !hostPattern.MatchString(n) {
			add("domains[%d] %q is not a valid hostname", i, d)
		}
		s.Domains[i] = n
	}

	// Fetch defaults and ranges.
	if s.Fetch.UserAgent == "" {
		s.Fetch.UserAgent = defaultUserAgent
	}
	if s.Fetch.DelayMS == nil {
		s.Fetch.DelayMS = intPtr(defaultDelayMS)
	}
	if s.Fetch.TimeoutSeconds == 0 {
		s.Fetch.TimeoutSeconds = defaultTimeout
	}
	if s.Fetch.Retries == nil {
		s.Fetch.Retries = intPtr(defaultRetries)
	}
	if s.Fetch.RespectRobotsTxt == nil {
		t := true
		s.Fetch.RespectRobotsTxt = &t
	}
	if *s.Fetch.DelayMS < 0 || *s.Fetch.DelayMS > 60000 {
		add("fetch.delay_ms must be between 0 and 60000")
	}
	if s.Fetch.TimeoutSeconds < 1 || s.Fetch.TimeoutSeconds > 120 {
		add("fetch.timeout_seconds must be between 1 and 120")
	}
	if *s.Fetch.Retries < 0 || *s.Fetch.Retries > 5 {
		add("fetch.retries must be between 0 and 5")
	}

	// Locale.
	if s.Locale.PriceUnit == "" {
		s.Locale.PriceUnit = UnitToman
	}
	if s.Locale.PriceUnit != UnitToman && s.Locale.PriceUnit != UnitRial {
		add("locale.price_unit must be %q or %q", UnitToman, UnitRial)
	}

	// Product page.
	p := &s.Pages.Product
	if len(p.Match) == 0 {
		add("pages.product.match must list at least one pattern")
	}
	s.compiledMatch = nil
	for i, m := range p.Match {
		re, err := regexp.Compile(m)
		if err != nil {
			add("pages.product.match[%d]: %v", i, err)
			continue
		}
		s.compiledMatch = append(s.compiledMatch, re)
	}
	checkSelector := func(name, sel string, required bool) {
		if sel == "" {
			if required {
				add("%s is required", name)
			}
			return
		}
		if _, err := cascadia.Compile(sel); err != nil {
			add("%s: invalid selector %q: %v", name, sel, err)
		}
	}
	checkSelector("pages.product.root", p.Root, true)
	checkSelector("pages.product.price_area", p.PriceArea, false)
	checkSelector("pages.product.availability.out_of_stock", p.Availability.OutOfStock, false)
	checkSelector("pages.product.availability.order_button", p.Availability.OrderButton, false)
	if len(p.Fields) == 0 {
		add("pages.product.fields must define at least title")
	}
	for name, f := range p.Fields {
		if !isKnownField(name) {
			add("pages.product.fields.%s is not a known field (known: %s)", name, strings.Join(KnownFields, ", "))
			continue
		}
		checkSelector("pages.product.fields."+name+".selector", f.Selector, true)
		switch f.Pick {
		case "":
			f.Pick = PickFirst
		case PickFirst, PickLast, PickJoin:
		default:
			add("pages.product.fields.%s.pick must be first, last, or join", name)
		}
		for _, a := range f.Attr {
			if strings.TrimSpace(a) == "" {
				add("pages.product.fields.%s.attr must not contain empty names", name)
			}
		}
		p.Fields[name] = f
	}

	// Validation rules.
	if s.Validation.RequiredFields == nil {
		s.Validation.RequiredFields = []string{FieldTitle}
	}
	for _, rf := range s.Validation.RequiredFields {
		if !isKnownField(rf) {
			add("validation.required_fields: unknown field %q", rf)
		} else if _, ok := p.Fields[rf]; !ok {
			add("validation.required_fields: %q has no selector in pages.product.fields", rf)
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func isKnownField(name string) bool {
	for _, k := range KnownFields {
		if k == name {
			return true
		}
	}
	return false
}

func markDuplicateDomains(entries []*Entry) {
	owner := map[string]*Entry{}
	for _, e := range entries {
		for _, d := range e.Domains {
			if prev, ok := owner[d]; ok && prev != e {
				msg := fmt.Errorf("domain %q is claimed by both %s and %s", d, filepath.Base(prev.File), filepath.Base(e.File))
				prev.Err, prev.Site = msg, nil
				e.Err, e.Site = msg, nil
				continue
			}
			owner[d] = e
		}
	}
}

// NormalizeHost lowercases a hostname and removes a trailing dot.
func NormalizeHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}

func intPtr(v int) *int { return &v }
