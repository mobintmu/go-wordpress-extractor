package app

import (
	"time"

	"go-wordpress/internal/aicrawler"
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
			newAIClient,
			newAIEngine,
			newLearner,
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
// one per request from the site config's fetch.engine (or the first enabled
// fetch.alternate_engines entry). The search engines ("duckduckgo",
// "brave", "mojeek", "yahoo", "yandex") are registered only when their
// APP_<ENGINE>_ENABLED switch is on.
func newFetchRouter(cfg *config.Config, opts fetch.Options, httpEngine *fetch.Fetcher, browser *fetch.BrowserFetcher, ai *fetch.AIEngine, log *zap.Logger) extract.Fetcher {
	engines := []fetch.Engine{httpEngine, browser, ai}
	engines = append(engines, fetch.NewSearchEngines(opts, fetch.SearchSettings(cfg.SearchSettings()), log)...)
	r := fetch.NewRouter(engines...)
	log.Info("fetch engines registered", zap.Strings("engines", r.Engines()))
	return r
}

// newAIClient builds the OpenAI-compatible client for the
// "ai-crawler-avalai" engine when APP_AI_CRAWLER_ENABLED is on, and returns
// nil otherwise.
func newAIClient(cfg *config.Config, m *metrics.Metrics) *aicrawler.Client {
	if !cfg.AICrawlerEnabled {
		return nil
	}
	return aicrawler.New(aicrawler.Settings{
		BaseURL:         cfg.AICrawlerBaseURL,
		Model:           cfg.AICrawlerModel,
		APIKey:          cfg.AICrawlerAPIKey,
		Timeout:         cfg.AICrawlerTimeout,
		MaxOutputTokens: cfg.AICrawlerMaxOutputTokens,
		Temperature:     cfg.AICrawlerTemperature,
		Retries:         cfg.AICrawlerRetries,
		MinInterval:     time.Duration(cfg.AICrawlerMinIntervalMS) * time.Millisecond,
		ResponseFormat:  cfg.AICrawlerResponseFormat,
		Observe: func(c aicrawler.Call) {
			if c.Cached {
				m.ObserveAICacheHit(c.Model, c.Host)
				return
			}
			m.ObserveAICall(c.Model, c.Purpose, c.Host, c.Outcome, c.InputTokens, c.OutputTokens, c.Duration)
		},
	})
}

// newAIEngine builds the "ai-crawler-avalai" engine, or nil when the AI
// crawler is off. It fetches with the http engine and, for JavaScript
// pages, with the browser engine when that is enabled.
func newAIEngine(cfg *config.Config, httpEngine *fetch.Fetcher, browser *fetch.BrowserFetcher, client *aicrawler.Client, log *zap.Logger) *fetch.AIEngine {
	return fetch.NewAIEngine(httpEngine, browser, client, aicrawler.NewCache(cfg.AICrawlerCacheTTL), cfg.AICrawlerMaxInputChars, log)
}

// newLearner builds the selector learner when the AI crawler and
// APP_AI_CRAWLER_LEARN_SELECTORS are on, and returns nil otherwise. Running
// learning jobs are cancelled and awaited on stop.
func newLearner(lc fx.Lifecycle, cfg *config.Config, client *aicrawler.Client, log *zap.Logger) *extract.Learner {
	if client == nil || !cfg.AICrawlerLearnSelectors {
		return nil
	}
	l := extract.NewLearner(client, extract.LearnSettings{
		Dir:               cfg.LearnedDir(),
		PromoteAfter:      cfg.AICrawlerLearnPromoteAfter,
		DemoteAfter:       cfg.AICrawlerLearnDemoteAfter,
		MaxAttemptsPerDay: cfg.AICrawlerLearnMaxAttemptsPerDay,
		DelayMS:           cfg.AICrawlerDefaultDelayMS,
	}, log)
	lc.Append(fx.Hook{OnStop: l.Stop})
	return l
}
