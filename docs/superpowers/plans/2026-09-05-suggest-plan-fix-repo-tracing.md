# Suggest-Planner-Fix + Repository-Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Der Suggest-Backbone-Filter zwingt den SQLite-Planner nicht mehr in den Backbone-Scan (6,96 s → 2,3 ms, Synology-502 behoben), der Plan ist per EXPLAIN-Test gepinnt, und jeder Repository-Aufruf des Serve-Pfads erzeugt einen Kind-Span (Debug-MCP/OTLP).

**Architecture:** Ein-Term-Fix (`+tc.backbone_id`) in `internal/adapters/sqlite/suggest.go` mit extrahiertem, testbarem Query-Builder; Tracing als `output.Repository`-Decorator im telemetry-Adapter, verdrahtet ausschließlich in `app.openRepo`.

**Tech Stack:** Go 1.26, modernc.org/sqlite, OTel SDK (`go.opentelemetry.io/otel`, bereits sanktioniert).

**Spec:** `docs/superpowers/specs/2026-09-05-suggest-plan-fix-repo-tracing.md`

## Global Constraints

- `Suggest`-Ergebnisse byte-identisch (nur Planwahl ändert sich); alle bestehenden Suggest-Tests unverändert grün.
- Der EXPLAIN-Regressionstest braucht die KONTROLL-Assertion (Vor-Fix-Form zeigt die Index-Zeile auf derselben DB) — ohne sie ist er blind.
- Tracing-Decorator NUR im Serve-Wiring (`app.openRepo`); Ingest/Bundle/Export bleiben unverdrahtet. Keine Query-Texte/Parameter als Span-Attribute.
- `internal/adapters/sqlite` und `internal/adapters/telemetry` sind gremlins-HEAVY (lokale Läufe = Controller-Verifikation); `internal/app` normales Gate. Tests trotzdem mutanten-tauglich.
- CHANGELOG unter `## [Unreleased]`; ZWEI Commits (Fix, Tracing) wie vom User verlangt; Conventional Commits; englische WHY-Doc-Kommentare mit Spec-Messwerten.
- KEIN `git add -A`/`git add .`.

---

### Task 1: Planner-Fix + Query-Builder-Extraktion + EXPLAIN-Test + Kommentar

