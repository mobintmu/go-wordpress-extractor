package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/server/drain"
	"go-wordpress/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// NewGinEngine builds the Gin engine with JSON request logging and panic
// recovery. There is no timeout middleware: each extraction carries its
// own deadline.
func NewGinEngine(cfg *config.Config, log *zap.Logger) *gin.Engine {
	if cfg.IsDevelopment() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(logger.GinMiddleware(log), logger.GinRecovery(log))
	return r
}

// CreateHTTPServer applies the connection limits from the proposal.
func CreateHTTPServer(engine *gin.Engine, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:              net.JoinHostPort(cfg.HTTPAddress, strconv.Itoa(cfg.HTTPPort)),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      cfg.MaxRequestDuration + 10*time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}

// ForceGrace is how long cancelled requests get to return after the drain
// limit is reached, before their connections are closed.
const ForceGrace = 5 * time.Second

// drainLogEvery is how often the number of running requests is logged
// while draining.
const drainLogEvery = 5 * time.Second

// StartHTTPServer listens on start and drains on stop:
//
//  1. /ready and new extractions return 503 SHUTTING_DOWN, and the
//     listener stops accepting connections.
//  2. Running requests get up to the shutdown limit (MAX_REQUEST_DURATION
//     plus 10s, or less if the stop context ends sooner) to finish. Their
//     count is logged every few seconds.
//  3. If any are still running at the limit, their contexts are cancelled
//     so they answer 503 SHUTTING_DOWN, and after ForceGrace every
//     remaining connection is closed.
//
// The hook returns only when no request is running, so later stop hooks
// (the browser engine) never close resources under a live request.
func StartHTTPServer(lc fx.Lifecycle, srv *http.Server, cfg *config.Config, d *drain.State, lim *limiter.Limiter, log *zap.Logger) {
	// Every request context derives from base, so cancelling it stops all
	// running extractions at once.
	base, cancelBase := context.WithCancel(context.Background())
	srv.BaseContext = func(net.Listener) context.Context { return base }
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			scheme := "http"
			if cfg.TLSEnabled() {
				scheme = "https"
			}
			log.Info("HTTP server starting", zap.String("addr", ln.Addr().String()), zap.String("scheme", scheme))
			go func() {
				var err error
				if cfg.TLSEnabled() {
					err = srv.ServeTLS(ln, cfg.TLSCertFile, cfg.TLSKeyFile)
				} else {
					err = srv.Serve(ln)
				}
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Fatal("HTTP server failed", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			defer cancelBase()
			return drainServer(ctx, srv, cfg.ShutdownTimeout(), d, lim, cancelBase, log)
		},
	})
}

// drainServer runs the stop steps described on StartHTTPServer.
func drainServer(ctx context.Context, srv *http.Server, limit time.Duration, d *drain.State, lim *limiter.Limiter, cancelRequests context.CancelFunc, log *zap.Logger) error {
	d.Start()
	started := time.Now()
	// Keep ForceGrace of the stop context for the forced step.
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - ForceGrace; left < limit {
			limit = max(left, 0)
		}
	}
	log.Info("draining: stopped accepting requests; waiting for running requests",
		zap.Int("running", lim.Active()), zap.Duration("limit", limit))

	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit)
	defer cancel()
	stopProgress := logProgress(drainCtx, lim, started, log)
	err := srv.Shutdown(drainCtx)
	stopProgress()
	if err == nil {
		log.Info("HTTP server stopped; all requests finished", zap.Duration("took", time.Since(started)))
		return nil
	}

	log.Warn("drain limit reached; cancelling running requests",
		zap.Int("running", lim.Active()), zap.Duration("waited", time.Since(started)))
	cancelRequests()
	if waitIdle(lim, ForceGrace) {
		log.Info("cancelled requests returned")
	} else {
		log.Error("requests still running after cancel; closing their connections", zap.Int("running", lim.Active()))
	}
	if cerr := srv.Close(); cerr != nil {
		log.Error("closing connections failed", zap.Error(cerr))
	}
	log.Warn("HTTP server stopped after forcing running requests to end", zap.Duration("took", time.Since(started)))
	return nil
}

// logProgress logs how many requests are still running until stop is called.
func logProgress(ctx context.Context, lim *limiter.Limiter, started time.Time, log *zap.Logger) (stop func()) {
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		t := time.NewTicker(drainLogEvery)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				log.Info("draining: still waiting", zap.Int("running", lim.Active()), zap.Duration("waited", time.Since(started).Round(time.Second)))
			}
		}
	}()
	return func() {
		close(done)
		<-exited
	}
}

// waitIdle waits up to limit for every extraction slot to be released.
func waitIdle(lim *limiter.Limiter, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for lim.Active() > 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}
