// Command server runs the stateless product extraction service.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go-wordpress/internal/app"
	"go-wordpress/internal/config"
	"go-wordpress/pkg/logger"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

//	@title			Product Extraction API
//	@version		1.0
//	@description	Extracts one product from one product page using per-site configuration.
//	@description	Each request responds within 3 minutes. Both extraction results return HTTP 200, so clients must read the status field.
//	@BasePath		/

//	@tag.name			Extraction
//	@tag.description	Product extraction for client applications.
//	@tag.name			Operations
//	@tag.description	Health, readiness, and operator endpoints.

//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-API-Key
//	@description				Client API key: gwp_<id>_<secret>

//	@securityDefinitions.basic	OpsBasicAuth
//	@description				Operator credentials (APP_OPS_USER and APP_OPS_PASSWORD_HASH) for Swagger UI.

func main() {
	if len(os.Args) > 1 && os.Args[1] == "diagnose-gapgpt" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "Usage: server diagnose-gapgpt <product URL>")
			os.Exit(2)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		config.LoadEnv()
		err := runGapGPTDiagnostic(ctx, os.Args[2])
		stop()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// Set up the JSON logger before anything else logs.
	log, err := logger.Init()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot start logger: %v\n", err)
		os.Exit(1)
	}
	config.LoadEnv()
	os.Exit(run(app.NewApp(), log))
}

func runGapGPTDiagnostic(ctx context.Context, url string) error {
	dir, err := filepath.Abs("secrets/gapgpt-diagnostics")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create GapGPT trace directory: %w", err)
	}
	file, err := os.CreateTemp(dir, "trace-*.log")
	if err != nil {
		return fmt.Errorf("create GapGPT trace file: %w", err)
	}
	fmt.Printf("GapGPT trace file: %s\n", file.Name())
	trace := bufio.NewWriter(file)
	runErr := app.DiagnoseGapGPT(ctx, url, trace, os.Stdout)
	if runErr != nil {
		fmt.Fprintln(trace, runErr)
	}
	if err := errors.Join(trace.Flush(), file.Close()); err != nil {
		return errors.Join(runErr, fmt.Errorf("save GapGPT trace: %w", err))
	}
	return runErr
}

// run starts the app, waits for SIGINT/SIGTERM (or an fx shutdown), then
// stops it gracefully: running extractions finish before the process
// exits (see server.StartHTTPServer). A second signal while draining
// exits at once without waiting.
func run(a interface {
	Start(context.Context) error
	Stop(context.Context) error
	Wait() <-chan fx.ShutdownSignal
	StartTimeout() time.Duration
	StopTimeout() time.Duration
}, log *zap.Logger) int {
	startCtx, cancel := context.WithTimeout(context.Background(), a.StartTimeout())
	defer cancel()
	if err := a.Start(startCtx); err != nil {
		log.Error("start failed", zap.Error(err))
		return 1
	}

	// fx relays SIGINT/SIGTERM here. Listen for a second signal only after
	// the first one arrived, so the first is not mistaken for the second.
	sig := <-a.Wait()
	log.Info("shutdown requested; finishing running requests (send the signal again to exit now)",
		zap.String("signal", sig.Signal.String()))
	force := make(chan os.Signal, 1)
	signal.Notify(force, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-force
		log.Warn("second signal: exiting without waiting for running requests", zap.String("signal", s.String()))
		_ = log.Sync()
		os.Exit(130)
	}()

	stopCtx, cancelStop := context.WithTimeout(context.Background(), a.StopTimeout())
	defer cancelStop()
	if err := a.Stop(stopCtx); err != nil {
		log.Error("shutdown did not finish cleanly", zap.Error(err))
		return 1
	}
	return sig.ExitCode
}
