# CORS-Preflight-Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ein Browser kann `POST /v1/match` und `POST /v1/translate`
cross-origin aufrufen — der Preflight wird beantwortet (204 + Header), und
`Access-Control-Allow-Methods` stammt aus der Routen-Tabelle statt aus einer
handgeschriebenen Liste.

**Architecture:** CORS wandert aus der `Use`-Kette (wo der Preflight nie
ankommt) in einen Wrapper um den fertigen Router, angesiedelt in
`internal/adapters/http/cors.go`. Echte Preflights werden über
`applyChain(chain, …)` beantwortet, damit sie observierbar und
rate-limitiert bleiben. `internal/middleware/cors.go` entfällt.

**Tech Stack:** Go 1.26, `gorilla/mux`, Standardbibliothek. Keine neue
Abhängigkeit.

**Spec:** `docs/superpowers/specs/2026-09-14-cors-preflight-fix.md`

## Global Constraints

- Nur erlaubte Bibliotheken (Go-Stdlib, `gorilla/mux`) — keine neue Dependency.
- `internal/adapters/http` ist mutation-gated: `make mutation PKG=./internal/adapters/http` muss `Not covered: 0` zeigen.
- Der Fitness-Test muss gegen den ALTEN Code fehlschlagen (Kontroll-Assertion), sonst pinnt er nichts.
- `Access-Control-Allow-Credentials` wird NIE gesetzt.
- Default ohne Konfiguration bleibt `Access-Control-Allow-Origin: *`.
- Allow-Headers bleibt `Accept, Content-Type, X-Request-ID`, Max-Age `86400`.
- Ein blankes `OPTIONS` (ohne `Origin` UND `Access-Control-Request-Method`) fällt an den Router durch.
- Antworten ohne `Origin`-Header ändern sich nicht.
- CHANGELOG unter `## [Unreleased]`, Conventional Commits, englische WHY-Kommentare.
- **NIEMALS `git add -A` oder `git add .`** — nur die in den Tasks genannten Dateien einzeln adden. Im Arbeitsbaum liegen unversionierte Dateien des Nutzers (`out/` mit mehreren GB, `console-*.png`, `.playwright-mcp/`, `build-full.yaml`), die auf keinen Fall committet werden dürfen.

## File Structure

- `internal/adapters/http/cors.go` — **neu**: `corsConfig`, `wrapCORS`, `routeAllowsMethod`, `isOriginAllowed`, `matchOrigin`, `splitOrigin`.
- `internal/adapters/http/cors_internal_test.go` — **neu**: Unit-Tests für `matchOrigin`/`splitOrigin`.
- `internal/adapters/http/cors_test.go` — **neu**: Verhaltenstests über `NewRouter` (Fitness-Test, Preflight, blankes OPTIONS, Allowlist, Vary).
- `internal/adapters/http/router.go` — **geändert**: `NewRouter` liefert `http.Handler`, CORS raus aus `chain`, Wrapper außen herum.
- `internal/middleware/cors.go` — **gelöscht**.
- `CHANGELOG.md`, `docs/reference/configuration.md` — Doku.

---

### Task 1: CORS-Wrapper mit routen-abgeleiteten Methoden

**Files:**
- Create: `internal/adapters/http/cors.go`
- Create: `internal/adapters/http/cors_internal_test.go`
- Create: `internal/adapters/http/cors_test.go`
- Modify: `internal/adapters/http/router.go`
- Delete: `internal/middleware/cors.go`

**Interfaces:**
- Consumes: `applyChain(mws []mux.MiddlewareFunc, h http.Handler) http.Handler` (existiert bereits in `router.go`), `Deps.CORSAllowedOrigins`.
- Produces: `NewRouter(deps Deps) http.Handler` (Rückgabetyp geändert von `*mux.Router`).

- [ ] **Step 1: Fitness-Test schreiben, der den Bug zeigt**

Neue Datei `internal/adapters/http/cors_test.go`. Dieser Test MUSS mit dem
heutigen Code fehlschlagen:

