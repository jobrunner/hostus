# Spec: Suggest-Planner-Fix (+backbone_id) und Repository-Tracing

Datum: 2026-09-05
Status: verabschiedet (User: „Ja, setze den Fix bitte um. Außerdem schließe bitte die Tracing-Lücken in einem weiteren Commit. Passe bitte auch den Kommentar-Nebenbefund.")

## Kontext

1. **502 auf der Synology-Instanz:** `GET /v1/suggest?q=Inula hirta&area=GER&
   entry_backbone=wcvp&target_space=eurosl` läuft nach exakt ~30 s in die
   Timeout-Middleware; der Reverse-Proxy macht daraus 502. Lokal reproduziert
   (7,8 s auf schnellem Gerät). Isoliert: `entry_backbone=wcvp` allein ist
   der Treiber; die Zeit skaliert mit der Backbone-Größe (wcvp 440k → 7,8 s,
   cdm 51k → 0,4 s, germansl → 17 ms).
   **Root Cause (EXPLAIN QUERY PLAN, real bewiesen):** Bei
   `tc.backbone_id = ?` kippt der SQLite-Planner die Join-Reihenfolge auf
   `SEARCH tc USING INDEX idx_taxon_concept_backbone_id` — er scannt ALLE
   Konzepte des Backbones und führt die korrelierte name_start-LIKE-EXISTS-
   Subquery pro Konzept aus, statt von den ~40 FTS-Treffern zu treiben.
   Gemessener Fix: unäres `+` am Term (`+tc.backbone_id = ?`, das
   dokumentierte SQLite-Idiom zum Abschalten der Index-Nutzung für genau
   einen Term): **6,96 s → 0,0023 s bei identischem Ergebnis.**
2. **Tracing-Lücke:** Der Debug-MCP (`list_spans`) zeigt für den 7,9-s-Request
   genau EINEN Span (otelmux, `GET /v1/suggest`) — unterhalb der HTTP-Schicht
   existieren keine Spans, die Analyse musste auf EXPLAIN ausweichen.
3. **Kommentar-Nebenbefund:** Der Doc-Kommentar am backboneFilter
   (suggest.go, „entry_backbone=cdm&q=ca … 0.29s") dokumentiert eine
   Vor-Fix-Messung und erklärt NICHT die Planner-Falle.

## Entscheidungen

1. **Fix:** `backboneFilter = " AND +tc.backbone_id = ?"` (suggest.go).
   Doc-Kommentar erweitert um die Planner-Falle (WARUM `+`: Index sähe
   selektiv aus, ist es beim Mehrheits-Backbone nicht; Messwerte 6,96 s →
   2,3 ms) und die 0,29-s-Messung wird auf dem realen Index NEU gemessen
   und aktualisiert.
2. **Regressionstest über den Query-Plan, nicht über Latenz:** Die
   Query-Konstruktion wird als unexportierte, pure Funktion
   `buildSuggestQuery(q string, opts output.SuggestOpts) (query string, args []any, ok bool)`
   aus `Suggest` extrahiert (verhaltensidentisch; `Suggest` ruft sie).
   Interner Test führt `EXPLAIN QUERY PLAN` auf der gebauten Query gegen
   eine geseedete Memory-DB aus und assertet, dass KEINE Planzeile
   `idx_taxon_concept_backbone_id` enthält (die Planwahl ist bei SQLite
   ohne ANALYZE strukturgetrieben, also auch auf kleiner DB
   deterministisch — im Test verifizieren: die Vor-Fix-Form MUSS auf
   derselben DB die Index-Zeile zeigen, sonst ist der Test blind; das als
   Kontroll-Assertion mit der bewusst falschen Query-Variante einbauen).
3. **rankFilter-Prüfung:** `tc.rank` hat keinen Index (schema.sql) — dieselbe
   Falle kann dort heute nicht zuschlagen. Das wird im backboneFilter-
   Kommentar als Ein-Satz-Abgrenzung festgehalten (damit niemand „zur
   Sicherheit" auch dort ein `+` streut oder umgekehrt einen rank-Index
   ergänzt, ohne die Falle zu kennen).
4. **Repository-Tracing (eigener Commit):** Neuer Konstruktor
   `telemetry.TraceRepository(inner output.Repository) output.Repository`
   (Paket `internal/adapters/telemetry`), dahinter ein unexportierter Typ
   `tracedRepository`, der `output.Repository` implementiert, jede Methode
   delegiert und um jeden Aufruf einen Span legt
   (`otel.Tracer("hostus/repository")`, Span-Name `repo.<Methodenname>`;
   bei Fehler grundsätzlich `RecordError` + `SetStatus(codes.Error, ...)`
   — AUSSER `context.Canceled`/`context.DeadlineExceeded`: ein Client-Abort
   ist keine Repository-Fehlfunktion (dieselbe Politik wie
   `middleware/loadshed.go`s `recordResponse`) und bekommt stattdessen nur
   ein `canceled=true`-Attribut, keinen Error-Status, kein RecordError.
   `SetStatus`s Beschreibungstext ist bei einem echten Fehler bewusst fest
   (`"repository error"`), NIE `err.Error()` — Adapterfehler können die
   verbatim Nutzereingabe tragen (z. B. sqlites `"suggest %q: ..."`), und
   die Span-Status-Beschreibung ist nicht der Ort dafür; `RecordError`
   selbst bleibt mit dem vollen Fehlertext, da der MemoryExporter lokal ist
   und der Fehlertext fürs Debugging wertvoll bleibt). Keine Query-Texte/
   Parameter als eigene Attribute (kein PII-/Kardinalitätsrisiko, Spans
   bleiben schlank); einzig vertretbares Attribut in v1: `canceled`.
   Verdrahtung in `app.openRepo`: das sqlite-Repo wird VOR der Rückgabe
   gewrappt — damit erscheinen die Kind-Spans automatisch im
   MemoryExporter/Debug-MCP (gleicher TracerProvider wie otelmux) und in
   jedem OTLP-Export. Voraussetzung: `telemetry.Setup` (installiert den
   TracerProvider) muss VOR `openRepo` laufen, da `otel.Tracer(...)` an
   den zur Konstruktionszeit aktuellen Provider bindet — `app.New` hält
   diese Reihenfolge bereits ein.
5. **Abgrenzung:** Kein Span pro SQL-Statement (das wäre ein
   driver-level Interceptor — Folgearbeit, falls Methoden-Granularität
   nicht reicht); keine Spans in Ingest-Pfaden (Batch-Läufe würden
   Millionen Spans erzeugen — der Decorator wird NUR im Serve-Wiring
   eingehängt).

## Verifikation (Controller)

- Synology-Query-Form lokal < 50 ms; `entry_backbone=cdm&q=ca` neu messen
  (Kommentarwert).
- Debug-MCP `get_trace` zeigt für einen Suggest-Request jetzt Kind-Spans
  (`repo.Suggest`, ggf. `repo.NameSpaces` …) unter dem HTTP-Span.
- Gates: verify; mutation lokal für `internal/adapters/sqlite` und
  `internal/adapters/telemetry` (beide gremlins-heavy) + `internal/app`.

## Projektweite Anforderungen

- `Suggest`-Verhalten byte-identisch (nur Planwahl ändert sich);
  bestehende Suggest-Tests unverändert grün.
- Hexagon: Decorator lebt im telemetry-Adapter, Port unangetastet.
- CHANGELOG unter `## [Unreleased]` (Fixed: Planner-Falle; Added:
  Repository-Spans); Conventional Commits; englische WHY-Doc-Kommentare
  mit den Messwerten dieser Spec.
