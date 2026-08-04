package main

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/config"
)

// freePort asks the OS for an ephemeral TCP port, then releases it. There
// is an inherent (tiny) race between releasing and the caller rebinding it,
// but it's the standard trick for "give me a free port" in tests and this
// codebase already reaches for it (see internal/adapters/http tests).
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not reserve a free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// TestBindServeFlagsOverridesConfig pins the flag->viper->config wiring: a
// --port the user actually passed must win over config.Load's defaults.
func TestBindServeFlagsOverridesConfig(t *testing.T) {
	cmd := newServeCmd()
	if err := cmd.Flags().Set("port", "9999"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("host", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := bindServeFlags(cmd, cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != 9999 {
		t.Fatalf("got port %d, want 9999", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("got host %q, want 127.0.0.1", cfg.Server.Host)
	}
}

// TestBindServeFlagsLeavesConfigDefaultsAloneWhenUnset ensures an untouched
// flag does not clobber the config default with the flag's own default
// (both happen to be 0.0.0.0:8080 here, but the point is the *mechanism*:
// unset flags must not take precedence over config.Load's resolution).
func TestBindServeFlagsLeavesConfigDefaultsAloneWhenUnset(t *testing.T) {
	cmd := newServeCmd()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := bindServeFlags(cmd, cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != 8080 {
		t.Fatalf("got port %d, want unchanged default 8080", cfg.Server.Port)
	}
}

// TestBindServeFlagsRejectsInvalidPort confirms a bad flag value is caught
// by cfg.Validate() rather than silently accepted.
func TestBindServeFlagsRejectsInvalidPort(t *testing.T) {
	cmd := newServeCmd()
	if err := cmd.Flags().Set("port", "99999"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := bindServeFlags(cmd, cfg); err == nil {
		t.Fatal("want error for out-of-range port, got nil")
	}
}

// TestUIFlagBeatsEnv pins the top rung of the configuration ladder for
// SP8's ui.enabled: a --ui the user actually passed must override
// HOSTUS_UI_ENABLED, in both directions. The "off" direction is the one
// that matters most — a bool flag wired so it can only ever turn the UI
// *on* would leave the top tier unable to disable anything.
func TestUIFlagBeatsEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		flag string
		want bool
	}{
		{"flag off beats env on", "true", "false", false},
		{"flag on beats env off", "false", "true", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOSTUS_UI_ENABLED", tt.env)

			cmd := newServeCmd()
			if err := cmd.Flags().Set("ui", tt.flag); err != nil {
				t.Fatal(err)
			}

			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			if err := bindServeFlags(cmd, cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.UI.Enabled != tt.want {
				t.Fatalf("got ui.enabled %v, want %v (env=%s, --ui=%s)", cfg.UI.Enabled, tt.want, tt.env, tt.flag)
			}
		})
	}
}

// TestUIFlagUnsetLeavesEnvAlone is the counterpart: the flag's own default
// (true) must not clobber an explicit HOSTUS_UI_ENABLED=false when the user
// never passed --ui.
func TestUIFlagUnsetLeavesEnvAlone(t *testing.T) {
	t.Setenv("HOSTUS_UI_ENABLED", "false")

	cmd := newServeCmd()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := bindServeFlags(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Enabled {
		t.Fatal("want ui.enabled false (env), got true — the untouched --ui default clobbered it")
	}
}

// TestUIFlagDefaultsToOn pins the default with neither flag nor env set.
func TestUIFlagDefaultsToOn(t *testing.T) {
	cmd := newServeCmd()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := bindServeFlags(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.UI.Enabled {
		t.Fatal("want ui.enabled default true")
	}
}

// TestRootDefaultsToServe confirms the root command with no subcommand
// behaves like "serve": it starts the HTTP server and shuts down cleanly
// when ctx is canceled, rather than e.g. doing nothing or erroring.
func TestRootDefaultsToServe(t *testing.T) {
	port := freePort(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--host=127.0.0.1", "--port=" + strconv.Itoa(port)})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()

	time.Sleep(100 * time.Millisecond) // let the listener come up
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want clean shutdown, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("root command did not shut down in time")
	}
}

// TestRootRejectsBadPort confirms a validation failure (not a panic, not a
// silent success) surfaces when serve is invoked with an out-of-range port.
func TestRootRejectsBadPort(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--port=99999"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("want error for out-of-range port, got nil")
	}
}
