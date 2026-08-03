package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPrefersEnvOverDefault(t *testing.T) {
	t.Setenv("HOSTUS_SERVER_PORT", "8443")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8443 {
		t.Fatalf("got %d, want 8443", cfg.Server.Port)
	}
}

func TestLoadAppliesDefaultsWhenEnvUnset(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != defaultServerPort {
		t.Fatalf("got %d, want default %d", cfg.Server.Port, defaultServerPort)
	}
	if cfg.Server.Host != defaultServerHost {
		t.Fatalf("got host %q, want default %q", cfg.Server.Host, defaultServerHost)
	}
	if cfg.Logging.Level != defaultLoggingLevel {
		t.Fatalf("got logging.level %q, want default %q", cfg.Logging.Level, defaultLoggingLevel)
	}
	if cfg.Logging.Format != defaultLoggingFormat {
		t.Fatalf("got logging.format %q, want default %q", cfg.Logging.Format, defaultLoggingFormat)
	}
	if !cfg.Metrics.Enabled {
		t.Fatal("want metrics.enabled default true")
	}
	if cfg.Metrics.Path != defaultMetricsPath {
		t.Fatalf("got metrics.path %q, want default %q", cfg.Metrics.Path, defaultMetricsPath)
	}
	if cfg.TLS.Enabled {
		t.Fatal("want tls.enabled default false")
	}
	if cfg.Telemetry.Enabled {
		t.Fatal("want telemetry.enabled default false")
	}
	if cfg.Telemetry.SampleRatio != defaultTelemetrySampleRatio {
		t.Fatalf("got telemetry.sample_ratio %v, want default %v", cfg.Telemetry.SampleRatio, defaultTelemetrySampleRatio)
	}
	if cfg.SQLite.Path != defaultSQLitePath {
		t.Fatalf("got sqlite.path %q, want default %q", cfg.SQLite.Path, defaultSQLitePath)
	}
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Fatalf("want empty cors.allowed_origins default, got %v", cfg.CORS.AllowedOrigins)
	}
	// Pinned against a literal duration (not a shared constant) so a
	// mutation of the multiplication in Defaults() (e.g. "* time.Second" ->
	// "/ time.Second") cannot hide behind comparing a value to itself.
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Fatalf("got server.read_timeout %v, want 30s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Fatalf("got server.write_timeout %v, want 30s", cfg.Server.WriteTimeout)
	}
}

// TestValidateServerPortBoundaries pins the exact port range edges (1 and
// 65535 valid; 0 and 65536 invalid) so boundary mutations of validateServer's
// comparisons are caught.
func TestValidateServerPortBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{"zero is invalid", "0", true},
		{"one is valid", "1", false},
		{"max valid boundary", "65535", false},
		{"above max is invalid", "65536", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOSTUS_SERVER_PORT", tt.port)
			_, err := Load("")
			if tt.wantErr && err == nil {
				t.Fatalf("port %s: want error, got nil", tt.port)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("port %s: want no error, got %v", tt.port, err)
			}
		})
	}
}

// TestLoadFindsConfigFileInSearchPath exercises the implicit (configPath=="")
// branch of Load where a config.yaml actually exists in the current
// directory, distinguishing a successful search-path read from the
// "not found" case.
func TestLoadFindsConfigFileInSearchPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := writeFile(path, "server:\n  port: 9123\n"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("want no error reading config.yaml from search path, got %v", err)
	}
	if cfg.Server.Port != 9123 {
		t.Fatalf("got %d, want 9123 from search-path config file", cfg.Server.Port)
	}
}

// TestLoadTLSValidation exercises Validate's TLS branch (only reachable via
// Load when tls.enabled is true) across the domains/email requirement pairs.
func TestLoadTLSValidation(t *testing.T) {
	tests := []struct {
		name      string
		enabled   string
		domains   string
		email     string
		wantErr   bool
		wantMatch string
	}{
		{"disabled needs nothing", "false", "", "", false, ""},
		{"enabled without domains fails", "true", "", "someone@example.test", true, "domains"},
		{"enabled without email fails", "true", "example.test", "", true, "email"},
		{"enabled with both succeeds", "true", "example.test", "someone@example.test", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOSTUS_TLS_ENABLED", tt.enabled)
			if tt.domains != "" {
				t.Setenv("HOSTUS_TLS_DOMAINS", tt.domains)
			}
			t.Setenv("HOSTUS_TLS_EMAIL", tt.email)

			_, err := Load("")
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantMatch) {
					t.Fatalf("error %q does not mention %q", err.Error(), tt.wantMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
		})
	}
}

// TestLoadTelemetryValidation exercises Validate's telemetry branch: the
// endpoint requirement when enabled, and the sample_ratio range including
// its exact boundaries (0 and 1 valid; just outside, invalid).
func TestLoadTelemetryValidation(t *testing.T) {
	tests := []struct {
		name        string
		enabled     string
		endpoint    string
		sampleRatio string
		wantErr     bool
	}{
		{"disabled, no endpoint needed", "false", "", "1.0", false},
		{"enabled with endpoint succeeds", "true", "collector:4317", "1.0", false},
		{"enabled without endpoint fails", "true", "", "1.0", true},
		{"sample ratio zero is valid", "false", "", "0", false},
		{"sample ratio one is valid", "false", "", "1", false},
		{"sample ratio just below zero fails", "false", "", "-0.01", true},
		{"sample ratio just above one fails", "false", "", "1.01", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOSTUS_TELEMETRY_ENABLED", tt.enabled)
			t.Setenv("HOSTUS_TELEMETRY_ENDPOINT", tt.endpoint)
			t.Setenv("HOSTUS_TELEMETRY_SAMPLE_RATIO", tt.sampleRatio)

			_, err := Load("")
			if tt.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("want no error, got %v", err)
			}
		})
	}
}

func TestLoadReadsConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := "server:\n  port: 9999\n  host: \"198.51.100.1\"\ncors:\n  allowed_origins:\n    - \"https://example.test\"\n"
	if err := writeFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9999 {
		t.Fatalf("got %d, want 9999 from config file", cfg.Server.Port)
	}
	if cfg.Server.Host != "198.51.100.1" {
		t.Fatalf("got host %q, want %q from config file", cfg.Server.Host, "198.51.100.1")
	}
	if len(cfg.CORS.AllowedOrigins) != 1 || cfg.CORS.AllowedOrigins[0] != "https://example.test" {
		t.Fatalf("got cors.allowed_origins %v, want [https://example.test]", cfg.CORS.AllowedOrigins)
	}
}

func TestLoadEnvOverridesConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := "server:\n  port: 9999\n"
	if err := writeFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOSTUS_SERVER_PORT", "7000")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 7000 {
		t.Fatalf("got %d, want env override 7000", cfg.Server.Port)
	}
}

func TestLoadMissingExplicitConfigFileFails(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("want error for missing explicit config file, got nil")
	}
}

func TestLoadInvalidPortFails(t *testing.T) {
	t.Setenv("HOSTUS_SERVER_PORT", "0")
	_, err := Load("")
	if err == nil {
		t.Fatal("want error for invalid server.port, got nil")
	}
}

func TestServerConfigAddress(t *testing.T) {
	s := ServerConfig{Host: "127.0.0.1", Port: 8080}
	if got, want := s.Address(), "127.0.0.1:8080"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
