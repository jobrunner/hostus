package httpx

import (
	"strings"
	"testing"
)

// TestUIRequestLifecycleProperties pins the request-lifecycle mechanics the
// console depends on (spec 2026-09-05): stale in-flight requests are ABORTED
// (not merely ignored at render time), aborts are never rendered as errors,
// every panel has a stale guard, and an unparseable 2xx body is an error —
// properties a refactor of app.js could silently drop, which no Go test
// would otherwise notice (there is no JS test infrastructure).
func TestUIRequestLifecycleProperties(t *testing.T) {
	a := embeddedAssets()

	// 1. Der Suggest bricht laufende Requests schon beim Tastendruck ab.
	for _, want := range []string{"AbortController", "suggestAbort.abort()"} {
		if !strings.Contains(a.js, want) {
			t.Errorf("app.js lost %q; typing would queue every keystroke's query on the single sqlite connection again", want)
		}
	}
	// 2. Aborts sind kein Fehler: api() klassifiziert AbortError, Render-Pfade
	//    kehren früh zurück.
	for _, want := range []string{`err && err.name === "AbortError"`, "aborted: true", "if (res.aborted) { return; }"} {
		if !strings.Contains(a.js, want) {
			t.Errorf("app.js lost %q; an aborted fetch would render as a bogus error box", want)
		}
	}
	// 3. Debounce-Fenster 250 ms (Spec-Entscheidung 3).
	if !strings.Contains(a.js, "setTimeout(runSuggest, 250)") {
		t.Error("app.js lost the 250ms debounce window (spec 2026-09-05, decision 3)")
	}
	// 4. Stale-Guards für Match und Translate (Suggest hat suggestSeq bereits).
	for _, want := range []string{"matchSeq", "translateSeq"} {
		if !strings.Contains(a.js, want) {
			t.Errorf("app.js lost %q; the last ARRIVING (not last sent) response would win that panel again", want)
		}
	}
	// 5. Leer-Query-Fix: der Early-Return erhöht die Sequenz.
	if !strings.Contains(a.js, "suggestSeq += 1; // Leer-Query") {
		t.Error("app.js lost the empty-query sequence bump; a stale response could fill the just-cleared panel")
	}
	// 6. Kaputtes JSON bei 2xx ist ein Fehler, kein leeres Ergebnis.
	if !strings.Contains(a.js, "!res.body") {
		t.Error("app.js lost the unparseable-body error path; a truncated 200 response would render as 'Keine Treffer.'")
	}
}
