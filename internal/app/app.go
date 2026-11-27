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
			newAIClients,
			newAIEngines,
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
	b := browserFetcher(cfg, opts, lim, log)
	if b != nil {
		lc.Append(fx.Hook{OnStart: b.Start, OnStop: b.Stop})
	}
	return b
}

func browserFetcher(cfg *config.Config, opts fetch.Options, lim *limiter.Limiter, log *zap.Logger) *fetch.BrowserFetcher {
	if !cfg.BrowserEnabled {
		return nil
	}
	return fetch.NewBrowserFetcher(opts, fetch.BrowserOptions{
		Channel:  cfg.BrowserChannel,
		Headless: cfg.BrowserHeadless,
		MaxPages: cfg.BrowserMaxPages,
	}, lim, log)
}

// newFetchRouter registers the fetch engines (strategies). The router picks
// one per request from the site config's fetch.engine (or the first enabled
// fetch.alternate_engines entry). The search engines ("duckduckgo",
// "brave", "mojeek", "yahoo", "yandex") are registered only when their
// APP_<ENGINE>_ENABLED switch is on.
func newFetchRouter(cfg *config.Config, opts fetch.Options, httpEngine *fetch.Fetcher, browser *fetch.BrowserFetcher, ai aiEngines, log *zap.Logger) extract.Fetcher {
	engines := []fetch.Engine{httpEngine, browser}
	for _, e := range ai {
		engines = append(engines, e)
	}
	engines = append(engines, fetch.NewSearchEngines(opts, fetch.SearchSettings(cfg.SearchSettings()), log)...)
	r := fetch.NewRouter(engines...)
	log.Info("fetch engines registered", zap.Strings("engines", r.Engines()))
	return r
}

// aiClients holds one LLM client per configured AI engine. A field is nil
// when its engine is not configured.
type aiClients struct {
	avalai     *aicrawler.Client
	cloudflare *aicrawler.Client
	gapgpt     *aicrawler.Client
}

// forEngine returns the client of engine, or nil.
func (c aiClients) forEngine(engine string) *aicrawler.Client {
	switch engine {
	case aicrawler.EngineAvalAI:
		return c.avalai
	case aicrawler.EngineCloudflare:
		return c.cloudflare
	case aicrawler.EngineGapGPT:
		return c.gapgpt
	}
	return nil
}

// aiEngines are the configured AI fetch engines, in a fixed order.
type aiEngines []*fetch.AIEngine

func aiSettings(cfg *config.Config, minIntervalMS int) aicrawler.Settings {
	return aicrawler.Settings{
		Timeout:         cfg.AICrawlerTimeout,
		MaxOutputTokens: cfg.AICrawlerMaxOutputTokens,
		Temperature:     cfg.AICrawlerTemperature,
		Retries:         cfg.AICrawlerRetries,
		MinInterval:     time.Duration(minIntervalMS) * time.Millisecond,
		ResponseFormat:  cfg.AICrawlerResponseFormat,
	}
}

// newAIClients builds the OpenAI-compatible clients of the AI engines whose
// settings are complete (APP_AI_CRAWLER_ENABLED plus the engine's own
// settings). They share the limits and metrics.
func newAIClients(cfg *config.Config, m *metrics.Metrics) aiClients {
	shared := func(minIntervalMS int) aicrawler.Settings {
		settings := aiSettings(cfg, minIntervalMS)
		settings.Observe = func(c aicrawler.Call) {
			if c.Cached {
				m.ObserveAICacheHit(c.Engine, c.Model, c.Host)
				return
			}
			m.ObserveAICall(c.Engine, c.Model, c.Purpose, c.Host, c.Outcome, c.InputTokens, c.OutputTokens, c.Duration)
		}
		return settings
	}
	var out aiClients
	if cfg.AvalAIConfigured() {
		out.avalai = aicrawler.New(aicrawler.AvalAI(cfg.AICrawlerBaseURL, cfg.AICrawlerModel, cfg.AICrawlerAPIKey, cfg.AICrawlerSearchModel), shared(cfg.AICrawlerMinIntervalMS))
	}
	if cfg.CloudflareConfigured() {
		out.cloudflare = aicrawler.New(aicrawler.Cloudflare(aicrawler.CloudflareSettings{
			AccountID:        cfg.CloudflareAccountID,
			GatewayID:        cfg.CloudflareGatewayID,
			Endpoint:         cfg.CloudflareEndpoint,
			BaseURL:          cfg.CloudflareBaseURL,
			Model:            cfg.CloudflareModel,
			ReasoningEffort:  cfg.CloudflareReasoningEffort,
			APIToken:         cfg.CloudflareAPIToken,
			ProviderAPIKey:   cfg.CloudflareProviderAPIKey,
			NoUnifiedBilling: cfg.CloudflareNoUnifiedBilling,
			CollectLog:       cfg.CloudflareCollectLog,
		}), shared(cfg.CloudflareMinIntervalMS))
	}
	if cfg.GapGPTConfigured() {
		out.gapgpt = gapgptClient(cfg, shared(cfg.AICrawlerMinIntervalMS))
	}
	return out
}

func gapgptClient(cfg *config.Config, settings aicrawler.Settings) *aicrawler.Client {
	return aicrawler.New(aicrawler.GapGPT(cfg.GapGPTEndpoint, cfg.GapGPTModel, cfg.GapGPTAPIKey), settings)
}

// newAIEngines builds one AI engine per configured client. They fetch with
// the http engine and, for JavaScript pages, with the browser engine when
// that is enabled. They share one answer cache, keyed on the engine too.
func newAIEngines(cfg *config.Config, httpEngine *fetch.Fetcher, browser *fetch.BrowserFetcher, clients aiClients, log *zap.Logger) aiEngines {
	cache := aicrawler.NewCache(cfg.AICrawlerCacheTTL)
	var out aiEngines
	for _, c := range []*aicrawler.Client{clients.avalai, clients.cloudflare, clients.gapgpt} {
		if e := fetch.NewAIEngine(httpEngine, browser, c, cache, cfg.AICrawlerMaxInputChars, log); e != nil {
			out = append(out, e)
		}
	}
	return out
}

// newLearner builds the selector learner when APP_AI_CRAWLER_LEARN_SELECTORS
// is on and APP_DEFAULT_ENGINE names an AI engine, and returns nil
// otherwise. Learning calls use the default engine: only websites without a
// rule are learned, and those are the ones it serves. Running learning jobs
// are cancelled and awaited on stop.
func newLearner(lc fx.Lifecycle, cfg *config.Config, clients aiClients, log *zap.Logger) *extract.Learner {
	client := clients.forEngine(cfg.DefaultAIEngine())
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
