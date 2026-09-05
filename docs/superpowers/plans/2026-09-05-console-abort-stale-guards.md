# Konsole: Abort + Stale-Guards Implementation Plan (Paket A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die Testkonsole bricht veraltete In-Flight-Requests ab (Suggest beim Tippen, Match/Translate beim erneuten Klick), rendert Aborts nie als Fehler, schließt die drei Stale-Lücken des Reviews und behandelt kaputtes JSON als Fehler.

**Architecture:** Reines Frontend (assets/app.js) + Go-Marker-Tests im bestehenden `ui_*_internal_test.go`-Muster. Keine API-Änderung.

**Tech Stack:** Vanilla JS (Stil der Datei: `var`, `function`, deutsche Kommentare), Go-Tests mit `strings.Contains` auf den eingebetteten Assets.

**Spec:** `docs/superpowers/specs/2026-09-05-console-abort-stale-guards.md`

## Global Constraints

- `internal/adapters/http` ist mutation-gated: die neuen Marker-Tests folgen exakt dem Muster von `ui_a11y_internal_test.go` (jede Prüfung mit erklärender Fehlermeldung, was verloren ginge).
- Abort darf NIE als Fehler gerendert werden (Spec-Entscheidung 2) — jeder `api()`-Konsument bekommt den `res.aborted`-Early-Return.
- Bestehende ui_a11y-/ui_sentinel-/ui_version-Tests bleiben grün; nötige Anpassungen nur MIT Kommentar.
- Verhaltensgleichheit für Nicht-Tipp-Nutzung: einzelner Request ohne Folge-Eingabe rendert exakt wie bisher.
- CHANGELOG unter `## [Unreleased]`; Conventional Commits.
- KEIN `git add -A`/`git add .` — nur benannte Dateien stagen (unversionierte lokale Nutzer-Dateien im Repo).

---

### Task 1: app.js-Umbau + Marker-Tests + CHANGELOG

