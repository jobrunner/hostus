// Package httpx assembles the HTTP-facing adapter: the middleware chain,
// health probes and the Prometheus metrics endpoint. It is named httpx
// (rather than http) to avoid shadowing the stdlib net/http package at
// import sites.
package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"

	"github.com/jobrunner/hostus/internal/middleware"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// Fallback defaults applied when the corresponding Deps field is left at
// its zero value, so a zero-value Deps{} (as used by tests and any caller
// that hasn't wired real configuration yet) never panics or produces a
// pathological router (e.g. a rate limiter that admits zero requests).
const (
	defaultRateLimitPerSecond = 20
	defaultLoadShedThreshold  = 1000

	// defaultLoadShedBackoffSeconds and defaultTimeoutSeconds are kept as
	// plain int constants (rather than pre-computed time.Duration values)
	// so the `* time.Second` conversion happens as a statement inside
	// NewRouter, where go test's coverage instrumentation — and therefore
	// gremlins' mutation coverage — can actually observe it. A `N *
	// time.Second` const expression is folded by the compiler and never
	// shows up as covered/uncovered in runtime coverage data.
	defaultLoadShedBackoffSeconds = 5
	defaultTimeoutSeconds         = 30
)

// Deps carries everything NewRouter needs to assemble the middleware chain.
// Every field is optional: a nil/zero field falls back to a sensible
// default so httpx.NewRouter(httpx.Deps{}) is safe to call.
type Deps struct {
	Logger *slog.Logger

	// RateLimitPerSecond configures the token-bucket rate limiter. <= 0
	// falls back to defaultRateLimitPerSecond.
	RateLimitPerSecond int

	// LoadShedThreshold is the number of consecutive upstream errors that
	// trips load shedding. <= 0 falls back to defaultLoadShedThreshold.
	LoadShedThreshold int
	// LoadShedBackoff is how long shedding stays active before a probe
	// request is allowed through. <= 0 falls back to defaultLoadShedBackoff.
	LoadShedBackoff time.Duration

	// Timeout bounds request context lifetime. <= 0 falls back to
	// defaultTimeout.
	Timeout time.Duration

	// CORSAllowedOrigins lists origins allowed to call the API. Empty
	// falls back to permissive "*" (safe here: hostus serves a read-only,
	// public taxonomy/trait index with no auth or user data, so a
	// permissive default doesn't expose anything sensitive).
	CORSAllowedOrigins []string

	// Repo backs /v1/concept, /v1/xref and /v1/match. A nil Repo means
	// those routes are not mounted (only health/metrics are), so a
	// zero-value Deps{} (as used by the existing middleware/health tests)
	// stays safe to serve.
	Repo output.Repository
}

// NewRouter assembles the hostus HTTP surface: the fixed middleware chain
// (Request-ID -> Logging -> Rate-Limiting -> Load-Shedding -> Timeouts ->
// CORS -> Metrics), wrapped in an outermost otelmux span, plus the health
// and metrics endpoints. The middleware order is an immutable global
// constraint (see CLAUDE.md) and must not be reordered.
func NewRouter(deps Deps) *mux.Router {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	rps := deps.RateLimitPerSecond
	if rps <= 0 {
		rps = defaultRateLimitPerSecond
	}
	limiter := middleware.NewRateLimiter(rps)

	threshold := deps.LoadShedThreshold
	if threshold <= 0 {
		threshold = defaultLoadShedThreshold
	}
	backoff := deps.LoadShedBackoff
	if backoff <= 0 {
		backoff = defaultLoadShedBackoffSeconds * time.Second
	}
	shedder := middleware.NewLoadShedder(threshold, backoff)

	timeout := deps.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds * time.Second
	}

	origins := deps.CORSAllowedOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	r := mux.NewRouter()

	// otelmux wraps the ENTIRE chain (it must be registered first: gorilla
	// mux applies Use()-registered middleware in registration order, so
	// the first one added is the outermost/first-to-run).
	r.Use(otelmux.Middleware("hostus"))

	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(logger))
	r.Use(middleware.RateLimit(limiter))
	r.Use(middleware.LoadShed(shedder))
	r.Use(middleware.Timeout(timeout))
	r.Use(middleware.CORS(origins))
	r.Use(middleware.Metrics)

	r.HandleFunc("/health/live", handleHealthLive).Methods(http.MethodGet)
	r.HandleFunc("/health/ready", handleHealthReady(deps.Repo)).Methods(http.MethodGet)
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	if deps.Repo != nil {
		r.HandleFunc("/v1/concept/{id}", handleConcept(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/xref", handleXref(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/match", handleMatch(deps.Repo)).Methods(http.MethodPost)
		r.HandleFunc("/v1/suggest", handleSuggest(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/concept/{id}/traits", handleTraits(deps.Repo)).Methods(http.MethodGet)
	}

	return r
}