**Files:**
- Modify: `internal/adapters/sqlite/suggest.go` (Zeilen ~116-299: Extraktion `buildSuggestQuery`, Fix Zeile ~226, Kommentar ~204-228)
- Create: `internal/adapters/sqlite/suggest_plan_internal_test.go`
- Modify: `CHANGELOG.md` (### Fixed)

**Interfaces:**
- Produces: unexportiert `buildSuggestQuery(q string, opts output.SuggestOpts) (query string, args []any, ok bool)` — exakt die heutige Konstruktion aus `Suggest` (match-Leerprüfung → `ok=false`); `Suggest` ruft sie und führt nur noch QueryContext + Scan aus.

- [ ] **Step 1: Fehlschlagenden Test schreiben**

`suggest_plan_internal_test.go` (internes Paket, wie `db_internal_test.go`):

```go
// TestSuggestQueryPlanDoesNotScanBackboneIndex pins the fix for the
// Synology 502 (spec 2026-09-05): with entry_backbone set, SQLite flipped
// the join order onto idx_taxon_concept_backbone_id — scanning EVERY
// concept of the backbone (440k for wcvp) and running the correlated
// name_start EXISTS per concept: measured 6.96s vs 0.0023s for identical
// results. The unary + on the term disables index use for it and restores
// the FTS-driven order. Plan choice in SQLite (without ANALYZE) is
// structural, so the assertion holds on a small seeded DB too — proven by
// the control assertion below, which shows the UNFIXED form picking the
// index on this very database (without it, this test could go blind if a
// future SQLite version changed planning).
func TestSuggestQueryPlanDoesNotScanBackboneIndex(t *testing.T) {
	db := openSeededSuggestDB(t) // Helper: Memory-/Temp-DB mit >=2 Backbones,
	// je >=1 Konzept+Name+fts-Eintrag; bestehende Seed-Helper des Pakets
	// wiederverwenden (nachschlagen), sonst minimal anlegen.

	opts := output.SuggestOpts{Limit: 10, Backbone: "wcvp"}
	query, args, ok := buildSuggestQuery("inula hirta", opts)
	if !ok {
		t.Fatal("buildSuggestQuery returned ok=false for a valid query")
	}
	plan := explainPlan(t, db, query, args) // Helper: EXPLAIN QUERY PLAN, alle detail-Spalten join'en
	if strings.Contains(plan, "idx_taxon_concept_backbone_id") {
		t.Errorf("suggest plan drives from idx_taxon_concept_backbone_id again (backbone scan, the Synology-502 shape):\n%s", plan)
	}

	// KONTROLLE: die Vor-Fix-Form MUSS den Index wählen, sonst ist die
	// Assertion oben nicht beweiskräftig.
	unfixed := strings.Replace(query, "+tc.backbone_id", "tc.backbone_id", 1)
	if unfixed == query {
		t.Fatal("fixed query does not contain +tc.backbone_id — fix missing or renamed")
	}
	controlPlan := explainPlan(t, db, unfixed, args)
	if !strings.Contains(controlPlan, "idx_taxon_concept_backbone_id") {
		t.Fatalf("control (unfixed) plan does not use the backbone index — the assertion above proves nothing on this database:\n%s", controlPlan)
	}
}
```

`explainPlan`: `db.sql.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)`, Spalten (id, parent, notused, detail) scannen, alle `detail` mit "\n" joinen.

- [ ] **Step 2: Fehlschlag verifizieren** — `go test ./internal/adapters/sqlite/ -run TestSuggestQueryPlan -v` → FAIL (buildSuggestQuery existiert nicht).

- [ ] **Step 3: Implementierung**
  - `buildSuggestQuery` extrahieren: gesamte Konstruktion (match, args, cteClause, inAreaExpr, rankFilter, backboneFilter, nameStartFilter, LIMIT-Budget) verhaltensidentisch verschieben; `Suggest` behält QueryContext/Scan/attachTargetSpaceNames.
  - Fix: `backboneFilter = " AND +tc.backbone_id = ?"`.
  - Kommentar (~204-228) erweitern: (a) Planner-Falle + `+`-Idiom + Messwerte (6,96 s → 2,3 ms auf dem realen Index, „Inula hirta", wcvp); (b) die 0,29-s-cdm-Messung durch eine NEUE Messung auf dem realen Index ersetzen (Controller liefert den Wert vor dem Commit — im Working-Ledger nachfragen bzw. der Koordinator trägt ihn in die Fix-Runde ein; bis dahin Platzhalter NICHT committen, sondern selbst messen, falls die Produktions-DB `out/hostus-deploy-v3.1.0-alpha.0.sqlite` lesbar ist: gleiche curl-Form gegen lokalen Serve); (c) Ein-Satz-Abgrenzung rankFilter: `tc.rank` hat keinen Index (schema.sql), die Falle existiert dort heute nicht — wer einen rank-Index ergänzt, muss diesen Kommentar lesen.
  - CHANGELOG (### Fixed): Suggest mit `entry_backbone` auf großem Backbone lief in den Backbone-Scan (Synology: 30-s-Timeout → Proxy-502); Planner-Idiom-Fix, Messwerte.

- [ ] **Step 4: Tests** — `go test ./internal/adapters/sqlite/` (alle Suggest-Bestandstests!), `go test ./...`, `make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/sqlite/suggest.go internal/adapters/sqlite/suggest_plan_internal_test.go CHANGELOG.md
git commit -m "fix(suggest): Backbone-Filter zwingt Planner nicht mehr in den Backbone-Scan"
```

---

### Task 2: Repository-Tracing-Decorator + Serve-Wiring + CHANGELOG

**Files:**
- Create: `internal/adapters/telemetry/tracedrepo.go`, `internal/adapters/telemetry/tracedrepo_test.go`
- Modify: `internal/app/app.go` (openRepo wrappt), `CHANGELOG.md` (### Added)

**Interfaces:**
- Produces: `func TraceRepository(inner output.Repository) output.Repository` (Konstruktor; Tracer intern via `otel.Tracer("hostus/repository")`).
- Consumes: `output.Repository` (alle ~25 ctx-Methoden — Signaturen aus `internal/ports/output/repository.go` übernehmen, NICHT raten).

- [ ] **Step 1: Fehlschlagende Tests schreiben**

`tracedrepo_test.go` (externes Testpaket; OTel-`sdktrace` + `tracetest.NewSpanRecorder` — sdk ist bereits Dependency):

```go
// TestTraceRepository_SpansAndErrorStatus pins the debug-MCP gap found on
// 2026-09-05: a 7.9s /v1/suggest trace contained ONE span (the otelmux
// HTTP span) and nothing below — the repository layer was invisible, and
// the slow-query analysis had to fall back to EXPLAIN. Every repository
// call must emit a child span named repo.<Method>; an error must be
// recorded and set the span status to Error.
func TestTraceRepository_SpansAndErrorStatus(t *testing.T) {
	// SpanRecorder + TracerProvider als global otel.SetTracerProvider für
	// den Test (und danach zurücksetzen), fakeRepo mit steuerbarem Fehler.
	// Fälle:
	//  1. Suggest ohne Fehler -> genau 1 Span "repo.Suggest", Status Unset/Ok.
	//  2. MatchExact mit Fehler -> Span "repo.MatchExact", Status=Error,
	//     RecordError-Event vorhanden, Fehler wird UNVERÄNDERT durchgereicht.
	//  3. Rückgabewerte werden 1:1 durchgereicht (Kandidatenliste identisch).
}

// TestTraceRepository_CoversEveryPortMethod pins completeness WITHOUT
// listing methods by hand (a new port method must not silently bypass
// tracing): reflect over output.Repository's method set, invoke each via
// reflection against a permissive fake, and assert one span per method
// with the repo.<Name> naming scheme.
func TestTraceRepository_CoversEveryPortMethod(t *testing.T) {
	// reflection-basiert; fake liefert Zero-Values. Assertion:
	// len(recorder.Ended()) == NumMethod und jeder Span-Name == "repo."+Method.
}
```

(Der Reflection-Test ist der Mutations-Schutz gegen „Methode delegiert ohne Span"; wenn einzelne Methoden ohne ctx existieren, im Test explizit ausnehmen und begründen.)

- [ ] **Step 2: Fehlschlag verifizieren** — `go test ./internal/adapters/telemetry/ -run TestTraceRepository -v` → FAIL.

- [ ] **Step 3: Implementierung**
  - `tracedrepo.go`: Struct `tracedRepository{inner output.Repository; tracer trace.Tracer}`; pro Port-Methode:
    ```go
    func (r *tracedRepository) Suggest(ctx context.Context, q string, opts output.SuggestOpts) ([]domain.SuggestItem, error) {
        ctx, span := r.tracer.Start(ctx, "repo.Suggest")
        defer span.End()
        out, err := r.inner.Suggest(ctx, q, opts)
        if err != nil {
            span.RecordError(err)
            span.SetStatus(codes.Error, err.Error())
        }
        return out, err
    }
    ```
    Datei-Kopf-Doc: WARUM (MCP-Trace-Befund, Spec-Referenz), warum KEINE Query-Attribute (PII/Kardinalität), warum nur Serve.
  - `app.go` openRepo: `return telemetry.TraceRepository(db), closer` (nur dort; Doc-Satz ergänzen).
  - CHANGELOG (### Added): Repository-Spans (`repo.<Methode>`) unter dem HTTP-Span — sichtbar im Debug-MCP (`get_trace`) und jedem OTLP-Export; Ingest-Pfade bewusst unverdrahtet.

- [ ] **Step 4: Tests** — `go test ./internal/adapters/telemetry/ ./internal/app/`, `go test ./...`, `make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/telemetry/tracedrepo.go internal/adapters/telemetry/tracedrepo_test.go internal/app/app.go CHANGELOG.md
git commit -m "feat(telemetry): Span pro Repository-Aufruf auf dem Serve-Pfad"
```

---

## Verifikation nach Abschluss (Controller, kein Task)

1. `make verify`; `make mutation` lokal: `./internal/adapters/sqlite`, `./internal/adapters/telemetry`, `./internal/app`.
2. E2E auf Produktions-DB: Synology-Query-Form < 50 ms; `entry_backbone=cdm&q=ca` neu messen und den Kommentarwert aus Task 1 gegenprüfen.
3. Debug-MCP: `get_trace` eines Suggest-Requests zeigt `repo.Suggest`-Kind-Span (+ ggf. weitere) unter `GET /v1/suggest`.
