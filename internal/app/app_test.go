package app_test

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/config"
)

// testConfig builds a minimal, valid *config.Config for app tests without
// going through config.Load (which touches the global viper singleton and
// the filesystem/env). Port 0 lets the OS assign an ephemeral port.
func testConfig() *config.Config {
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

func TestNewBuildsAppWithHealthyRouter(t *testing.T) {
	a, err := app.New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Router == nil {
		t.Fatal("want non-nil Router")
	}
	if a.Logger == nil {
		t.Fatal("want non-nil Logger")
	}
	if a.Telemetry == nil {
		t.Fatal("want non-nil Telemetry")
	}

	rr := httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest("GET", "/health/live", nil))
	if rr.Code != 200 {
		t.Fatalf("got status %d, want 200", rr.Code)
	}
}

// TestServeShutsDownCleanlyOnContextCancel passes an already-canceled ctx
// rather than sleeping and canceling concurrently: http.Server.Shutdown is
// documented safe to call at any point relative to ListenAndServe (even
// before the listener has bound), so the deterministic already-done ctx
// exercises the same ctx.Done() branch without a wall-clock race — and
// without a long fallback timeout that would make a broken select hang the
// whole test (and, under mutation testing, the whole suite run) for
// seconds instead of failing fast.
func TestServeShutsDownCleanlyOnContextCancel(t *testing.T) {
	a, err := app.New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want clean shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not shut down in time")
	}
}

// TestServeReturnsErrorOnBindFailure pins the ListenAndServe error branch
// inside Serve's background goroutine: a real (non-ErrServerClosed) listen
// failure must surface as Serve's return value, not be swallowed. It forces
// that failure deterministically by holding the exact port App tries to
// bind, rather than by mutating config into some other invalid state.
func TestServeReturnsErrorOnBindFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	defer func() { _ = l.Close() }()
	port := l.Addr().(*net.TCPAddr).Port

	cfg := testConfig()
	cfg.Server.Port = port

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- a.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want a bind error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not report the bind failure in time")
	}
}

// TestShutdownWithoutServeIsSafe confirms Shutdown can be called on an App
// that never had Serve invoked on it (the future mcp/stdio command's use
// case: it needs to flush telemetry without ever starting an HTTP
// listener).
func TestShutdownWithoutServeIsSafe(t *testing.T) {
	a, err := app.New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown without Serve: %v", err)
	}
}

// TestRunServesUntilContextCancelled exercises the Run(ctx, cfg) entry
// point serve calls: New + Serve wired together end to end, with the same
// already-canceled-ctx approach as TestServeShutsDownCleanlyOnContextCancel
// (see its comment for why).
func TestRunServesUntilContextCancelled(t *testing.T) {
	cfg := testConfig()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, cfg) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want clean shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not shut down in time")
	}
}
