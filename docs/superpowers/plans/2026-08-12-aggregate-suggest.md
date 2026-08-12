# Marker-insensitive Aggregate Suggest — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans / subagent-driven-development to implement task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Typing any aggregate spelling (`agg./aggr./s.l.`) in suggest finds the taxon, and the hit is flagged as an aggregate.

**Architecture:** (1) strip the aggregate marker from the suggest FTS query; (2) index resolved aggregate name_space_entry names into fts with an `is_aggregate` flag; (3) surface `SuggestItem.aggregate` through the DTO, OpenAPI and the console badge.

**Tech Stack:** Go 1.26, modernc.org/sqlite FTS5, gorilla/mux. Design: `docs/superpowers/specs/2026-08-12-aggregate-suggest-design.md`.

## Global Constraints
- No new dependency. Reuse `domain.aggregateMarkers`/`AggregateBases`.
- Zero `//nolint` / `#nosec` budget; gosec G304 → `filepath.Clean`.
- TDD: failing test first, watched to fail, minimal code, mutation `Not covered = 0`.
- OpenAPI: `SuggestItem` schema must gain the field (the schema-content contract test enforces it).

---

### Task 1: `domain.StripAggregateMarkers`

**Files:** Modify `internal/domain/normalize.go`; Test `internal/domain/normalize_test.go`.

**Interfaces:**
- Produces: `func StripAggregateMarkers(canon string) string` — the fully-stripped base (last element of `AggregateBases`), or `canon` unchanged when no marker.

- [ ] **Step 1: failing test** — table over `"achillea millefolium aggr."→"achillea millefolium"`, `"… agg."`, `"… s.l."`, `"… s. l."` (spaced), layered `"… aggr. s. l."→"…"`, no-marker `"achillea millefolium"` unchanged, `"… s. str."` unchanged.
- [ ] **Step 2:** run, expect FAIL (undefined).
- [ ] **Step 3: implement**
```go
// StripAggregateMarkers returns canon with every trailing aggregate marker
// (agg./aggr./s.l./s. l./s.lat./sl.) removed — the fully-stripped base — or
// canon unchanged when it carries none. It reuses AggregateBases so the marker
// set has one definition.
func StripAggregateMarkers(canon string) string {
	bases := AggregateBases(canon)
	if len(bases) == 0 {
		return canon
	}
	return bases[len(bases)-1]
}
```
- [ ] **Step 4:** run, expect PASS.
- [ ] **Step 5:** `make mutation PKG=./internal/domain` → Not covered = 0.
- [ ] **Step 6:** commit `feat(domain): StripAggregateMarkers reusing the aggregate marker set`.

---

### Task 2: normalize the suggest FTS query

**Files:** Modify `internal/adapters/sqlite/suggest.go` (`ftsPrefixToken`); Test `internal/adapters/sqlite/suggest_internal_test.go` (or the suggest test file).

- [ ] **Step 1: failing test** — `ftsPrefixToken("Achillea millefolium agg.")` returns the same token as `ftsPrefixToken("Achillea millefolium")` (i.e. `"achillea millefolium"*`); assert the marker is gone.
- [ ] **Step 2:** run, expect FAIL.
- [ ] **Step 3: implement** — in `ftsPrefixToken`, after `token := domain.Canonicalize(q)`, add `token = domain.StripAggregateMarkers(token)`. Keep the `minQueryRunes` check AFTER stripping.
- [ ] **Step 4:** run, expect PASS; also existing suggest tests stay green.
- [ ] **Step 5:** `make mutation PKG=./internal/adapters/sqlite` → Not covered = 0.
- [ ] **Step 6:** commit `feat(suggest): strip aggregate markers from the FTS query`.

---

### Task 3: schema — `fts_name_map.is_aggregate`

**Files:** Modify `internal/adapters/sqlite/schema.sql`, `internal/adapters/sqlite/db.go` (`expectedSchemaColumns`).

- [ ] **Step 1:** add `is_aggregate INTEGER NOT NULL DEFAULT 0` to the `fts_name_map` CREATE TABLE.
- [ ] **Step 2:** add `is_aggregate` to `expectedSchemaColumns`' `fts_name_map` entry (the schema-drift guard from db.go).
- [ ] **Step 3:** run the sqlite package tests (schema verification on `Open`) → green (default 0 keeps existing inserts valid).
- [ ] **Step 4:** commit `feat(schema): fts_name_map.is_aggregate flag`.

*(No standalone behavior test here — the column is exercised by Tasks 4–5. This task is the schema half of their deliverable and is committed with them if a reviewer would reject it alone; keep separate only if green independently.)*

---

### Task 4: ingest — index aggregate name-space aliases

**Files:** Modify the FTS-build path in `internal/adapters/sqlite/db.go` (the loop that inserts `fts_name`/`fts_name_map` from `concept_name`); Test `internal/adapters/sqlite/*_test.go`.

