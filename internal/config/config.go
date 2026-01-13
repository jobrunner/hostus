package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port                   int      `mapstructure:"port"`
	HostName               string   `mapstructure:"host_name"`
	EnableTLS              bool     `mapstructure:"enable_tls"`
	CORSOrigins            []string `mapstructure:"cors_origins"`
	RateLimit              int      `mapstructure:"rate_limit"`
	UpstreamErrorThreshold int      `mapstructure:"upstream_error_threshold"`
	UpstreamBackoffSeconds int      `mapstructure:"upstream_backoff_seconds"`
	CacheTTLSeconds        int      `mapstructure:"cache_ttl_seconds"`
	LogLevel               string   `mapstructure:"log_level"`
	GBIFBaseURL            string   `mapstructure:"gbif_base_url"`
	GBIFTimeoutSeconds     int      `mapstructure:"gbif_timeout_seconds"`
	RequestTimeoutSeconds  int      `mapstructure:"request_timeout_seconds"`
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

	setDefaults(v)

	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	_ = v.ReadInConfig()

	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
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

func LoadWithFlags(flags map[string]any) (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	applyFlags(cfg, flags)
	return cfg, nil
}

func applyFlags(cfg *Config, flags map[string]any) {
	applyIntFlag(flags, "port", &cfg.Port)
	applyStringFlag(flags, "host-name", &cfg.HostName)
	applyBoolFlag(flags, "enable-tls", &cfg.EnableTLS)
	applyCORSFlag(flags, "cors-origins", &cfg.CORSOrigins)
	applyIntFlag(flags, "rate-limit", &cfg.RateLimit)
	applyIntFlag(flags, "upstream-error-threshold", &cfg.UpstreamErrorThreshold)
	applyIntFlag(flags, "upstream-backoff-seconds", &cfg.UpstreamBackoffSeconds)
	applyIntFlag(flags, "cache-ttl-seconds", &cfg.CacheTTLSeconds)
	applyStringFlag(flags, "log-level", &cfg.LogLevel)
}

func applyIntFlag(flags map[string]any, key string, target *int) {
	if v, ok := flags[key].(int); ok && v != 0 {
		*target = v
	}
}

func applyStringFlag(flags map[string]any, key string, target *string) {
	if v, ok := flags[key].(string); ok && v != "" {
		*target = v
	}
}

func applyBoolFlag(flags map[string]any, key string, target *bool) {
	if v, ok := flags[key].(bool); ok {
		*target = v
	}
}

func applyCORSFlag(flags map[string]any, key string, target *[]string) {
	if v, ok := flags[key].(string); ok && v != "" {
		*target = strings.Split(v, ",")
	}
}
