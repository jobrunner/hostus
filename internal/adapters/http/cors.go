package httpx

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/gorilla/mux"
)

// corsMaxAgeSeconds is how long a browser may cache a preflight result.
const corsMaxAgeSeconds = "86400" // 24 hours

// corsAllowHeaders is the fixed request-header allowlist. X-Request-ID is
// hostus-specific: a client that correlates its own logs with the service's
// may set it, and a preflight has to admit it explicitly.
const corsAllowHeaders = "Accept, Content-Type, X-Request-ID"

// corsWrapper decorates the assembled router with CORS.
//
// READ THIS BEFORE MOVING IT BACK INTO THE Use CHAIN:
// gorilla/mux runs Use-registered middleware only for requests that MATCH a
// route. An OPTIONS preflight against a route registered as .Methods(POST)
// matches nothing and goes to the MethodNotAllowedHandler — outside the
// chain. Registered with router.Use, CORS therefore answered every preflight
// with a bare 405 and no CORS headers, which made /v1/match and
// /v1/translate unusable from a browser (measured on v3.4.0-alpha.0). The
// trap is that a GET-only client never sends a preflight, so the defect
// stays invisible until an endpoint takes a JSON body.
//
// allowAll mirrors the historical default: no configured origins means
// "Access-Control-Allow-Origin: *". That stays safe only because
// Access-Control-Allow-Credentials is never set — without it a browser
// sends neither cookies nor Authorization, so a wildcard exposes nothing
// beyond what an unauthenticated GET already serves.
type corsWrapper struct {
	router    *mux.Router
	origins   []string
	allowAll  bool
	preflight http.Handler
}

// newCORSWrapper builds the wrapper. preflight is the 204 responder already
// wrapped in the middleware chain, so a preflight stays observable (request
// id, logs, spans, metrics) and rate-limited — answering it here directly
// would hand any client an OPTIONS-shaped bypass of the load shedder.
func newCORSWrapper(router *mux.Router, origins []string, preflight http.Handler) *corsWrapper {
	return &corsWrapper{
		router:  router,
		origins: origins,
		// "*" anywhere in the list means allow-all, not just as the sole
		// entry: a "*" that sits next to concrete origins would otherwise be
		// silently inert, because matchOrigin only treats a pattern as a
		// wildcard when its HOST starts with "*." — a bare "*" matches
		// nothing at all there.
		allowAll:  slices.Contains(origins, "*"),
		preflight: preflight,
	}
}

// Unwrap exposes the wrapped router. NewRouter returns an http.Handler, so
// the only way back to the *mux.Router — which the OpenAPI contract test
// needs in order to Walk the mounted routes — is through this accessor.
func (c *corsWrapper) Unwrap() *mux.Router { return c.router }

// routeProbe is what one lookup in the routing table tells us about the
// method a preflight announces.
type routeProbe struct {
	// pathExists is true when SOME route is registered for this path, even
	// if not for the announced method.
	pathExists bool
	// methodServed is true when the announced method is actually served.
	methodServed bool
}

func (c *corsWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	requestedMethod := r.Header.Get("Access-Control-Request-Method")

	// The routing table is consulted EXACTLY once, because both decisions
	// below hang off the same answer: which method to advertise, and whether
	// this request may be short-circuited at all.
	var route routeProbe
	if origin != "" && requestedMethod != "" {
		route = c.probeRoute(r, requestedMethod)
	}

	if origin != "" {
		if c.allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			if c.isOriginAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			// Vary is set for any cross-origin request, allowed or not: the
			// response depends on Origin, so a shared cache must not hand
			// one origin's response to another. Under a wildcard the
			// response does not depend on Origin, so Vary would only cost
			// cache hits.
			w.Header().Add("Vary", "Origin")
		}

		// Advertise the method the ROUTER actually accepts for this path
		// rather than a hand-written list. A literal "GET, POST, OPTIONS"
		// is correct exactly until someone adds a DELETE route: nothing
		// fails at build time, and the endpoint is simply unusable from a
		// browser.
		if route.methodServed {
			w.Header().Set("Access-Control-Allow-Methods", requestedMethod+", OPTIONS")
		}
		w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		w.Header().Set("Access-Control-Max-Age", corsMaxAgeSeconds)
	}

	// Only a REAL preflight is short-circuited: OPTIONS carrying both Origin
	// and Access-Control-Request-Method, FOR A PATH THAT EXISTS. A bare
	// OPTIONS keeps falling through to the router exactly as before, so
	// enabling CORS never turns a 405 into a silent 204.
	//
	// The path condition is not pedantry. The 204 responder runs through the
	// middleware chain, and middleware.Metrics labels its series with
	// r.URL.Path — answering an unrouted path would let anyone mint
	// unbounded Prometheus time series with OPTIONS /zz/1, /zz/2, ... An
	// unknown path therefore falls through to the router's own 404, which is
	// also the semantically honest answer: there is nothing here to preflight.
	//
	// The 204 is returned whether or not the origin is allowed: without the
	// Allow-Origin header the browser rejects the response anyway, and a
	// uniform answer avoids leaking which origins are configured.
	if r.Method == http.MethodOptions && origin != "" && requestedMethod != "" && route.pathExists {
		c.preflight.ServeHTTP(w, r)
		return
	}

	c.router.ServeHTTP(w, r)
}