```go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// TestCORS_PreflightOnPostRouteIsAnswered pins the regression that made
// /v1/match unusable from a browser: gorilla/mux runs Use-registered
// middleware only for MATCHED routes, so an OPTIONS preflight against a
// route registered as .Methods(POST) matched nothing, fell through to the
// MethodNotAllowedHandler outside the chain, and came back as a bare 405
// with no CORS headers at all. Measured on v3.4.0-alpha.0.
func TestCORS_PreflightOnPostRouteIsAnswered(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})

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
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/ -run TestCORS_PreflightOnPostRouteIsAnswered -v`
Expected: FAIL mit `preflight status = 405, want 204`.
Den beobachteten Fehlschlag im Report festhalten — er ist der Beleg, dass der Test etwas pinnt.

- [ ] **Step 3: `internal/adapters/http/cors.go` anlegen**

```go
package http

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
```

- [ ] **Step 4: `router.go` umbauen**

Vier Änderungen, sonst nichts:

1. Signatur: `func NewRouter(deps Deps) http.Handler` (Doc-Kommentar
   anpassen: die Kette endet jetzt auf `Metrics`, CORS sitzt außen herum
   und ist kein Kettenglied mehr — mit dem WHY aus `cors.go` als
   Ein-Satz-Verweis).
2. `middleware.CORS(origins)` aus dem `chain`-Slice entfernen (die übrigen
   sieben Glieder bleiben in exakt dieser Reihenfolge).
3. Den `middleware`-Import auf Bedarf prüfen (die anderen Glieder nutzen
   ihn weiterhin — Import bleibt).
4. Am Ende statt `return r`:

```go
	// CORS wraps the FINISHED router: a preflight against a POST-only route
	// matches no route at all, so it never reaches Use-registered
	// middleware (see cors.go). The 204 responder is wrapped in the same
	// chain by hand, so preflights stay observable and rate-limited.
	preflight := applyChain(chain, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	return newCORSWrapper(r, origins, preflight)
```

- [ ] **Step 5: `internal/middleware/cors.go` löschen**

```bash
git rm internal/middleware/cors.go
```

Anschließend prüfen, dass keine Referenz übrig ist:
`grep -rn "middleware.CORS" --include="*.go" . | grep -v third_party`
Erwartet: keine Treffer.

- [ ] **Step 6: Fitness-Test muss jetzt bestehen**

Run: `go test ./internal/adapters/http/ -run TestCORS_PreflightOnPostRouteIsAnswered -v`
Expected: PASS

- [ ] **Step 7: Verhaltenstests ergänzen** (in `cors_test.go`)

Jeder Test benennt im Kommentar, welchen Fehlschlag er verhindert:

```go
// TestCORS_BareOptionsStillReachesRouter guards the "enabling CORS turns
// every OPTIONS into a 204" failure mode: without Origin and
// Access-Control-Request-Method this is not a preflight, so the router's
// own 405 must survive.
func TestCORS_BareOptionsStillReachesRouter(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		t.Fatal("bare OPTIONS was answered as a preflight")
	}
}

// TestCORS_UnservedMethodIsNotAdvertised: a method the router does not
// serve for this path must never appear in Allow-Methods, or a browser is
// told to try a request that can only 405.
func TestCORS_UnservedMethodIsNotAdvertised(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want empty for an unserved method", got)
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
func TestCORS_PreflightCarriesRequestID(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})

	req := httptest.NewRequest(http.MethodOptions, "/v1/match", nil)
	req.Header.Set("Origin", "https://habitatus.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("preflight carries no X-Request-ID — it bypassed the middleware chain")
	}
}
```

**Hinweis:** Falls die Request-ID-Middleware den Header anders benennt, den
tatsächlichen Namen aus `internal/middleware/requestid.go` verwenden — der
Test prüft „die Kette lief", nicht eine bestimmte Schreibweise.

- [ ] **Step 8: Unit-Tests für die Origin-Musterlogik** (`cors_internal_test.go`, Paket `http`)

