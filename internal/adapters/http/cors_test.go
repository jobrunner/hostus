package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// stubCORSRepo is a do-nothing Repository. The /v1 routes — and therefore
// the POST-only /v1/match this whole fix is about — are only registered when
// Deps.Repo is non-nil, so a zero-value Deps{} would give a router that has
// no POST route at all and could not show the bug. None of the CORS tests
// ever reach a handler (a preflight is short-circuited, /health/live needs no
// repo), so the embedded nil interface is never called.
type stubCORSRepo struct {
	output.Repository
}

// TestCORS_PreflightOnPostRouteIsAnswered pins the regression that made
// /v1/match unusable from a browser: gorilla/mux runs Use-registered
// middleware only for MATCHED routes, so an OPTIONS preflight against a
// route registered as .Methods(POST) matched nothing, fell through to the
// MethodNotAllowedHandler outside the chain, and came back as a bare 405
// with no CORS headers at all. Measured on v3.4.0-alpha.0.
func TestCORS_PreflightOnPostRouteIsAnswered(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Fatal("preflight carries no Access-Control-Allow-Origin")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPost) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to contain POST", got)
	}
}

// TestCORS_BareOptionsStillReachesRouter guards the "enabling CORS turns
// every OPTIONS into a 204" failure mode: without Origin and
// Access-Control-Request-Method this is not a preflight, so the router's
// own 405 must survive.
func TestCORS_BareOptionsStillReachesRouter(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		t.Fatal("bare OPTIONS was answered as a preflight")
	}
}

// TestCORS_OptionsWithOriginButNoRequestMethodIsNotAPreflight pins the
// second half of the same guard: Origin alone does not make a preflight
// (Access-Control-Request-Method is mandatory in one), so the router's 405
// must still win — otherwise any cross-origin OPTIONS probe would be
// answered 204 for a path the service does not serve under that verb.
func TestCORS_OptionsWithOriginButNoRequestMethodIsNotAPreflight(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 — OPTIONS without Access-Control-Request-Method is not a preflight", rec.Code)
	}
}

// TestCORS_NonOptionsRequestReachesRouter guards the mirror-image failure:
// short-circuiting on the CORS headers alone would swallow the real request.
// A cross-origin GET carrying an Access-Control-Request-Method header (which
// a hand-rolled client may well send) must still be served by the router.
func TestCORS_NonOptionsRequestReachesRouter(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a non-OPTIONS request must reach the router", rec.Code)
	}
}

// TestCORS_UnservedMethodIsNotAdvertised: a method the router does not
// serve for this path must never appear in Allow-Methods, or a browser is
// told to try a request that can only 405.
func TestCORS_UnservedMethodIsNotAdvertised(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want empty for an unserved method", got)
	}
	// The PATH exists (mux reports ErrMethodMismatch), so this is still a
	// preflight worth answering — the browser learns "this endpoint is here,
	// but not for DELETE" instead of a 405 it cannot read cross-origin.
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 — an existing path must still be preflighted", rec.Code)
	}
}

// TestCORS_UnknownPathIsNotPreflighted: a path that matches no route at all
// is not short-circuited. Two failure modes at once — a promised method on a
// path that can only 404, and (the reason this is a hard rule rather than
// taste) unbounded Prometheus cardinality: the 204 responder runs through
// middleware.Metrics, which labels its series with r.URL.Path, so answering
// arbitrary paths would let any client mint one time series per request.
func TestCORS_UnknownPathIsNotPreflighted(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/zz/no-such-endpoint", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want empty for an unrouted path", got)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 — an unrouted path must not be answered as a preflight", rec.Code)
	}
}

// TestCORS_MethodNotAllowedConsumesRateLimitToken pins the DoS hole the
// review measured: mux wraps neither NotFoundHandler nor
// MethodNotAllowedHandler in the Use chain, so before router.go set it by
// hand a client could pull unlimited 405s out of OPTIONS /v1/match (no
// Origin, hence no preflight short-circuit) without spending a single
// rate-limit token. With RateLimitPerSecond=1 the second such request must
// therefore be rejected, not served another 405.
func TestCORS_MethodNotAllowedConsumesRateLimitToken(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}, RateLimitPerSecond: 1})

	send := func() int {
		req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := send(); got != http.StatusMethodNotAllowed {
		t.Fatalf("first bare OPTIONS = %d, want 405", got)
	}
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("second bare OPTIONS = %d, want 429 — the 405 path bypassed the rate limiter", got)
	}
}

// TestCORS_PreflightIsRateLimited is the claim spec decision 3 actually
// makes, and which an X-Request-ID assertion alone does not prove: a
// preflight flood must hit the same limiter as everything else, or OPTIONS
// becomes a shape that bypasses the DoS protections.
func TestCORS_PreflightIsRateLimited(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}, RateLimitPerSecond: 1})

	send := func() int {
		req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
		req.Header.Set("Origin", "https://habitatus.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := send(); got != http.StatusNoContent {
		t.Fatalf("first preflight = %d, want 204", got)
	}
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("second preflight = %d, want 429 — preflights bypassed the rate limiter", got)
	}
}

// TestCORS_ServedMethodIsAdvertisedWithOptions: the advertised value must
// name the requested method AND OPTIONS, because the browser's own preflight
// verb has to be listed for the cached result to be reusable.
func TestCORS_ServedMethodIsAdvertisedWithOptions(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/suggest", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want %q", got, "GET, OPTIONS")
	}
}