**Interfaces:**
- Consumes: `name_space_entry(space, ext_id, concept_id, name, aggregate, resolution)`.
- Produces: for each `aggregate=1 AND concept_id<>''` entry, one `fts_name_map(concept_id, is_aggregate=1)` + `fts_name(rowid, canonical=Canonicalize(name), '')`.

- [ ] **Step 1: failing test** — ingest a fixture with an accepted concept and an aggregate `name_space_entry` (aggregate=1, concept_id=that concept); assert `fts_name` contains a row whose canonical == the aggregate name AND its `fts_name_map.is_aggregate = 1`. Add a second entry with empty concept_id → assert it is NOT indexed.
- [ ] **Step 2:** run, expect FAIL.
- [ ] **Step 3: implement** — after the existing `concept_name`→fts loop, add a second loop over `SELECT concept_id, name FROM name_space_entry WHERE aggregate = 1 AND concept_id <> ''`, inserting `fts_name_map (concept_id, is_aggregate) VALUES (?, 1)` then `fts_name (rowid, canonical, vernacular_de) VALUES (?, ?, '')` with `domain.Canonicalize(name)`. Runs in the same ingest tx, after name spaces are populated.
- [ ] **Step 4:** run, expect PASS.
- [ ] **Step 5:** `make mutation PKG=./internal/adapters/sqlite` → Not covered = 0.
- [ ] **Step 6:** commit `feat(ingest): index aggregate name-space aliases into FTS`.

---

### Task 5: suggest query carries `aggregate`; domain.SuggestItem

**Files:** Modify `internal/adapters/sqlite/suggest.go` (SQL + `scanSuggestItem`), `internal/domain/suggest.go` (`SuggestItem`); Test the sqlite suggest test.

**Interfaces:**
- Produces: `domain.SuggestItem.Aggregate bool`.

- [ ] **Step 1: failing test** — ingest nominate concept + aggregate alias; `Suggest("achillea millefolium agg.")` returns the nominate concept with `Aggregate == true`; a plain non-aggregate concept returns `Aggregate == false`.
- [ ] **Step 2:** run, expect FAIL (field/column absent).
- [ ] **Step 3: implement** — add `Aggregate bool` to `domain.SuggestItem`; add `MAX(fnm.is_aggregate) AS aggregate` to the suggest SELECT (in the `GROUP BY tc.id`); extend `scanSuggestItem` to read it (as int→bool). Column order must match the scan order.
- [ ] **Step 4:** run, expect PASS; existing suggest ranking tests stay green.
- [ ] **Step 5:** `make mutation PKG=./internal/adapters/sqlite` + `./internal/domain` → Not covered = 0.
- [ ] **Step 6:** commit `feat(suggest): carry an aggregate flag per concept`.

---

### Task 6: HTTP DTO + OpenAPI + console badge

**Files:** Modify `internal/adapters/http/suggest.go` (`suggestItemDTO` + mapping), `api/openapi/openapi.yaml` (`SuggestItem`), `docs/reference/http-api.md`, `internal/adapters/http/assets/app.js` + `index.html`; Tests: existing http suggest test + the schema-content contract test (auto).

- [ ] **Step 1: failing test** — extend an http suggest test asserting the response item carries `"aggregate": true` when the underlying item is aggregate; the schema-content test (`TestOpenAPISchemasMatchDTOs`) fails until the OpenAPI `SuggestItem` gains the property.
- [ ] **Step 2:** run, expect FAIL.
- [ ] **Step 3: implement**
  - `suggestItemDTO`: add `Aggregate bool \`json:"aggregate,omitempty"\``; set it in `suggestResponseToDTO`.
  - `openapi.yaml` `SuggestItem.properties`: add `aggregate: {type: boolean}` (NOT in `required` — omitempty).
  - `http-api.md`: document the field.
  - `app.js`: in the suggest row render, if `r.aggregate`, append a small badge (e.g. `el("span","badge","agg.")`); `index.html`/`style.css` get a `.badge` rule if needed.
- [ ] **Step 4:** run — http suggest test + `TestOpenAPISchemasMatchDTOs` + `TestRoutesMatchOpenAPISpec` green.
- [ ] **Step 5:** `make mutation PKG=./internal/adapters/http` → Not covered = 0.
- [ ] **Step 6:** commit `feat(http): expose SuggestItem.aggregate + console badge`.

---

### Task 7: gate + serve smoke + PR

- [ ] `make verify` + `make doc-drift` + `make lint --build-tags integration` + `make security-check` green.
- [ ] Serve smoke against the real consolidated DB: `GET /v1/suggest?q=Achillea%20millefolium%20agg.` returns the taxon (and, if an aggregate alias exists in that DB, `aggregate: true`).
- [ ] CHANGELOG under `[Unreleased]`.
- [ ] Open PR, adversarial review, fix findings, re-review, merge; sync master; delete branch.

## Self-Review
- Spec coverage: query-normalization (T1,T2), alias-indexing (T3,T4), flag pass-through (T5,T6), display (T6) — all covered.
- No placeholders: every step has concrete code/paths.
- Type consistency: `StripAggregateMarkers`, `SuggestItem.Aggregate`, `is_aggregate` used consistently across tasks.
