package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
	"github.com/jobrunner/hostus/internal/adapters/telemetry"
	"github.com/jobrunner/hostus/internal/config"
)

// serveLogWriter is the second sink for the app logger's fan-out handler,
// alongside the always-on RingLog telemetry.Setup always installs. It's a
// var (not a hardcoded os.Stderr reference in New) so internal tests can
// redirect it to a buffer instead of the process's real stderr.
var serveLogWriter io.Writer = os.Stderr

// shutdownTimeoutSeconds bounds how long App.Serve waits for in-flight
// requests to drain and telemetry to flush once shutdown starts.
const shutdownTimeoutSeconds = 10

// shutdownTimeout is shutdownTimeoutSeconds converted to a time.Duration.
// It's a var (computed via a function body, not a folded const expression)
// for two reasons: go test's coverage instrumentation can't observe a
// compile-time-folded "N * time.Second" const initializer (see the
// identical note on httpx's defaultLoadShedBackoffSeconds), and internal
// tests can shrink it to force a fast, deterministic shutdown-deadline
// expiry without wall-clock sleeps.
var shutdownTimeout = newShutdownTimeout()

func newShutdownTimeout() time.Duration {
	return shutdownTimeoutSeconds * time.Second
}

// App is the hostus composition root. It wires telemetry, a logger backed
// by the telemetry ring log, and the HTTP router into a single reusable
// unit, so both the `serve` command (this task, HTTP transport) and the
// future `mcp` command (S10, stdio transport) can build the exact same
// stack via New and then read logs/spans off Telemetry without starting an
// HTTP listener at all.
type App struct {
	// Config is the fully loaded/validated configuration New was built from.
	Config *config.Config
	// Logger is backed by Telemetry.Log, so anything logged through it is
	// also visible to the debug MCP surface.
	Logger *slog.Logger
	// Telemetry exposes the trace/metric providers plus the always-on
	// in-memory span and log ring buffers.
	Telemetry *telemetry.Providers
	// Router is the fully assembled HTTP handler (middleware chain, health
	// probes, metrics endpoint).
	Router http.Handler

	shutdownTelemetry func(context.Context) error
	server            *http.Server
}

// New builds an App from cfg: telemetry providers and ring buffers via
// telemetry.Setup, a *slog.Logger backed by the telemetry ring log, and the
// HTTP router via httpx.NewRouter with Deps populated from cfg. It does not
// start listening; call Serve for that, or read Router/Telemetry/Logger
// directly (as the future mcp command does).
func New(cfg *config.Config) (*App, error) {
	providers, shutdownTelemetry, err := telemetry.Setup(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("setting up telemetry: %w", err)
	}

	// Fan out every log record to both the always-on RingLog (so the debug
	// MCP keeps seeing logs) and stderr (so `hostus serve` is not silent on
	// the terminal).
	logger := slog.New(telemetry.NewFanoutHandler(
		providers.Log,
		slog.NewTextHandler(serveLogWriter, nil),
	))

	router := httpx.NewRouter(httpx.Deps{
		Logger:             logger,
		CORSAllowedOrigins: cfg.CORS.AllowedOrigins,
	})

	return &App{
		Config:            cfg,
		Logger:            logger,
		Telemetry:         providers,
		Router:            router,
		shutdownTelemetry: shutdownTelemetry,
	}, nil
}

// Serve starts an HTTP server on Config.Server's host:port and blocks until
// ctx is done or the server fails to serve, then gracefully shuts down
// (bounded by shutdownTimeout) and flushes telemetry. A clean shutdown
// (ctx cancellation, or a graceful listener close) returns nil.
func (a *App) Serve(ctx context.Context) error {
	a.server = &http.Server{
		Addr:         a.Config.Server.Address(),
		Handler:      a.Router,
		ReadTimeout:  a.Config.Server.ReadTimeout,
		WriteTimeout: a.Config.Server.WriteTimeout,
	}

	a.Logger.Info("listening", "addr", a.server.Addr)

	serveErr := make(chan error, 1)
	go func() {
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		runErr = err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := a.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

// Shutdown gracefully stops the HTTP server (if Serve ever started one) and
// flushes/closes telemetry. It is exported so callers that never call
// Serve — the future mcp/stdio command chief among them — can still tear
// telemetry down cleanly via the same App.
func (a *App) Shutdown(ctx context.Context) error {
	var errs []error
	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutting down http server: %w", err))
		}
	}
	if a.shutdownTelemetry != nil {
		if err := a.shutdownTelemetry(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutting down telemetry: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Run builds an App for cfg and serves it until ctx is done. It is the
// entry point the `serve` command calls.
func Run(ctx context.Context, cfg *config.Config) error {
	a, err := New(cfg)
	if err != nil {
		return err
	}
	return a.Serve(ctx)
}
