// Package logger builds the process-wide JSON logger. Every log line the
// server writes (application logs, HTTP access logs, Gin and Fx internals,
// and anything sent through the standard library "log" package) goes
// through this logger, so stdout holds one JSON object per line.
package logger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	once   sync.Once
	shared *zap.Logger
	err    error
)

// Init builds the shared logger once, installs it as zap's global logger,
// and redirects the standard library "log" package and Gin's debug output
// to it. Call it first in main so that early messages are JSON too.
func Init() (*zap.Logger, error) {
	once.Do(func() {
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"stdout"}
		cfg.EncoderConfig.TimeKey = "time"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		shared, err = cfg.Build()
		if err != nil {
			return
		}
		zap.ReplaceGlobals(shared)
		zap.RedirectStdLog(shared)

		ginLog := shared.Named("gin")
		gin.DebugPrintFunc = func(format string, values ...any) {
			ginLog.Debug(strings.TrimSpace(fmt.Sprintf(format, values...)))
		}
		gin.DebugPrintRouteFunc = func(method, path, handler string, handlers int) {
			ginLog.Debug("route", zap.String("method", method), zap.String("path", path),
				zap.String("handler", handler), zap.Int("handlers", handlers))
		}
	})
	return shared, err
}

// NewLogger provides the shared logger to Fx.
func NewLogger() (*zap.Logger, error) { return Init() }

// FxLogger makes Fx write its lifecycle events as JSON.
func FxLogger(log *zap.Logger) fxevent.Logger {
	l := &fxevent.ZapLogger{Logger: log.Named("fx")}
	l.UseLogLevel(zapcore.DebugLevel)
	return l
}

// GinMiddleware writes one JSON access-log line per request, replacing
// gin.Logger.
func GinMiddleware(log *zap.Logger) gin.HandlerFunc {
	log = log.Named("http")
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", time.Since(start)),
			zap.Int("size", c.Writer.Size()),
			zap.String("user_agent", c.Request.UserAgent()),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}
		switch {
		case status >= 500:
			log.Error("request", fields...)
		case status >= 400:
			log.Warn("request", fields...)
		default:
			log.Info("request", fields...)
		}
	}
}

// GinRecovery recovers from panics in handlers and logs them as JSON,
// replacing gin.Recovery.
func GinRecovery(log *zap.Logger) gin.HandlerFunc {
	log = log.Named("http")
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, rec any) {
		log.Error("panic recovered",
			zap.Any("panic", rec),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Stack("stack"),
		)
		c.AbortWithStatus(500)
	})
}

func RegisterLoggerLifecycle(lc fx.Lifecycle, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			log.Info("logger stopping")
			if err := log.Sync(); err != nil && !isIgnorableSyncError(err) {
				return err
			}
			return nil
		},
	})
}

func isIgnorableSyncError(err error) bool {
	return strings.Contains(err.Error(), "invalid argument") ||
		strings.Contains(err.Error(), "bad file descriptor")
}
