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

	// UIEnabled mounts the embedded test console at "/". False registers
	// nothing at all, so "/" and every asset path below it are 404 — the
	// zero value therefore keeps the router API-only. "Default on" is a
	// configuration decision (config.Defaults sets ui.enabled=true) and
	// deliberately not a router-level fallback: a zero-value Deps must
	// never expose a surface its caller did not ask for.
	UIEnabled bool

	// Version is the running build's version, rendered in the console's
	// footer so a tester can tell two deployments apart from the page
	// itself. Empty renders as "dev", matching `hostus version`'s
	// placeholder for an unstamped build. It is display-only: no route
	// behavior depends on it.
	Version string
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

	// The fixed chain, built as a slice rather than seven r.Use calls: it
	// has to be applied twice. gorilla/mux runs Use()-registered middleware
	// only for MATCHED routes — Router.Match assigns NotFoundHandler
	// without wrapping it — so the SPA fallback below has to be wrapped by
	// hand from the very same slice. One source of truth, no drift.
	//
	// otelmux is first because it must wrap the ENTIRE chain: mux applies
	// middleware in registration order, so the first one added is the
	// outermost/first-to-run.
	chain := []mux.MiddlewareFunc{
		otelmux.Middleware("hostus"),
		middleware.RequestID,
		middleware.Logging(logger),
		middleware.RateLimit(limiter),
		middleware.LoadShed(shedder),
		middleware.Timeout(timeout),
		middleware.CORS(origins),
		middleware.Metrics,
	}
	for _, mw := range chain {
		r.Use(mw)
	}

	r.HandleFunc("/health/live", handleHealthLive).Methods(http.MethodGet)
	r.HandleFunc("/health/ready", handleHealthReady(deps.Repo)).Methods(http.MethodGet)
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	if deps.Repo != nil {
		r.HandleFunc("/v1/concept/{id}", handleConcept(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/xref", handleXref(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/match", handleMatch(deps.Repo)).Methods(http.MethodPost)
		r.HandleFunc("/v1/suggest", handleSuggest(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/concept/{id}/traits", handleTraits(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/concept/{id}/synonyms", handleSynonyms(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/translate", handleTranslate(deps.Repo)).Methods(http.MethodPost)
		r.HandleFunc("/v1/sec", handleSec(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/areas", handleAreas(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/backbones", handleBackbones(deps.Repo)).Methods(http.MethodGet)
		r.HandleFunc("/v1/spaces", handleSpaces(deps.Repo)).Methods(http.MethodGet)
	}

	// Registered last and inside the same middleware chain as everything
	// else: the console must be observable (request id, logs, spans,
	// metrics) and shed/limited exactly like the API it drives. Registering
	// it after the API routes also means the UI can never shadow a /v1,
	// /health or /metrics path.
	if deps.UIEnabled {
		// Built once here: the document embeds deps.Version, and both the "/"
		// route and the SPA fallback must serve the very same bytes and ETag.
		ui := handleUI(deps.Version)
		r.HandleFunc("/", ui).Methods(http.MethodGet, http.MethodHead)
		r.HandleFunc("/assets/{name}", handleUIAsset).Methods(http.MethodGet, http.MethodHead)

		// SPA deep links. NotFoundHandler is the only hook that fires
		// AFTER every route has been tried, which is what keeps a 405 a
		// 405 and an unknown /v1 path a 404; a catch-all PathPrefix("/")
		// route would swallow both. It is wrapped in the same chain by
		// hand because mux does not wrap it (see `chain` above).
		r.NotFoundHandler = applyChain(chain, spaFallback(ui))
	}

	return r
}

// applyChain wraps h in mws so that mws[0] is outermost, matching the
// order gorilla/mux itself applies Use()-registered middleware in.
func applyChain(mws []mux.MiddlewareFunc, h http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i].Middleware(h)
	}
	return h
}
