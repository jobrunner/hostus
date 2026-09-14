package httpx

import (
	"net/http"
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
		router:    router,
		origins:   origins,
		allowAll:  len(origins) == 1 && origins[0] == "*",
		preflight: preflight,
	}
}

// Unwrap exposes the wrapped router. NewRouter returns an http.Handler, so
// the only way back to the *mux.Router — which the OpenAPI contract test
// needs in order to Walk the mounted routes — is through this accessor.
func (c *corsWrapper) Unwrap() *mux.Router { return c.router }

func (c *corsWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		if c.allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if c.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if !c.allowAll {
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
		if m := r.Header.Get("Access-Control-Request-Method"); m != "" && c.routeAllowsMethod(r, m) {
			w.Header().Set("Access-Control-Allow-Methods", m+", OPTIONS")
		}
		w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		w.Header().Set("Access-Control-Max-Age", corsMaxAgeSeconds)
	}

	// Only a REAL preflight is short-circuited: OPTIONS carrying both Origin
	// and Access-Control-Request-Method. A bare OPTIONS keeps falling
	// through to the router exactly as before, so enabling CORS never turns
	// a 405 into a silent 204.
	//
	// The 204 is returned whether or not the origin is allowed: without the
	// Allow-Origin header the browser rejects the response anyway, and a
	// uniform answer avoids leaking which origins are configured.
	if r.Method == http.MethodOptions && origin != "" && r.Header.Get("Access-Control-Request-Method") != "" {
		c.preflight.ServeHTTP(w, r)
		return
	}

	c.router.ServeHTTP(w, r)
}

// routeAllowsMethod asks the router whether method+path would match a route.
// mux reports a path that exists under a different method via
// RouteMatch.MatchErr == ErrMethodMismatch, so a method the service does not
// serve is never advertised as allowed.
func (c *corsWrapper) routeAllowsMethod(r *http.Request, method string) bool {
	probe := r.Clone(r.Context())
	probe.Method = method
	var match mux.RouteMatch
	return c.router.Match(probe, &match) && match.MatchErr == nil
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
	if origin == pattern {
		return true
	}

	oScheme, oHost, oPort := splitOrigin(origin)
	pScheme, pHost, pPort := splitOrigin(pattern)
	if oScheme != pScheme || oPort != pPort {
		return false
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
	return scheme, rest, port
}
