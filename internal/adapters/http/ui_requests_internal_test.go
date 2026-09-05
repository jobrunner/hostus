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
// wantAbortedGuardCount is the exact number of `if (res.aborted) { return; }`
// call sites app.js is expected to carry: loadAreas, runSuggest, the Match
// click handler, the Translate click handler, loadSecSpaces, and the two
// loadCatalog calls (backbones + spaces) — 7 in total. The concept panel's
// Promise.all has its OWN combined form (checked separately below), not this
// literal, because a single shared then-handler covers two api() calls.
//
// This is an EXACT count, not just an existence check, on purpose: an
// existence check would stay green if a refactor stripped 6 of the 7 guards
// and left one behind by accident — the single most important property this
// test protects ("abort is never rendered as an error, in EVERY api()
// then-handler") would then silently regress undetected. Any new api() call
// site that legitimately needs this exact guard form MUST bump this constant
// (with a matching bump to the call-site list above); a guard removed
// without a corresponding intentional change is exactly what this pins
// against.
const wantAbortedGuardCount = 7

func TestUIRequestLifecycleProperties(t *testing.T) {
	a := embeddedAssets()

	// 1. Der Suggest bricht laufende Requests schon beim Tastendruck ab.
	for _, want := range []string{"AbortController", "suggestAbort.abort()"} {
		if !strings.Contains(a.js, want) {
			t.Errorf("app.js lost %q; typing would queue every keystroke's query on the single sqlite connection again", want)
		}
	}
	// 2. Aborts sind kein Fehler: api() klassifiziert AbortError, Render-Pfade
	//    kehren früh zurück — und zwar in JEDEM then-Handler, nicht nur in
	//    irgendeinem. Ein Existenz-Check ("kommt der String einmal vor?")
	//    bliebe grün, wenn ein Refactor 6 von 7 Guards entfernt; deshalb wird
	//    hier exakt gezählt.
	for _, want := range []string{`err && err.name === "AbortError"`, "aborted: true"} {
		if !strings.Contains(a.js, want) {
			t.Errorf("app.js lost %q; an aborted fetch would render as a bogus error box", want)
		}
	}
	if got := strings.Count(a.js, "if (res.aborted) { return; }"); got != wantAbortedGuardCount {
		t.Errorf("app.js has %d occurrences of the aborted-guard %q, want exactly %d; "+
			"a guard missing from even one api() then-handler means an aborted request "+
			"for THAT call site would render as a bogus error (or stale data) instead of "+
			"being silently discarded — count intentionally pinned exactly, not just checked for presence",
			got, "if (res.aborted) { return; }", wantAbortedGuardCount)
	}
	// The concept panel covers two api() calls (concept + synonyms) with one
	// shared Promise.all then-handler, so it uses its own combined form
	// instead of the single-response literal counted above.
	if !strings.Contains(a.js, "all[0].aborted || all[1].aborted") {
		t.Error("app.js lost the combined abort guard on the concept panel's Promise.all; " +
			"an aborted concept or synonyms fetch would render as a bogus error or stale panel")
	}
	// 3. Debounce-Fenster 250 ms (Spec-Entscheidung 3).
	if !strings.Contains(a.js, "setTimeout(runSuggest, 250)") {
		t.Error("app.js lost the 250ms debounce window (spec 2026-09-05, decision 3)")
	}
	// 4. Stale-Guards für Match und Translate (Suggest hat suggestSeq bereits).
	//    Die Variable allein pinnen reicht nicht: ein entkernter Guard, bei
	//    dem `matchSeq`/`translateSeq` nur noch deklariert, aber nicht mehr
	//    VERGLICHEN wird, ließe die letzte EINTREFFENDE statt der zuletzt
	//    gesendeten Antwort wieder gewinnen — deshalb zusätzlich die
	//    Vergleichsform pinnen.
	for _, want := range []string{"matchSeq", "translateSeq", "seq !== matchSeq", "seq !== translateSeq"} {
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
