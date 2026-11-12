package app

import (
	"time"

	"go-wordpress/internal/apikey"
	"go-wordpress/internal/config"
	"go-wordpress/internal/extract"
	"go-wordpress/internal/fetch"
	"go-wordpress/internal/health"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/metrics"
	"go-wordpress/internal/server"
	"go-wordpress/internal/server/drain"
	"go-wordpress/internal/siteconfig"
	"go-wordpress/internal/sites"
	"go-wordpress/pkg/logger"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// AppOptions lists every component. Fx builds them and runs start and
// stop hooks. The service has no database, cache, or background jobs.
func AppOptions() fx.Option {
	return fx.Options(
		fx.Provide(
			logger.NewLogger,
			config.NewConfig,
			drain.New,
			metrics.New,
			limiter.New,
			apikey.NewStore,
			siteconfig.NewRegistry,
			fetchOptions,
			fetch.New,
			newBrowserFetcher,
			newFetchRouter,
			extract.NewService,
			extract.NewHandler,
			sites.NewHandler,
			health.New,
			server.NewGinEngine,
			server.CreateHTTPServer,
		),
		fx.Invoke(
			// Registered first so it stops last and flushes every other stop log.
			logger.RegisterLoggerLifecycle,
			apikey.RegisterReloader,
			siteconfig.RegisterReloader,
			server.RegisterRoutes,
			server.StartHTTPServer,
		),
	)
}

// NewApp builds the application. The stop timeout is longer than the
// request limit, so running requests can finish while the instance drains.
func NewApp(extra ...fx.Option) *fx.App {
	opts := []fx.Option{
		AppOptions(),
		fx.StopTimeout(config.MaxRequestDurationFromEnv() + 30*time.Second),
		fx.WithLogger(logger.FxLogger),
	}
	return fx.New(append(opts, extra...)...)
}

// fetchOptions applies the configured outbound proxy to the fetch defaults.
func fetchOptions(cfg *config.Config) fetch.Options {
	o := fetch.DefaultOptions()
	o.Proxy = cfg.ProxyURL()
	return o
}

// newBrowserFetcher builds the "browser" fetch engine when
// APP_BROWSER_ENABLED is on, and returns nil otherwise. Chrome is launched
// at startup, so a missing driver or browser stops the app with a clear
// error instead of failing requests later.
func newBrowserFetcher(lc fx.Lifecycle, cfg *config.Config, opts fetch.Options, lim *limiter.Limiter, log *zap.Logger) *fetch.BrowserFetcher {
	if !cfg.BrowserEnabled {
		return nil
	}
	b := fetch.NewBrowserFetcher(opts, fetch.BrowserOptions{
		Channel:  cfg.BrowserChannel,
		Headless: cfg.BrowserHeadless,
		MaxPages: cfg.BrowserMaxPages,
	}, lim, log)
	lc.Append(fx.Hook{
		OnStart: b.Start,
		OnStop:  b.Stop,
	})
	return b
}

// newFetchRouter registers the fetch engines (strategies). The router picks
// one per request from the site config's fetch.engine.
func newFetchRouter(httpEngine *fetch.Fetcher, browser *fetch.BrowserFetcher) extract.Fetcher {
	return fetch.NewRouter(httpEngine, browser)
}
