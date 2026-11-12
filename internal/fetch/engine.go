package fetch

import (
	"context"
	"net/url"

	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/siteconfig"
)

// Engine is the Strategy for retrieving one product page. Each
// implementation is one way of fetching (Colly over net/http, a real Chrome
// browser, ...); the Router picks one per request from the site config's
// fetch.engine. Every engine returns the same Page and Failure types, so
// page checks, extraction and normalization do not depend on the engine.
type Engine interface {
	// Name is the value a site config uses to select this engine.
	Name() string
	// Fetch retrieves target for site within ctx's deadline. The Page is
	// filled as far as the fetch got, so it can serve as evidence when the
	// fetch fails.
	Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure)
}

// Engine names, as written in fetch.engine.
const (
	EngineHTTP    = siteconfig.EngineHTTP
	EngineBrowser = siteconfig.EngineBrowser
)

// Name makes the Colly fetcher the "http" strategy. Nothing else in it
// changes.
func (f *Fetcher) Name() string { return EngineHTTP }

// Compile-time checks that the concrete strategies satisfy Engine.
var (
	_ Engine = (*Fetcher)(nil)
	_ Engine = (*BrowserFetcher)(nil)
)
