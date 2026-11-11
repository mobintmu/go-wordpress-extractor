package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/server/drain"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// NewGinEngine builds the Gin engine with request logging and panic
// recovery. There is no timeout middleware: each extraction carries its
// own deadline.
func NewGinEngine(cfg *config.Config) *gin.Engine {
	if cfg.IsDevelopment() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
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

// StartHTTPServer listens on start and drains on stop: /ready and new
// extractions return 503, then running requests get up to
// MAX_REQUEST_DURATION to finish.
func StartHTTPServer(lc fx.Lifecycle, srv *http.Server, cfg *config.Config, d *drain.State, log *zap.Logger) {
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
			d.Start()
			log.Info("draining: waiting for running requests", zap.Duration("limit", cfg.ShutdownTimeout()))
			shutdownCtx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout())
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Error("shutdown did not finish cleanly", zap.Error(err))
				return err
			}
			log.Info("HTTP server stopped")
			return nil
		},
	})
}
