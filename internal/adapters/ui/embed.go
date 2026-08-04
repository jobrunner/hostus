// Package ui embeds the single-page test console and serves it as ONE
// self-contained HTML response.
//
// Two constraints shape everything here.
//
// The service is offline-first by design (UC1 is field use with an offline
// bundle), so the console must not reference a CDN, a webfont or any other
// external origin — a page that needed the network would contradict the
// product and would fail only where it matters, in the field.
//
// The console shares the API's single global 20 rps token bucket (the UI
// route sits inside the fixed middleware chain, see the router). A page
// that pulled a dozen assets would eat the very budget a tester is trying
// to observe latency in. The stylesheet and the script are therefore
// INLINED into the document at build time, and the Content-Security-Policy
// admits them by sha256 hash rather than by 'unsafe-inline'.
package ui

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

// Sentinels inside assets/index.html that the stylesheet and the script are
// substituted for. They are CSS/JS comments so the raw asset stays a valid
// document that a browser or an editor can still open.
const (
	styleSentinel  = "/*hostus:style*/"
	scriptSentinel = "/*hostus:script*/"
)

//go:embed assets/index.html
var indexHTML string

//go:embed assets/style.css
var styleCSS string

//go:embed assets/app.js
var appJS string

//go:embed assets/style.css assets/app.js
var assetsFS embed.FS

// assetContentTypes is the explicit extension -> Content-Type table. It is
// deliberately not mime.TypeByExtension: that consults the host's mime
// database, so the same binary would answer differently on two machines.
var assetContentTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
}

var (
	documentOnce = sync.OnceValue(buildDocument)
	cspOnce      = sync.OnceValue(buildCSP)
	etagOnce     = sync.OnceValue(buildDocumentETag)
)

// StyleSource and ScriptSource expose the raw assets so a test can assert
// that what the CSP hashes is exactly what the document inlines.
func StyleSource() string  { return styleCSS }
func ScriptSource() string { return appJS }

// Document is the composed, self-contained console page.
func Document() []byte { return []byte(documentOnce()) }

// ContentSecurityPolicy is the policy served with the page.
func ContentSecurityPolicy() string { return cspOnce() }

func buildDocument() string {
	doc := strings.Replace(indexHTML, styleSentinel, styleCSS, 1)
	return strings.Replace(doc, scriptSentinel, appJS, 1)
}

// hashSource renders s as a CSP hash-source expression. The hash covers the
// exact bytes the browser will see as the element's text content, which is
// only true because buildDocument substitutes the asset verbatim.
func hashSource(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// buildCSP names no origin at all beyond 'self', and admits the two inline
// elements by hash. 'unsafe-inline' is deliberately absent: with a hash
// present the console still runs, while anything injected into the page
// later does not.
func buildCSP() string {
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + hashSource(appJS),
		"style-src " + hashSource(styleCSS),
		"connect-src 'self'",
		"img-src 'self' data:",
		"font-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"object-src 'none'",
	}, "; ")
}

func buildDocumentETag() string {
	return etagFor([]byte(documentOnce()))
}

func etagFor(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// serveBytes writes b with a strong ETag so a reload costs a 304 rather
// than the whole document. modTime is deliberately zero: the assets are
// compiled in, so there is no honest modification time to report, and the
// content hash is the stronger validator anyway.
func serveBytes(w http.ResponseWriter, r *http.Request, name, contentType, etag string, b []byte) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", etag)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

// Handler serves the console document.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspOnce())
		serveBytes(w, r, "index.html", "text/html; charset=utf-8", etagOnce(), Document())
	})
}

// AssetHandler serves the individual embedded assets under /assets/. The
// page itself never fetches them — it inlines both — but they stay
// reachable so a tester can read the exact CSS and JS the console runs
// without extracting them from the page source. Only the two known names
// resolve; anything else is a 404.
func AssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Base(r.URL.Path)
		contentType, ok := assetContentTypes[path.Ext(name)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		content, err := assetsFS.ReadFile("assets/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		serveBytes(w, r, name, contentType, etagFor(content), content)
	})
}
