package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jobrunner/hostus/internal/api"
	"github.com/jobrunner/hostus/internal/cache"
	"github.com/jobrunner/hostus/internal/config"
	"github.com/jobrunner/hostus/internal/gbif"
	"github.com/jobrunner/hostus/internal/middleware"
)

var version = "dev"

func main() {
	flags := parseFlags()

	cfg, err := config.LoadWithFlags(flags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("starting hostus",
		slog.String("version", version),
		slog.Int("port", cfg.Port),
		slog.String("log_level", cfg.LogLevel),
	)

	gbifClient := gbif.NewClient(cfg.GBIFBaseURL, cfg.GBIFTimeout())
	taxaCache := cache.New(cfg.CacheTTL())
	loadShedder := middleware.NewLoadShedder(cfg.UpstreamErrorThreshold, cfg.UpstreamBackoff())
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit)

	handler := api.NewHandler(api.HandlerConfig{
		GBIFClient:  gbifClient,
		Cache:       taxaCache,
		LoadShedder: loadShedder,
		Logger:      logger,
	})

	router := api.NewRouter(handler)

	// Apply middleware chain (order matters!)
	var h http.Handler = router
	h = middleware.Metrics(h)
	h = middleware.CORS(cfg.CORSOrigins)(h)
	h = middleware.Timeout(cfg.RequestTimeout())(h)
	h = middleware.LoadShed(loadShedder)(h)
	h = middleware.RateLimit(rateLimiter)(h)
	h = middleware.Logging(logger)(h)
	h = middleware.RequestID(h)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: cfg.RequestTimeout() + 5*time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("server listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-done
	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", slog.String("error", err.Error()))
	}

	logger.Info("server stopped")
}

func parseFlags() map[string]any {
	flags := make(map[string]any)

	port := flag.Int("port", 0, "Server port")
	hostName := flag.String("host-name", "", "Host name")
	enableTLS := flag.Bool("enable-tls", false, "Enable TLS")
	corsOrigins := flag.String("cors-origins", "", "CORS allowed origins (comma-separated)")
	rateLimit := flag.Int("rate-limit", 0, "Rate limit (requests per second)")
	upstreamErrorThreshold := flag.Int("upstream-error-threshold", 0, "Upstream error threshold for load shedding")
	upstreamBackoffSeconds := flag.Int("upstream-backoff-seconds", 0, "Upstream backoff duration in seconds")
	cacheTTLSeconds := flag.Int("cache-ttl-seconds", 0, "Cache TTL in seconds")
	logLevel := flag.String("log-level", "", "Log level (debug, info, warn, error)")

	flag.Parse()

	flags["port"] = *port
	flags["host-name"] = *hostName
	flags["enable-tls"] = *enableTLS
	flags["cors-origins"] = *corsOrigins
	flags["rate-limit"] = *rateLimit
	flags["upstream-error-threshold"] = *upstreamErrorThreshold
	flags["upstream-backoff-seconds"] = *upstreamBackoffSeconds
	flags["cache-ttl-seconds"] = *cacheTTLSeconds
	flags["log-level"] = *logLevel

	return flags
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
}