```go
func TestMatchOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		pattern string
		want    bool
	}{
		{"exact", "https://a.example", "https://a.example", true},
		{"exact mismatch", "https://b.example", "https://a.example", false},
		{"wildcard subdomain", "https://sub.example.com", "https://*.example.com", true},
		{"wildcard deep subdomain", "https://a.b.example.com", "https://*.example.com", true},
		{"wildcard excludes bare domain", "https://example.com", "https://*.example.com", false},
		{"wildcard excludes lookalike", "https://evilexample.com", "https://*.example.com", false},
		{"wildcard keeps scheme", "http://sub.example.com", "https://*.example.com", false},
		{"wildcard keeps port", "https://sub.example.com:8443", "https://*.example.com", false},
		{"pattern without scheme matches nothing real", "https://sub.example.com", "*.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchOrigin(tt.origin, tt.pattern); got != tt.want {
				t.Fatalf("matchOrigin(%q, %q) = %v, want %v", tt.origin, tt.pattern, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 9: Volle Test- und Lint-Runde**

Run: `go test ./... && make lint && make arch`
Expected: alles grün. Falls `NewRouter`s neuer Rückgabetyp irgendwo klemmt,
an der Aufrufstelle beheben (erwartet: nirgends — `app.App.Router` ist
bereits `http.Handler`).

- [ ] **Step 10: Mutation-Gate**

Run: `make mutation PKG=./internal/adapters/http`
Expected: `Not covered: 0`. Lebende Mutanten mit Tests töten, die eine
echte Verhaltensaussage treffen — keine Assertion, die nur den Mutanten
spiegelt.

**Achtung tag-loser `switch`:** Mutations-Coverage wird solchen
`case`-Bedingungen nicht zugerechnet. Falls beim Töten ein `switch { case
x > 1: … }` entsteht, stattdessen verschachtelte `if`s oder eine
Datentabelle verwenden (gocritic verbietet `ifElseChain` ab drei Zweigen).

- [ ] **Step 11: Commit**

```bash
git add internal/adapters/http/cors.go internal/adapters/http/cors_test.go \
        internal/adapters/http/cors_internal_test.go internal/adapters/http/router.go
git rm internal/middleware/cors.go
git commit -m "fix(http): CORS-Preflight beantworten statt 405 (POST-Endpunkte browser-tauglich)"
```

---

### Task 2: Dokumentation

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `docs/reference/configuration.md`
- Modify: `example.env` (nur falls der Kommentar dort die Wildcard-Syntax nicht erwähnt)

**Interfaces:**
- Consumes: das Verhalten aus Task 1.
- Produces: nichts (reine Doku).

- [ ] **Step 1: CHANGELOG unter `## [Unreleased]`**

```markdown
### Fixed

* **http:** CORS-Preflight (`OPTIONS` mit `Origin` +
  `Access-Control-Request-Method`) wird beantwortet statt mit einem nackten
  `405` ohne CORS-Header abgewiesen. `POST /v1/match` und
  `POST /v1/translate` waren dadurch aus einem Browser nicht aufrufbar.
  Ursache: `gorilla/mux` führt `Use`-Middleware nur für gematchte Routen
  aus — ein Preflight gegen eine `.Methods(POST)`-Route matcht nichts.
  CORS umschließt jetzt den fertigen Router. Zusätzlich stammt
  `Access-Control-Allow-Methods` aus der Routen-Tabelle statt aus einer
  handgeschriebenen `GET, OPTIONS`-Liste.

### Added

* **http:** Origin-Allowlist versteht Subdomain-Wildcards
  (`https://*.example.com`); Schema und Port müssen exakt passen.
  `Vary: Origin` wird bei konfigurierter Allowlist immer gesetzt.
```

- [ ] **Step 2: `docs/reference/configuration.md`**

Den `cors.allowed_origins`-Abschnitt suchen und ergänzen: Wildcard-Syntax,
scheme/port-genaue Semantik, der `*`-Default samt Begründung (read-only,
kein `Allow-Credentials`), und dass `Allow-Methods` aus den Routen stammt.
Falls es den Abschnitt noch nicht gibt, einen anlegen, der zum Stil der
Datei passt.

- [ ] **Step 3: Doku-Build**

Run: `make docs` (oder `uvx --with mkdocs-material mkdocs build --strict`)
Expected: grün.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md docs/reference/configuration.md
git commit -m "docs(cors): Preflight-Fix und Wildcard-Allowlist dokumentieren"
```
