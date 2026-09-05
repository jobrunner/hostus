# Spec: Konsole — Request-Abbruch beim Tippen und Stale-Guards (Paket A)

Datum: 2026-09-05
Status: verabschiedet (Frontend-Review 2026-09-05; User: „Schreibe bitte zwei Pläne für a und b und setze sie hintereinander um")

## Kontext

Der Suggest der Testkonsole (assets/app.js) debounced mit 150 ms und
verwirft veraltete Antworten nur beim RENDERN (`suggestSeq`); die Requests
selbst laufen serverseitig immer voll durch — es gibt im gesamten Frontend
keinen `AbortController`. Da der Serve-Pfad auf EINER SQLite-Verbindung
läuft (`SetMaxOpenConns(1)`, siehe Paket-B-Spec), staut jeder Tipp-Puls die
Folge-Requests; der Reverse-Proxy vor hostus quittiert den Stau mit 502.
Verifiziert: `r.Context()` fließt bis `QueryContext` — ein Client-Abort gibt
die Verbindung sofort frei, der Abbruch entlastet den Server real.

Der Frontend-Review fand zusätzlich drei bestätigte Korrektheitsfehler:
Match- und Translate-Panel haben KEINE Stale-Guards (die zuletzt
eintreffende, nicht die zuletzt gesendete Antwort gewinnt), und der
Leer-Query-Pfad von `runSuggest` erhöht `suggestSeq` nicht (eine alte
Antwort füllt das gerade geleerte Panel nachträglich). Plus: eine
2xx-Antwort mit kaputtem JSON wird als „Keine Treffer" statt als Fehler
gerendert.

## Entscheidungen

1. **AbortController für Suggest:** Ein modulweiter Controller; bereits in
   `scheduleSuggest` (also beim Tastendruck, nicht erst beim Timer-Feuern)
   wird ein laufender Suggest-Request abgebrochen. `runSuggest` erzeugt pro
   Request einen frischen Controller und reicht `signal` durch `api()`
   (dessen `init`-Merge das Feld bereits transportiert).
2. **Abort ist kein Fehler:** `api()` klassifiziert eine Abort-Rejection
   (`err.name === "AbortError"`) als `{aborted: true, ok: false, ...}`;
   JEDER Render-Pfad, der `api()` konsumiert, kehrt bei `res.aborted`
   kommentarlos zurück (kein errorBox, kein Log). Ohne diese Regel würde
   der Abbruch selbst als „Fehler HTTP_0: AbortError" gerendert.
3. **Debounce 150 ms → 250 ms** (weniger Requests bei gleichem Tippgefühl;
   der Seq-Guard bleibt als zweite Verteidigung bestehen).
4. **Leer-Query-Fix:** Der Early-Return für leere Queries in `runSuggest`
   erhöht `suggestSeq` (und bricht via Controller ab), sodass keine alte
   Antwort ein geleertes Panel füllt.
5. **Stale-Guards + Abort für Match und Translate:** analog `suggestSeq`
   je ein Sequenz-Zähler; ein erneuter Klick bricht den laufenden Request
   des Panels ab. (Konzept-Panel hat bereits `currentConceptID` als Guard.)
6. **JSON-Parse-Fehler sind Fehler:** Render-Pfade behandeln
   `res.ok && !res.body` als Fehlerfall (errorBox mit Hinweis auf
   ungültige JSON-Antwort), nicht als leeres Ergebnis.
7. **Regressionsschutz im Stil des Repos:** Die Mechanismen werden — wie
   die bestehenden a11y-Eigenschaften — als String-Marker-Tests auf den
   eingebetteten Assets gepinnt (`ui_*_internal_test.go`-Muster,
   `strings.Contains(a.js, ...)`), da es keine JS-Testinfrastruktur gibt.
   End-to-End-Verifikation (Tippen bricht Requests wirklich ab) macht der
   Controller per Browser gegen einen laufenden Server.

## Nicht in diesem Paket

Serverseitige Ursachen (Ein-Verbindungs-Pool, toter Load-Shedder) — das ist
Paket B (`2026-09-05-serve-read-pool-loadshed.md`).

## Projektweite Anforderungen

- `internal/adapters/http` ist mutation-gated (die Marker-Tests liegen dort).
- Bestehende ui_a11y-/ui_sentinel-Tests dürfen nicht brechen; hash-/
  marker-pinnende Bestandstests werden, wo nötig, MIT Kommentar angepasst.
- CHANGELOG unter `## [Unreleased]`; Conventional Commits; JS bleibt
  framework-frei im Stil der Datei (var, function, deutsche UI-Kommentare —
  app.js kommentiert bereits deutsch, dieser Stil bleibt).
