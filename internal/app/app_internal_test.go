package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/config"
)

// whiteboxTestConfig mirrors app_test.go's testConfig but lives here (same
// package) so these internal, field-poking tests don't need an exported
// constructor for it.
func whiteboxTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         0,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		Logging:   config.LoggingConfig{Level: "info", Format: "json"},
		Telemetry: config.TelemetryConfig{Enabled: false, SampleRatio: 1.0},
	}
}

// TestShutdownAggregatesTelemetryError pins Shutdown's error-aggregation
// branch (the `if err != nil` around shutdownTelemetry) deterministically,
// without depending on a real OTLP failure: it swaps in a stub that always
// errors and checks the aggregate error surfaces it.
func TestShutdownAggregatesTelemetryError(t *testing.T) {
	a, err := New(whiteboxTestConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sentinel := errors.New("boom")
	a.shutdownTelemetry = func(context.Context) error { return sentinel }

	if err := a.Shutdown(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want an error wrapping %v", err, sentinel)
	}
}

// TestShutdownSkipsNilTelemetryFunc pins the nil-check branch: a nil
// shutdownTelemetry (which New never actually produces, but Shutdown must
// stay defensive against) must not panic and must not fabricate an error.
func TestShutdownSkipsNilTelemetryFunc(t *testing.T) {
	a, err := New(whiteboxTestConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.shutdownTelemetry = nil

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}

// TestServeSurfacesShutdownErrorWhenRunErrWasNil pins Serve's
// "if err := a.Shutdown(...); err != nil && runErr == nil" branch: when
// the exit reason was ctx cancellation (runErr nil) but the subsequent
// Shutdown itself fails, Serve must report that Shutdown failure rather
// than silently returning nil. Forcing shutdownTelemetry to error is the
// deterministic way to make Shutdown fail without racing real network
// I/O (http.Server.Shutdown returns nil almost immediately when there are
// no in-flight connections, regardless of ctx deadline, so a short
// shutdownTimeout alone can't reliably force this branch).
func TestServeSurfacesShutdownErrorWhenRunErrWasNil(t *testing.T) {
	a, err := New(whiteboxTestConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sentinel := errors.New("boom")
	a.shutdownTelemetry = func(context.Context) error { return sentinel }

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done, so Serve's select picks ctx.Done() (runErr stays nil)

	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx) }()

	select {
	case err := <-done:
		if !errors.Is(err, sentinel) {
			t.Fatalf("got %v, want an error wrapping %v", err, sentinel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return in time")
	}
}

// TestRunPropagatesNewError pins Run's "if err != nil { return err }"
// branch right after New(cfg): when New fails, Run must surface that error
// without ever reaching Serve. It forces New to fail deterministically via
// a malformed OTLP endpoint (telemetry.Setup's otlpmetrichttp.New rejects
// it synchronously at construction — see the invalid-IP-literal case),
// rather than via any hard-to-reach OS-level failure.
func TestRunPropagatesNewError(t *testing.T) {
	cfg := whiteboxTestConfig()
	cfg.Telemetry = config.TelemetryConfig{
		Enabled:     true,
		Endpoint:    "http://[::1]:invalidport",
		SampleRatio: 1,
	}

	if err := Run(context.Background(), cfg); err == nil {
		t.Fatal("want New's error propagated, got nil")
	}
}

// TestNewLoggerFansOutToStderrWriterAndRingLog pins New's fan-out wiring:
// a.Logger must deliver every record to BOTH serveLogWriter (stderr in
// production, redirected here to a buffer so the test doesn't depend on the
// real process stderr) and the RingLog telemetry.Setup always installs
// (which the debug MCP reads from). Losing either sink is the exact defect
// this task fixes: `hostus serve` being silent on the terminal, or the MCP
// losing visibility into logs.
func TestNewLoggerFansOutToStderrWriterAndRingLog(t *testing.T) {
	var buf bytes.Buffer
	orig := serveLogWriter
	serveLogWriter = &buf
	defer func() { serveLogWriter = orig }()

	a, err := New(whiteboxTestConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	a.Logger.Info("probe-message")

	if !strings.Contains(buf.String(), "probe-message") {
		t.Fatalf("want stderr sink to receive the record, got %q", buf.String())
	}

	recs := a.Telemetry.Log.Records(slog.LevelInfo, 10)
	found := false
	for _, r := range recs {
		if r.Msg == "probe-message" {
			found = true
		}
	}
	if !found {
		t.Fatal("want RingLog to also receive the record (debug MCP visibility)")
	}
}

// TestNewShutdownTimeoutMatchesSeconds pins the arithmetic that converts
// shutdownTimeoutSeconds into a time.Duration, against a literal 10s (not
// the shared shutdownTimeoutSeconds constant) so a mutation of either the
// constant or the "* time.Second" multiplication is actually exercised
// instead of comparing the computation to itself.
func TestNewShutdownTimeoutMatchesSeconds(t *testing.T) {
	if got, want := newShutdownTimeout(), 10*time.Second; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}
