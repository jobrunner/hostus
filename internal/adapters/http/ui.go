package httpx

import (
	"net/http"
	"strings"

	"github.com/jobrunner/hostus/internal/adapters/ui"
)

// apiPrefixes are the path prefixes the API owns. An unknown path UNDER one
// of them keeps mux's plain 404: the API contract must not change shape
// because a UI exists. Everything else that does not match a route is an
// SPA deep link and gets the console.
var apiPrefixes = []string{"/v1", "/health", "/metrics", "/openapi"}

// underAPIPrefix reports whether p is the prefix itself or a path below it.
// The segment boundary matters: "/metrics" and "/metrics/foo" belong to the
// API, "/metrics-dashboard" is just an unrouted path and may be a deep link.
func underAPIPrefix(p string) bool {
	for _, prefix := range apiPrefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// handleUI serves the embedded console at "/". It is a plain HTTP adapter:
// the console talks to the same public /v1 API a browser would, so nothing
// here reaches into the application or domain layer.
func handleUI(w http.ResponseWriter, r *http.Request) {
	ui.Handler().ServeHTTP(w, r)
}

// handleUIAsset serves the individually addressable embedded assets.
func handleUIAsset(w http.ResponseWriter, r *http.Request) {
	ui.AssetHandler().ServeHTTP(w, r)
}

// spaFallback answers every request that matched no route at all.
//
// Only GET and HEAD reach the console: replying to a POST on an unrouted
// path with an HTML page would be a new behavior nobody asked for, so
// those keep the plain 404 they have today. Note that mux reports a
// method mismatch on a KNOWN path (405) before it ever consults
// NotFoundHandler, so this never swallows a 405.
func spaFallback(page http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if underAPIPrefix(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		page.ServeHTTP(w, r)
	}
}