// probeRoute asks the router whether method+path would match a route.
// mux reports a path that exists under a different method via
// RouteMatch.MatchErr == ErrMethodMismatch, so a method the service does not
// serve is never advertised as allowed, while the path still counts as
// existing.
//
// The bool Match returns is deliberately not the whole answer: whether mux
// returns true on a method mismatch depends on MethodNotAllowedHandler being
// set, so MatchErr is the stable signal.
func (c *corsWrapper) probeRoute(r *http.Request, method string) routeProbe {
	// A shallow copy suffices — Match only reads the request, and unlike
	// r.Clone it costs no header/URL allocation on a hot path.
	probe := *r
	probe.Method = method
	var match mux.RouteMatch
	if c.router.Match(&probe, &match) && match.MatchErr == nil {
		return routeProbe{pathExists: true, methodServed: true}
	}
	return routeProbe{pathExists: errors.Is(match.MatchErr, mux.ErrMethodMismatch)}
}

// isOriginAllowed reports whether origin matches any configured pattern.
func (c *corsWrapper) isOriginAllowed(origin string) bool {
	for _, pattern := range c.origins {
		if matchOrigin(origin, pattern) {
			return true
		}
	}
	return false
}

// matchOrigin matches an origin against one pattern: either exactly, or as a
// "*.example.com" wildcard covering subdomains (but not the bare domain).
//
// Only the host label is wildcarded. Scheme and port must still match
// exactly, so "https://*.example.com" does NOT admit
// "http://sub.example.com" (plaintext) or "https://sub.example.com:8443" (a
// different service on the same host) — an origin is the scheme/host/port
// triple, and widening it silently would hand responses to servers the
// operator never listed.
func matchOrigin(origin, pattern string) bool {
	oScheme, oHost, oPort := splitOrigin(origin)
	pScheme, pHost, pPort := splitOrigin(pattern)
	if oScheme != pScheme || oPort != pPort {
		return false
	}
	// Scheme and host arrive case-folded from splitOrigin, so the exact
	// comparison is case-insensitive — the predecessor in
	// internal/middleware/cors.go used strings.EqualFold, and an operator who
	// configured "https://Habitatus.Example" must keep getting an
	// Access-Control-Allow-Origin after this refactor. Browsers send the
	// origin lowercased (RFC 6454), so a case-sensitive compare would fail
	// silently in exactly the configuration nobody tests.
	if oHost == pHost {
		return true
	}
	if !strings.HasPrefix(pHost, "*.") {
		return false
	}
	suffix := pHost[1:] // "*.example.com" -> ".example.com"
	// len > len(suffix) keeps "example.com" itself out, and requiring the
	// leading dot keeps "evil-example.com" out.
	return strings.HasSuffix(oHost, suffix) && len(oHost) > len(suffix)
}

// splitOrigin breaks an origin (or a wildcard pattern) into scheme, host and
// port. A pattern written without a scheme yields an empty scheme, which then
// only matches an equally scheme-less origin — browsers always send one, so
// such a pattern matches nothing rather than being quietly widened.
//
// Scheme and host come back lowercased because both are case-insensitive per
// RFC 3986; the port is not folded (it is digits) and the path is discarded
// entirely, so no case-sensitive component is ever touched.
func splitOrigin(origin string) (scheme, host, port string) {
	rest := origin
	if idx := strings.Index(rest, "://"); idx != -1 {
		scheme, rest = rest[:idx], rest[idx+3:]
	}
	if idx := strings.Index(rest, "/"); idx != -1 {
		rest = rest[:idx]
	}
	if idx := strings.LastIndex(rest, ":"); idx != -1 {
		rest, port = rest[:idx], rest[idx+1:]
	}
	return strings.ToLower(scheme), strings.ToLower(rest), port
}
