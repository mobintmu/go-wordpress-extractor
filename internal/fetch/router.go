package fetch

import (
	"context"
	"net/url"
	"reflect"
	"sort"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/siteconfig"
)

// Router is the Strategy context: it holds the registered engines and
// delegates each fetch to the one the site config names in fetch.engine.
// It has no per-engine branches, so adding an engine is one new type that
// implements Engine plus registering it here.
type Router struct {
	engines  map[string]Engine
	fallback string
}

// NewRouter registers engines by name. A nil engine (for example the
// browser engine when it is disabled on this instance) is skipped. Sites
// with no fetch.engine use the "http" engine.
func NewRouter(engines ...Engine) *Router {
	r := &Router{engines: make(map[string]Engine, len(engines)), fallback: EngineHTTP}
	for _, e := range engines {
		if isNil(e) {
			continue
		}
		r.engines[e.Name()] = e
	}
	return r
}

// Engine returns the engine registered under name.
func (r *Router) Engine(name string) (Engine, bool) {
	e, ok := r.engines[name]
	return e, ok
}

// Engines returns the registered engine names, sorted.
func (r *Router) Engines() []string {
	out := make([]string, 0, len(r.engines))
	for n := range r.engines {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Fetch selects the site's engine and delegates to it. When fetch.engine is
// not enabled on this instance, the first enabled engine from
// fetch.alternate_engines is used instead. The choice is made once: the
// chosen engine's failure is returned as it is, never retried on another
// engine.
func (r *Router) Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure) {
	names := site.Engines()
	if names[0] == "" {
		names[0] = r.fallback
	}
	for _, name := range names {
		engine, ok := r.engines[name]
		if !ok {
			continue
		}
		page, fail := engine.Fetch(ctx, site, target)
		if page == nil {
			page = &Page{RequestedURL: target.String()}
		}
		if page.Engine == "" {
			page.Engine = name
		}
		return page, fail
	}
	if len(names) == 1 {
		return &Page{RequestedURL: target.String()}, diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceHigh,
			"The site config selects fetch engine %q, which is not enabled on this instance (enabled: %v).", names[0], r.Engines())
	}
	return &Page{RequestedURL: target.String()}, diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceHigh,
		"The site config selects fetch engine %q with alternates %q, and none of them is enabled on this instance (enabled: %v).",
		names[0], names[1:], r.Engines())
}

// isNil reports whether e is nil or a typed nil pointer, which fx and
// optional constructors produce for disabled engines.
func isNil(e Engine) bool {
	if e == nil {
		return true
	}
	v := reflect.ValueOf(e)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
