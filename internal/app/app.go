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
	"go-wordpress/pkg/logger"

	"go.uber.org/fx"
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
			fetch.DefaultOptions,
			fetch.New,
			extract.NewService,
			extract.NewHandler,
			health.New,
			server.NewGinEngine,
			server.CreateHTTPServer,
		),
		fx.Invoke(
			apikey.RegisterReloader,
			siteconfig.RegisterReloader,
			server.RegisterRoutes,
			server.StartHTTPServer,
			logger.RegisterLoggerLifecycle,
		),
	)
}

// NewApp builds the application. The stop timeout is longer than the
// request limit, so running requests can finish while the instance drains.
func NewApp(extra ...fx.Option) *fx.App {
	opts := []fx.Option{
		AppOptions(),
		fx.StopTimeout(config.MaxRequestDurationFromEnv() + 30*time.Second),
	}
	return fx.New(append(opts, extra...)...)
}
