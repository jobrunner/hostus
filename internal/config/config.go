// Package config loads hostus configuration via Viper: defaults, an
// optional YAML file, and HOSTUS_-prefixed environment variables (highest
// precedence of the two; CLI flag binding is wired in a later task).
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Default values, also asserted against in tests so config.go and
// config_test.go cannot silently drift apart.
const (
	defaultServerHost           = "0.0.0.0"
	defaultServerPort           = 8080
	defaultServerTimeoutSeconds = 30
	defaultLoggingLevel         = "info"
	defaultLoggingFormat        = "json"
	defaultMetricsEnabled       = true
	defaultMetricsPath          = "/metrics"
	defaultTLSEnabled           = false
	defaultTelemetryEnabled     = false
	defaultTelemetrySampleRatio = 1.0
	defaultSQLitePath           = "./data/hostus.db"
	defaultSQLiteMaxReadConns   = 4
	defaultUIEnabled            = true
)

// Config holds all application configuration for hostus 2.0.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	TLS       TLSConfig       `mapstructure:"tls"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	SQLite    SQLiteConfig    `mapstructure:"sqlite"`
	CORS      CORSConfig      `mapstructure:"cors"`
	UI        UIConfig        `mapstructure:"ui"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// Address returns the server's listen address as "host:port".
func (s *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json, text
}

// MetricsConfig holds Prometheus metrics endpoint configuration.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// TLSConfig holds TLS/CertMagic configuration.
type TLSConfig struct {
	Enabled bool     `mapstructure:"enabled"`
	Domains []string `mapstructure:"domains"`
	Email   string   `mapstructure:"email"`
}

// TelemetryConfig holds OpenTelemetry export configuration.
type TelemetryConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	Endpoint    string  `mapstructure:"endpoint"`
	SampleRatio float64 `mapstructure:"sample_ratio"`
}

// SQLiteConfig holds the on-disk cache database location and the serve
// path's read-connection pool size.
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
	// MaxReadConns bounds the connection pool `hostus serve` opens for its
	// read-only path (see sqlite.OpenPool). Ingest/bundle/export always use
	// a single connection regardless of this value — see sqlite.Open.
	MaxReadConns int `mapstructure:"max_read_conns"`
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// UIConfig holds the embedded test console's toggle. Enabled by default:
// the console is the surface the operator judges the system by, so it ships
// on and has to be switched off deliberately (env HOSTUS_UI_ENABLED, or
// `serve --ui=false`).
type UIConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// Defaults sets viper's default configuration values.
func Defaults() {
	// The multiplication runs here (inside a function body covered by the
	// test suite) rather than in a package-level const initializer, so a
	// mutation of the arithmetic is actually exercised by the tests.
	serverTimeout := defaultServerTimeoutSeconds * time.Second

	viper.SetDefault("server.host", defaultServerHost)
	viper.SetDefault("server.port", defaultServerPort)
	viper.SetDefault("server.read_timeout", serverTimeout)
	viper.SetDefault("server.write_timeout", serverTimeout)

	viper.SetDefault("logging.level", defaultLoggingLevel)
	viper.SetDefault("logging.format", defaultLoggingFormat)

	viper.SetDefault("metrics.enabled", defaultMetricsEnabled)
	viper.SetDefault("metrics.path", defaultMetricsPath)

	viper.SetDefault("tls.enabled", defaultTLSEnabled)
	viper.SetDefault("tls.domains", []string{})
	viper.SetDefault("tls.email", "")

	viper.SetDefault("telemetry.enabled", defaultTelemetryEnabled)
	viper.SetDefault("telemetry.endpoint", "")
	viper.SetDefault("telemetry.sample_ratio", defaultTelemetrySampleRatio)

	viper.SetDefault("sqlite.path", defaultSQLitePath)
	viper.SetDefault("sqlite.max_read_conns", defaultSQLiteMaxReadConns)

	viper.SetDefault("cors.allowed_origins", []string{})

	viper.SetDefault("ui.enabled", defaultUIEnabled)
}

// Load loads configuration from defaults, an optional config file, and
// HOSTUS_-prefixed environment variables (env overrides file, file overrides
// defaults). If configPath is non-empty it is read as an explicit file and
// any error (including "not found") is returned; otherwise "config.yaml" is
// searched for in ".", "./config" and "/etc/hostus" and its absence is not
// an error.
func Load(configPath string) (*Config, error) {
	viper.Reset()
	Defaults()

	viper.SetEnvPrefix("HOSTUS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if configPath != "" {
		viper.SetConfigFile(configPath)
		if err := viper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file %q: %w", configPath, err)
		}
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("/etc/hostus")

		if err := viper.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return nil, fmt.Errorf("reading config file: %w", err)
			}
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// Validate checks the loaded configuration for internal consistency.
func (c *Config) Validate() error {
	if err := c.validateServer(); err != nil {
		return err
	}
	if err := c.validateTLS(); err != nil {
		return err
	}
	return c.validateTelemetry()
}

func (c *Config) validateServer() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d", c.Server.Port)
	}
	return nil
}

func (c *Config) validateTLS() error {
	if !c.TLS.Enabled {
		return nil
	}
	if len(c.TLS.Domains) == 0 {
		return fmt.Errorf("tls.enabled is true but tls.domains is empty")
	}
	if c.TLS.Email == "" {
		return fmt.Errorf("tls.enabled is true but tls.email is empty")
	}
	return nil
}

func (c *Config) validateTelemetry() error {
	if c.Telemetry.SampleRatio < 0 || c.Telemetry.SampleRatio > 1 {
		return fmt.Errorf("telemetry.sample_ratio must be in [0, 1], got %f", c.Telemetry.SampleRatio)
	}
	if c.Telemetry.Enabled && c.Telemetry.Endpoint == "" {
		return fmt.Errorf("telemetry.enabled is true but telemetry.endpoint is empty")
	}
	return nil
}
