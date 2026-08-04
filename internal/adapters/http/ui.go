package httpx

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// The embedded single-page test console. It lives in this package rather
// than in an adapters/ui sibling on purpose: the assets are an
// implementation detail of the HTTP adapter, not a second adapter. A
// separate package would have needed a depguard exception for the
// adapter-to-adapter import, and weakening an architecture rule for a
// purely mechanical reason is the wrong trade.
//
// Two constraints shape everything below.
//
// The service is offline-first by design (UC1 is field use with an offline
// bundle), so the console must not reference a CDN, a webfont or any other
// external origin — a page that needed the network would contradict the
// product and would fail only where it matters, in the field.
//
// The console shares the API's single global 20 rps token bucket (the UI
// route sits inside the fixed middleware chain, see NewRouter). A page that
// pulled a dozen assets would eat the very budget a tester is trying to
// observe latency in. The stylesheet and the script are therefore INLINED
// into the document at build time, and the Content-Security-Policy admits
// them by sha256 hash rather than by 'unsafe-inline'.

// Sentinels inside assets/index.html that the stylesheet and the script are
// substituted for. They are CSS/JS comments so the raw asset stays a valid
// document that a browser or an editor can still open.
const (
	uiStyleSentinel  = "/*hostus:style*/"
	uiScriptSentinel = "/*hostus:script*/"
)

//go:embed assets/index.html
var uiIndexHTML string

//go:embed assets/style.css
var uiStyleCSS string

//go:embed assets/app.js
var uiAppJS string

//go:embed assets/style.css assets/app.js
var uiAssetsFS embed.FS

// uiAssetContentTypes is the explicit extension -> Content-Type table. It is
// deliberately not mime.TypeByExtension: that consults the host's mime
// database, so the same binary would answer differently on two machines.
var uiAssetContentTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
}

var (
	uiDocumentOnce = sync.OnceValue(buildUIDocument)
	uiCSPOnce      = sync.OnceValue(buildUICSP)
	uiETagOnce     = sync.OnceValue(buildUIDocumentETag)
)

// buildUIDocument composes the one self-contained page.
func buildUIDocument() string {
	doc := strings.Replace(uiIndexHTML, uiStyleSentinel, uiStyleCSS, 1)
	return strings.Replace(doc, uiScriptSentinel, uiAppJS, 1)
}

// uiHashSource renders s as a CSP hash-source expression. The hash covers
// the exact bytes the browser will see as the element's text content, which
// is only true because buildUIDocument substitutes the asset verbatim.
func uiHashSource(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// buildUICSP names no origin at all beyond 'self', and admits the two
// inline elements by hash. 'unsafe-inline' is deliberately absent: with a
// hash present the console still runs, while anything injected into the
// page later does not.
func buildUICSP() string {
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + uiHashSource(uiAppJS),
		"style-src " + uiHashSource(uiStyleCSS),
		"connect-src 'self'",
		"img-src 'self' data:",
		"font-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"object-src 'none'",
	}, "; ")
}

func buildUIDocumentETag() string {
	return uiETagFor([]byte(uiDocumentOnce()))
}

func uiETagFor(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// serveUIBytes writes b with a strong ETag so a reload costs a 304 rather
// than the whole document. modTime is deliberately zero: the assets are
// compiled in, so there is no honest modification time to report, and the
// content hash is the stronger validator anyway.
func serveUIBytes(w http.ResponseWriter, r *http.Request, name, contentType, etag string, b []byte) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", etag)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

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
	w.Header().Set("Content-Security-Policy", uiCSPOnce())
	serveUIBytes(w, r, "index.html", "text/html; charset=utf-8", uiETagOnce(), []byte(uiDocumentOnce()))
}

// handleUIAsset serves the individually addressable embedded assets under
// /assets/. The page itself never fetches them — it inlines both — but they
// stay reachable so a tester can read the exact CSS and JS the console runs
// without extracting them from the page source. Only the two known names
// resolve; anything else is a 404.
func handleUIAsset(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.URL.Path)
	contentType, ok := uiAssetContentTypes[path.Ext(name)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	content, err := uiAssetsFS.ReadFile("assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveUIBytes(w, r, name, contentType, uiETagFor(content), content)
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
