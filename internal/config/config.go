package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port                    int           `mapstructure:"port"`
	HostName                string        `mapstructure:"host_name"`
	EnableTLS               bool          `mapstructure:"enable_tls"`
	CORSOrigins             []string      `mapstructure:"cors_origins"`
	RateLimit               int           `mapstructure:"rate_limit"`
	UpstreamErrorThreshold  int           `mapstructure:"upstream_error_threshold"`
	UpstreamBackoffSeconds  int           `mapstructure:"upstream_backoff_seconds"`
	CacheTTLSeconds         int           `mapstructure:"cache_ttl_seconds"`
	LogLevel                string        `mapstructure:"log_level"`
	GBIFBaseURL             string        `mapstructure:"gbif_base_url"`
	GBIFTimeoutSeconds      int           `mapstructure:"gbif_timeout_seconds"`
	RequestTimeoutSeconds   int           `mapstructure:"request_timeout_seconds"`
}

func (c *Config) CacheTTL() time.Duration {
	return time.Duration(c.CacheTTLSeconds) * time.Second
}

func (c *Config) UpstreamBackoff() time.Duration {
	return time.Duration(c.UpstreamBackoffSeconds) * time.Second
}

func (c *Config) GBIFTimeout() time.Duration {
	return time.Duration(c.GBIFTimeoutSeconds) * time.Second
}

func (c *Config) RequestTimeout() time.Duration {
	return time.Duration(c.RequestTimeoutSeconds) * time.Second
}

func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("port", 8080)
	v.SetDefault("host_name", "localhost")
	v.SetDefault("enable_tls", false)
	v.SetDefault("cors_origins", []string{"*"})
	v.SetDefault("rate_limit", 100)
	v.SetDefault("upstream_error_threshold", 5)
	v.SetDefault("upstream_backoff_seconds", 30)
	v.SetDefault("cache_ttl_seconds", 300)
	v.SetDefault("log_level", "info")
	v.SetDefault("gbif_base_url", "https://api.gbif.org/v1")
	v.SetDefault("gbif_timeout_seconds", 10)
	v.SetDefault("request_timeout_seconds", 30)

	// Environment variables
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	// .env file
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	_ = v.ReadInConfig() // Ignore error if .env doesn't exist

	// Bind environment variables explicitly
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func bindEnvVars(v *viper.Viper) {
	_ = v.BindEnv("port", "PORT")
	_ = v.BindEnv("host_name", "HOST_NAME")
	_ = v.BindEnv("enable_tls", "ENABLE_TLS")
	_ = v.BindEnv("cors_origins", "CORS_ORIGINS")
	_ = v.BindEnv("rate_limit", "RATE_LIMIT")
	_ = v.BindEnv("upstream_error_threshold", "UPSTREAM_ERROR_THRESHOLD")
	_ = v.BindEnv("upstream_backoff_seconds", "UPSTREAM_BACKOFF_SECONDS")
	_ = v.BindEnv("cache_ttl_seconds", "CACHE_TTL_SECONDS")
	_ = v.BindEnv("log_level", "LOG_LEVEL")
	_ = v.BindEnv("gbif_base_url", "GBIF_BASE_URL")
	_ = v.BindEnv("gbif_timeout_seconds", "GBIF_TIMEOUT_SECONDS")
	_ = v.BindEnv("request_timeout_seconds", "REQUEST_TIMEOUT_SECONDS")
}

func LoadWithFlags(flags map[string]interface{}) (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	// CLI flags override everything
	if v, ok := flags["port"].(int); ok && v != 0 {
		cfg.Port = v
	}
	if v, ok := flags["host-name"].(string); ok && v != "" {
		cfg.HostName = v
	}
	if v, ok := flags["enable-tls"].(bool); ok {
		cfg.EnableTLS = v
	}
	if v, ok := flags["cors-origins"].(string); ok && v != "" {
		cfg.CORSOrigins = strings.Split(v, ",")
	}
	if v, ok := flags["rate-limit"].(int); ok && v != 0 {
		cfg.RateLimit = v
	}
	if v, ok := flags["upstream-error-threshold"].(int); ok && v != 0 {
		cfg.UpstreamErrorThreshold = v
	}
	if v, ok := flags["upstream-backoff-seconds"].(int); ok && v != 0 {
		cfg.UpstreamBackoffSeconds = v
	}
	if v, ok := flags["cache-ttl-seconds"].(int); ok && v != 0 {
		cfg.CacheTTLSeconds = v
	}
	if v, ok := flags["log-level"].(string); ok && v != "" {
		cfg.LogLevel = v
	}

	return cfg, nil
}