// TestCORS_PreflightAdvertisesHeadersAndMaxAge: without the header
// allowlist a browser drops a JSON POST (Content-Type is not a CORS-safe
// value), and without Max-Age it repeats the preflight on every call.
func TestCORS_PreflightAdvertisesHeadersAndMaxAge(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Accept, Content-Type, X-Request-ID" {
		t.Fatalf("Access-Control-Allow-Headers = %q, want %q", got, "Accept, Content-Type, X-Request-ID")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("Access-Control-Max-Age = %q, want %q", got, "86400")
	}
}

// TestCORS_WildcardDefaultSetsStarWithoutVary: the zero-value Deps must keep
// the historical permissive default, and must NOT emit Vary — under a
// wildcard the response does not depend on Origin, so Vary would only cost
// shared-cache hits.
func TestCORS_WildcardDefaultSetsStarWithoutVary(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://anything.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rec.Header().Get("Vary"); strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want no Origin under a wildcard", got)
	}
}

// TestCORS_AllowlistRejectsForeignOrigin: a configured allowlist must not
// echo an origin it does not list, and must carry Vary so a shared cache
// cannot serve one origin's response to another.
func TestCORS_AllowlistRejectsForeignOrigin(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"https://ok.example"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for a foreign origin", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want it to contain Origin", got)
	}
}

// TestCORS_AllowlistEchoesListedOrigin is the positive counterpart.
func TestCORS_AllowlistEchoesListedOrigin(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"https://ok.example"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://ok.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ok.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the listed origin echoed", got)
	}
	// Vary matters most on the ALLOWED half: this response carries a concrete
	// Allow-Origin, so a shared cache that ignored Origin would hand
	// ok.example's response — headers and all — to any other origin asking
	// for the same URL.
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want it to contain Origin on an allowed response", got)
	}
}

// TestCORS_AllowlistIsCaseInsensitive pins the regression the review caught:
// internal/middleware/cors.go compared with strings.EqualFold, and browsers
// send the origin lowercased (RFC 6454). An operator who configured
// HOSTUS_CORS_ALLOWED_ORIGINS=https://Habitatus.Example would otherwise stop
// getting Access-Control-Allow-Origin after the upgrade — a silent break of
// a working browser app.
func TestCORS_AllowlistIsCaseInsensitive(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"https://Habitatus.Example"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://habitatus.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the origin echoed despite the configured casing", got)
	}
}

// TestCORS_OriginWithPathIsNotEchoed pins the one thing that must never
// happen: Access-Control-Allow-Origin carrying a value the CLIENT composed.
// Because the path is stripped on both sides before comparing, a request
// announcing "https://ok.example/anything" would otherwise satisfy the
// allowlist entry "https://ok.example" and then be mirrored back raw,
// suffix and all.
func TestCORS_OriginWithPathIsNotEchoed(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"https://ok.example"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://ok.example/anything")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty — an Origin with a path is not an origin", got)
	}
}

// TestCORS_AllowlistEntryWithPathStillMatches is the intended counterpart:
// pasting the app URL into the allowlist is the commonest configuration slip,
// and the entry comes from the operator, so it is read as the origin it means.
func TestCORS_AllowlistEntryWithPathStillMatches(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"https://ok.example/app"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://ok.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ok.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the origin echoed despite the path in the allowlist entry", got)
	}
}

// TestCORS_WildcardAmongConcreteOriginsStillAllowsAll: a "*" that is not the
// only entry must still mean allow-all. Anchored to allowAll's old
// len(origins)==1 shape, the star would be inert — matchOrigin never matches
// a bare "*", since it only treats a HOST starting with "*." as a wildcard —
// so the operator's intended "allow everything" would silently reject.
func TestCORS_WildcardAmongConcreteOriginsStillAllowsAll(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"*", "https://a.example"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://anywhere.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// TestCORS_PreflightFromForeignOriginStillGets204 pins spec decision 9: the
// preflight is answered uniformly whether or not the origin is allowed. The
// browser rejects it anyway for lack of Allow-Origin, and answering 204 only
// for listed origins would turn the endpoint into an origin oracle — probe
// with candidate origins, read the status, enumerate the allowlist.
func TestCORS_PreflightFromForeignOriginStillGets204(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}, CORSAllowedOrigins: []string{"https://ok.example"}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 — a foreign origin must get the same answer as a listed one", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for a foreign origin", got)
	}
}

// TestCORS_MultiOriginAllowlistChecksEveryEntry: an allowlist longer than
// one entry must be searched to the end, and a list of exactly two entries
// must not be mistaken for the "*" wildcard shape.
func TestCORS_MultiOriginAllowlistChecksEveryEntry(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{CORSAllowedOrigins: []string{"https://first.example", "https://second.example"}})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://second.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://second.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the second listed origin echoed", got)
	}
}

// TestCORS_RequestWithoutOriginIsUnchanged: a same-origin request must not
// grow CORS headers it never had.
func TestCORS_RequestWithoutOriginIsUnchanged(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	for _, h := range []string{
		"Access-Control-Allow-Origin", "Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods", "Access-Control-Max-Age",
	} {
		if got := rec.Header().Get(h); got != "" {
			t.Fatalf("%s = %q, want empty without an Origin header", h, got)
		}
	}
}

// TestCORS_PreflightCarriesRequestID proves the preflight runs through the
// middleware chain (spec decision 3) rather than being answered beside it —
// otherwise an OPTIONS flood would bypass rate limiting and load shedding.
// middleware.RequestID is the chain's second link and the only one that
// leaves a visible mark on the response (X-Request-ID), so its presence is
// the observable proof the chain ran.
func TestCORS_PreflightCarriesRequestID(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubCORSRepo{}})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("preflight carries no X-Request-ID — it bypassed the middleware chain")
	}
}