**Files:**
- Modify: `internal/adapters/http/assets/app.js`
- Create: `internal/adapters/http/ui_requests_internal_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: bestehendes `api(path, init)` (app.js:209-225; `init`-Keys werden nach `opts` kopiert, `signal` fließt also bereits durch), `suggestSeq`/`runSuggest`/`scheduleSuggest` (app.js:420-456), Match-Handler (~app.js:670), Translate-Handler (~app.js:787).
- Produces: nichts Paket-externes.

- [ ] **Step 1: Fehlschlagende Marker-Tests schreiben**

`internal/adapters/http/ui_requests_internal_test.go`, Muster und Asset-Zugriff exakt wie `ui_a11y_internal_test.go` (dortige Helper/Fixture-Ladung wiederverwenden — ansehen, wie `a.js` dort befüllt wird):

```go
// TestUIRequestLifecycleProperties pins the request-lifecycle mechanics the
// console depends on (spec 2026-09-05): stale in-flight requests are ABORTED
// (not merely ignored at render time), aborts are never rendered as errors,
// every panel has a stale guard, and an unparseable 2xx body is an error —
// properties a refactor of app.js could silently drop, which no Go test
// would otherwise notice (there is no JS test infrastructure).
func TestUIRequestLifecycleProperties(t *testing.T) {
	a := loadUIAssets(t) // an das tatsächliche Fixture-Muster der Nachbartests anpassen

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
```

(Die exakten Marker-Strings müssen zur Implementierung in Step 3 passen — beide zusammen entwickeln; die Marker sind Teil der Spezifikation, nicht bloße Grep-Zufälle: jeder pinnt einen benannten Mechanismus.)

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/adapters/http/ -run TestUIRequestLifecycleProperties -v`
Expected: FAIL (Marker existieren noch nicht).

- [ ] **Step 3: app.js implementieren**

Im Stil der Datei (var/function, deutsche Kommentare, keine Klassen):

**3a. `api()` (Zeile 209-225):** Reject-Handler klassifiziert Abort:
```js
    }, function (err) {
      if (err && err.name === "AbortError") {
        // Bewusst KEIN Fehlerobjekt: ein Abbruch ist die Folge einer neueren
        // Eingabe, nie ein Zustand, den der Nutzer sehen soll.
        return { aborted: true, ok: false, status: 0, body: null, raw: "", ms: Math.round(performance.now() - started) };
      }
      return { ok: false, status: 0, body: null, raw: String(err), ms: Math.round(performance.now() - started) };
    });
```

**3b. Suggest (Zeile ~415-456):** modulweiter Controller neben `suggestSeq`/`suggestTimer`:
```js
  var suggestAbort = null;

  function abortSuggest() {
    if (suggestAbort !== null) { suggestAbort.abort(); suggestAbort = null; }
  }
```
`scheduleSuggest`: zuerst `abortSuggest()` (Abbruch beim TASTENDRUCK, nicht erst beim Timer — die eine SQLite-Verbindung wird sofort frei), dann `clearTimeout`, dann `setTimeout(runSuggest, 250)`.
`runSuggest`, Leer-Query-Pfad: vor dem Return
```js
      suggestSeq += 1; // Leer-Query verwirft auch jede noch offene Antwort
```
Nicht-leerer Pfad: `suggestAbort = new AbortController();` und `api(path, { signal: suggestAbort.signal })`; im then-Handler als ERSTE Zeile `if (res.aborted) { return; }`, danach der bestehende seq-Guard.

**3c. Match-Panel (~Zeile 670) und Translate-Panel (~Zeile 787):** je
```js
  var matchSeq = 0;
  var matchAbort = null;
```
Klick-Handler: laufenden Controller aborten, neuen erzeugen, `matchSeq += 1` merken, `signal` durchreichen; then-Handler: `if (res.aborted) { return; }` + `if (seq !== matchSeq) { return; }`. Translate identisch (`translateSeq`/`translateAbort`).

**3d. Alle übrigen `api()`-then-Handler** (areas-Loader, Konzept-Panel/Promise.all, Synonyme): `if (res.aborted) { return; }` als erste Zeile ergänzen — sie bekommen zwar heute kein Signal, aber der Guard macht die Regel „Abort ist nie ein Fehler" flächendeckend und zukunftssicher. Bei `Promise.all` genügt der Guard im gemeinsamen then.

**3e. Kaputtes JSON:** in den Render-Einstiegen von Suggest/Match/Translate/Konzept/Synonyme die Bedingung `if (!res.ok)` zu `if (!res.ok || !res.body)` erweitern; `errorBox` liefert dann „Fehler HTTP_<status>: <raw bzw. 'ungültige JSON-Antwort'>" — dazu in `errorBox` den Fallback-Text präzisieren:
```js
    var msg = res.body && res.body.error ? res.body.error.message : (res.raw || "ungültige oder leere JSON-Antwort");
```

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/adapters/http/ -v -run 'TestUI'` — Expected: PASS inkl. aller Bestandstests (ui_a11y, ui_sentinel, ui_version). Danach `go test ./internal/adapters/http/` und `make lint`.

- [ ] **Step 5: CHANGELOG** unter `## [Unreleased]` → `### Fixed`:

```markdown
* **Testkonsole:** Beim Tippen im Suggest wird der laufende Request jetzt
  abgebrochen (AbortController), statt jede Eingabepause als vollen Query
  auf der einzigen SQLite-Verbindung auslaufen zu lassen — der Tipp-Stau,
  den ein Reverse-Proxy vor hostus als 502 quittierte, entfällt clientseitig;
  Debounce 150 ms → 250 ms. Zusätzlich: Match- und Translate-Panel verwerfen
  veraltete Antworten jetzt per Sequenz-Guard (vorher gewann die zuletzt
  EINTREFFENDE statt der zuletzt gesendeten Antwort), eine geleerte
  Suggest-Eingabe verwirft noch offene Antworten, und eine 2xx-Antwort mit
  ungültigem JSON wird als Fehler statt als „Keine Treffer." angezeigt.
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/assets/app.js internal/adapters/http/ui_requests_internal_test.go CHANGELOG.md
git commit -m "fix(ui): Suggest bricht veraltete Requests ab; Stale-Guards für Match/Translate"
```

---

## Verifikation nach Abschluss (Controller, kein Task)

1. `make verify`; `make mutation PKG=./internal/adapters/http`.
2. Browser-E2E gegen laufenden Server (Produktions-DB): schnelles Tippen in Suggest → Netzwerk-Log zeigt abgebrochene Requests, Ergebnis entspricht dem letzten Query; Feld leeren während Request läuft → Panel bleibt leer; Match zweimal schnell klicken → letzte gesendete Antwort gewinnt.
