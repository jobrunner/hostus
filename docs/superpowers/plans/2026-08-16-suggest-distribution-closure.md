# distribution_effective Closure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `GET /v1/suggest?...&area=<code>` fast AND fully correct for all areas by precomputing the deterministic `in_area` name-fallback into a derived `distribution_effective` table, so runtime `in_area` is a simple indexed lookup.

**Architecture:** New derived table `distribution_effective` (own distribution ∪ resolved CDM name-fallback), built at the end of the ingest command and self-healed on `Open`. Suggest's `in_area` and its in-area recall union both read this table; the runtime name-fallback subquery and its `|| ''` planner hacks are removed.

**Tech Stack:** Go 1.26.6, modernc.org/sqlite, embedded schema.sql applied idempotently on Open.

Design spec: `docs/superpowers/specs/2026-08-15-suggest-distribution-closure-design.md`.

## Global Constraints

- Go **1.26.6** (flake-pinned); allowed libraries only (stdlib, mux, viper, cobra, modernc.org/sqlite, OTel, prometheus).
- **Zero** `//nolint` / `#nosec` (debt-guard). Dynamic SQL only via literal-format `fmt.Sprintf`/bound `?` params, never tainted concatenation (gosec G201/G202).
- `make verify` green (fmt/vet/lint/test/arch/debt/poc); sqlite mutation gate `Not covered = 0`.
- Real data only; never commit bulk data. Validate on `full.sqlite` in the session scratchpad.
- CHANGELOG changes under `## [Unreleased]`; never hand-edit VERSION.
- Feature branch `feature/suggest-short-prefix-speed` (continues the existing PR).

---

### Task 1: Schema — `distribution_effective` table + index

**Files:**
- Modify: `internal/adapters/sqlite/schema.sql` (after the `distribution` / `idx_distribution_area` block)

**Interfaces:**
- Produces: table `distribution_effective(concept_id, area_scheme, area_code, origin)` PK `(concept_id, area_scheme, area_code)`; index `idx_distribution_effective_area(area_scheme, area_code)`.

- [ ] **Step 1: Add the DDL (idempotent)**

```sql
-- Derived: the EFFECTIVE distribution per concept = own distribution, OR — for a
-- concept with none of its own (CDM sec. concepts) — the areas of any WCVP
-- concept sharing its accepted canonical_fold (the in_area name fallback,
-- precomputed). Lets Suggest resolve in_area as an indexed point lookup instead
-- of a per-row correlated name-fallback. Rebuilt by BuildDistributionClosure
-- (ingest finalize + Open self-heal); never written directly.
CREATE TABLE IF NOT EXISTS distribution_effective (
  concept_id  TEXT NOT NULL REFERENCES taxon_concept(id),
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  origin      TEXT NOT NULL,          -- 'own' | 'name'
  PRIMARY KEY (concept_id, area_scheme, area_code)
);
CREATE INDEX IF NOT EXISTS idx_distribution_effective_area
  ON distribution_effective(area_scheme, area_code);
```

- [ ] **Step 2: Verify schema still applies** — `go test -run TestOpen ./internal/adapters/sqlite/` (or any test that opens a DB) passes (schema is valid). No dedicated test; exercised by Task 2.

---

### Task 2: `BuildDistributionClosure` — the builder (TDD)

**Files:**
- Create: `internal/adapters/sqlite/closure.go`
- Test: `internal/adapters/sqlite/closure_internal_test.go`

**Interfaces:**
- Produces: `func (db *DB) BuildDistributionClosure(ctx context.Context) error` — rebuilds `distribution_effective` from scratch (idempotent: DELETE then two `INSERT OR IGNORE` passes).
- Consumes: existing seed helpers `ingestVia`, `species`, `mustTx` (suggest_inarea_internal_test.go) and `openTestDB`.

- [ ] **Step 1: Write the failing test**

```go
func TestBuildDistributionClosure(t *testing.T) {
	db := openTestDB(t)
	// WCVP: accepted "Pentanema hirtum" in GER, synonym "Inula hirta".
	seedWCVPInulaHirta(t, db)
	// CDM: "Inula hirta" (no own dist, twin via name) + "Zzz nowcvp" (no twin).
	seedCDMInulaHirta(t, db)
	if err := db.BuildDistributionClosure(context.Background()); err != nil {
		t.Fatal(err)
	}
	// WCVP own-distribution concept -> 'own' row for GER.
	if got := effRows(t, db, "wcvp:concept:pentanema-hirtum"); got != "GER:own" {
		t.Errorf("wcvp pentanema: got %q, want GER:own", got)
	}
	// CDM concept with a WCVP twin in GER -> 'name' row for GER.
	if got := effRows(t, db, "cdm:concept:inula-hirta"); got != "GER:name" {
		t.Errorf("cdm inula-hirta: got %q, want GER:name", got)
	}
	// CDM concept whose name WCVP does not carry -> no rows.
	if got := effRows(t, db, "cdm:concept:zzz-nowcvp"); got != "" {
		t.Errorf("cdm zzz-nowcvp: got %q, want none", got)
	}
}

// effRows returns "CODE:origin,CODE:origin" (sorted) for a concept's
// distribution_effective rows, "" if none.
func effRows(t *testing.T, db *DB, conceptID string) string {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(),
		`SELECT area_code, origin FROM distribution_effective
		 WHERE concept_id = ? ORDER BY area_scheme, area_code`, conceptID)
	mustTx(t, err)
	defer func() { _ = rows.Close() }()
	var parts []string
	for rows.Next() {
		var code, origin string
		mustTx(t, rows.Scan(&code, &origin))
		parts = append(parts, code+":"+origin)
	}
	mustTx(t, rows.Err())
	return strings.Join(parts, ",")
}
```

- [ ] **Step 2: Run it, watch it fail** — `go test -run TestBuildDistributionClosure ./internal/adapters/sqlite/` → FAIL (`BuildDistributionClosure` undefined).

- [ ] **Step 3: Implement `closure.go`**

```go
package sqlite

import "context"

// BuildDistributionClosure (re)builds distribution_effective from scratch: every
// concept's own distribution (origin 'own'), plus — for a concept with NO own
// distribution and a non-empty accepted canonical_fold — the areas of any WCVP
// concept sharing that fold (origin 'name', the precomputed in_area name
// fallback). Idempotent: safe to run repeatedly (ingest finalize + Open
// self-heal). The `wtc.backbone_id = 'wcvp'` join is fine here (batch build, not
// a per-row correlated subquery, so no adverse plan — unlike Suggest).
func (db *DB) BuildDistributionClosure(ctx context.Context) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: closure begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := []string{
		`DELETE FROM distribution_effective`,
		`INSERT OR IGNORE INTO distribution_effective (concept_id, area_scheme, area_code, origin)
		 SELECT concept_id, area_scheme, area_code, 'own' FROM distribution`,
		`INSERT OR IGNORE INTO distribution_effective (concept_id, area_scheme, area_code, origin)
		 SELECT c.id, wd.area_scheme, wd.area_code, 'name'
		 FROM taxon_concept c
		 JOIN name an ON an.id = c.accepted_name
		 JOIN name wn ON wn.canonical_fold = an.canonical_fold
		 JOIN concept_name wcn ON wcn.name_id = wn.id
		 JOIN taxon_concept wtc ON wtc.id = wcn.concept_id AND wtc.backbone_id = 'wcvp'
		 JOIN distribution wd ON wd.concept_id = wtc.id
		 WHERE an.canonical_fold <> ''
		   AND NOT EXISTS (SELECT 1 FROM distribution d0 WHERE d0.concept_id = c.id)`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite: closure build: %w", err)
		}
	}
	return tx.Commit()
}
```
(Add the `fmt` import.)

- [ ] **Step 4: Run tests, watch pass** — `go test -run TestBuildDistributionClosure ./internal/adapters/sqlite/` → PASS.

- [ ] **Step 5: Measure build time on real data** — after Task 4 wiring, rebuild on `full.sqlite` and record the wall time (the ad-hoc estimate query ran >2 min; if the batch build is too slow, add a transient index or drive the 'name' pass from the zero-distribution concept set explicitly and re-measure — the build is one-time, but keep it to low minutes). Document the measured time in the CHANGELOG (Task 7).

- [ ] **Step 6: Commit** — `feat(sqlite): BuildDistributionClosure (own ∪ resolved name-fallback)`

---

### Task 3: Expose on the Repository port + call after all ingest steps

**Files:**
- Modify: `internal/ports/output/repository.go` (add to `Repository`)
- Modify: `cmd/hostus/ingest.go` (`runIngest`, after all ingest steps succeed)

**Interfaces:**
- Produces: `Repository.BuildDistributionClosure(ctx context.Context) error` (already satisfied by `*DB` from Task 2).

- [ ] **Step 1: Add to the `Repository` interface**

```go
	// BuildDistributionClosure (re)builds the derived distribution_effective
	// table. Call once after ALL backbones (incl. CDM) are ingested — it
	// resolves CDM concepts' in_area name fallback against WCVP twins, which
	// must already be present.
	BuildDistributionClosure(ctx context.Context) error
```

- [ ] **Step 2: Run build to confirm the interface is satisfied** — `go build ./...` (compile-checks `*DB` implements it; it does from Task 2).

- [ ] **Step 3: Call it at the end of `runIngest`** — after backbones + CDM + xref + namespace ingest have all completed (locate the final success point in `cmd/hostus/ingest.go`), before returning:

```go
	if err := repo.BuildDistributionClosure(cmd.Context()); err != nil {
		return fmt.Errorf("building distribution closure: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "distribution closure rebuilt")
```

- [ ] **Step 4: Verify** — `go build ./... && go vet ./...`. (End-to-end ingest is exercised manually on real data in Task 6; no unit test for the cobra wiring.)

- [ ] **Step 5: Commit** — `feat(ingest): rebuild distribution closure after all backbones`

---

### Task 4: Self-heal build on `Open` (TDD)

**Files:**
- Modify: `internal/adapters/sqlite/db.go` (`Open`, after the existing migrations)
- Modify: `internal/adapters/sqlite/closure.go` (add `distributionClosureEmpty` guard helper)
- Test: `internal/adapters/sqlite/closure_internal_test.go`

**Interfaces:**
- Consumes: `BuildDistributionClosure` (Task 2).
- Produces: `Open` builds the closure iff `distribution_effective` is empty AND `distribution` is non-empty (pre-C2 DBs self-heal once; already-built DBs are untouched, so serve startup stays cheap).

- [ ] **Step 1: Write the failing test**

```go
func TestOpenSelfHealsDistributionClosure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heal.sqlite")
	db := openTestDBAt(t, path)
	seedWCVPInulaHirta(t, db)
	seedCDMInulaHirta(t, db)
	// Simulate a pre-C2 DB: distribution present, closure empty.
	if _, err := db.sql.ExecContext(context.Background(), `DELETE FROM distribution_effective`); err != nil {
		t.Fatal(err)
	}
	mustTx(t, db.Close())

	// Re-open: self-heal must build the closure.
	db2, err := Open(path)
	mustTx(t, err)
	t.Cleanup(func() { _ = db2.Close() })
	if got := effRows(t, db2, "cdm:concept:inula-hirta"); got != "GER:name" {
		t.Errorf("after self-heal: got %q, want GER:name", got)
	}
}
```
(Confirm `*DB` has a `Close()`; if not, add a thin one wrapping `db.sql.Close()`. `openTestDBAt` already exists in bundle_internal_test.go.)

- [ ] **Step 2: Run it, watch it fail** — closure stays empty after re-Open.

- [ ] **Step 3: Implement the guard + Open hook**

In `closure.go`:
```go
// distributionClosureEmpty reports whether distribution_effective needs a
// self-heal build: it has no rows but distribution does.
func distributionClosureEmpty(ctx context.Context, sqlDB *sql.DB) (bool, error) {
	var effN, distN int
	if err := sqlDB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM distribution_effective),
		(SELECT count(*) FROM distribution)`).Scan(&effN, &distN); err != nil {
		return false, fmt.Errorf("sqlite: closure emptiness check: %w", err)
	}
	return effN == 0 && distN > 0, nil
}
```
In `Open`, after `migrateConceptRelationPK`:
```go
	if empty, err := distributionClosureEmpty(context.Background(), sqlDB); err != nil {
		return nil, err
	} else if empty {
		db := &DB{sql: sqlDB} // or the already-constructed *DB, depending on Open's shape
		if err := db.BuildDistributionClosure(context.Background()); err != nil {
			return nil, err
		}
	}
```
(Adapt to how `Open` constructs `*DB`; call `BuildDistributionClosure` on that instance after the schema+migrations, before returning.)

- [ ] **Step 4: Run tests, watch pass** — `go test -run 'TestOpenSelfHeals|TestBuildDistributionClosure' ./internal/adapters/sqlite/` → PASS.

- [ ] **Step 5: Commit** — `feat(sqlite): self-heal distribution closure on Open`

---

### Task 5: Rewrite Suggest to read `distribution_effective` (TDD)

**Files:**
- Modify: `internal/adapters/sqlite/suggest.go` (`Suggest` query build)
- Modify: `internal/adapters/sqlite/suggest_inarea_internal_test.go`, `suggest_matchpool_internal_test.go` (build the closure after seeding; add the CDM sparse-area recall case)

**Interfaces:**
- `in_area` becomes `EXISTS (SELECT 1 FROM distribution_effective de WHERE de.concept_id = tc.id AND de.area_scheme = 'wgsrpd_l3' AND de.area_code IN (%s))`.
- `in_area_rows` becomes `SELECT DISTINCT fnm.rowid FROM distribution_effective de JOIN fts_name_map fnm ON fnm.concept_id = de.concept_id WHERE de.area_scheme = 'wgsrpd_l3' AND de.area_code IN (%s) AND fnm.rowid IN (SELECT rowid FROM match_rows)`.
- Removed: the name-fallback `EXISTS(... wn/wcn/wtc/wd ...)`, both `|| ''` hacks, the `an.canonical_fold <> ''` guard, and the `NOT EXISTS(d0)` branch — all folded into the closure.
- Arg order (area case) becomes: `match, suggestMatchPool, match (match_rows), codeArgs (in_area_rows), codeArgs (in_area EXISTS)`, then rank, then budget. (Only ONE `in_area` code group now, not two.)

- [ ] **Step 1: Update the in_area tests to build the closure + add the CDM recall case.** In `suggestInArea`/`suggestInAreaCount`/`suggestCount` helpers (or the seed helpers), call `mustTx(t, db.BuildDistributionClosure(context.Background()))` after seeding and before `Suggest`. Add:

```go
// A CDM concept in a SPARSE area, in-area ONLY via its WCVP name twin (no own
// distribution), with poor bm25, must still surface — the closure must feed the
// recall union. Regression the pure bm25 pool caused (pa×PHX dropped CDM taxa).
func TestSuggest_InAreaCDMTwinInSparseArea(t *testing.T) {
	db := openTestDB(t)
	seedSparseCDMTwin(t, db) // WCVP "Foo bar" in area "ZZ", CDM "Foo bar" no own dist
	mustTx(t, db.BuildDistributionClosure(context.Background()))
	orig := suggestMatchPool
	t.Cleanup(func() { suggestMatchPool = orig })
	suggestMatchPool = 1
	if !suggestInArea(t, db, "Foo", "cdm:concept:foo-bar") {
		t.Error("CDM Foo bar in ZZ via twin: not surfaced/in_area=false under a tiny pool — closure recall union broken")
	}
}
```
Write `seedSparseCDMTwin` (mirror `seedTwoInAreaOneOut`, one WCVP with own dist in "ZZ" + one CDM same name, no own dist).

- [ ] **Step 2: Run — watch the NEW test fail** (Suggest still uses the old name-fallback; but crucially it should fail only because the query hasn't switched to the closure recall union — confirm it is RED before the rewrite, or GREEN-for-wrong-reason; if the old fallback already makes it pass, note that and rely on the pool=1 recall union assertion which the old code cannot satisfy for a CDM concept).

- [ ] **Step 3: Rewrite the query** — replace `inAreaExpr` and the `in_area_rows` CTE per the Interfaces above; delete the name-fallback subquery, `|| ''` hacks, `canonical_fold <> ''` guard; fix `args` to append `codeArgs` for in_area_rows once and for the in_area EXISTS once (two groups total in the area case, not three). Keep the bm25 pool + union structure and `idx_distribution_area`→now `idx_distribution_effective_area` for in_area_rows.

- [ ] **Step 4: Run the full suggest test set** — `go test -run TestSuggest ./internal/adapters/sqlite/` → all PASS (existing in_area equivalence + matchpool recall + new CDM-twin case).

- [ ] **Step 5: `make verify` + mutation** — `make verify` green; `make mutation PKG=./internal/adapters/sqlite` → `Not covered = 0`.

- [ ] **Step 6: Commit** — `perf(suggest): resolve in_area via distribution_effective (fast + fully correct)`

---

### Task 6: Real-data verification + CHANGELOG

**Files:**
- Modify: `CHANGELOG.md` (`## [Unreleased]`)

- [ ] **Step 1: Rebuild closure on full.sqlite** — restart serve on `full.sqlite` (Open self-heals) OR run `hostus ingest`; confirm `distribution_effective` populated (`SELECT count(*), count(*) FILTER (WHERE origin='name')`).
- [ ] **Step 2: Measure** — `suggest?q=ca&area=GER`, `ca&area=PHX`, `pa&area=PHX` warm timings; confirm `pa&area=PHX` now returns the previously-dropped CDM taxa (Panicum dactylon etc.), i.e. in-area recall == unbounded. Record numbers + closure build time.
- [ ] **Step 3: CHANGELOG** — replace the interim `Changed (Suggest…)` entry with the closure-based description (fast + fully correct all areas; runtime name-fallback removed; closure built at ingest + self-heal on Open; measured timings/recall/build time).
- [ ] **Step 4: Commit** — `docs(changelog): distribution_effective closure`.

---

### Task 7 (follow-up, SEPARATE PR after merge): `suggestMatchPool` tuning

Now that in-area recall is guaranteed by the closure union, the bm25 pool only governs non-in-area relevance fill (~fetchBudget rows needed). Measure shrinking `suggestMatchPool` (e.g. 5000 → 1000 → 500): confirm no top-`limit` change for representative dense/sparse queries, pick the smallest value that holds, record the measurement. Own branch, own PR — not part of this plan's PR.

---

## Self-Review

- **Spec coverage:** table+index (T1), closure builder own∪name (T2), ingest hook (T3), Open self-heal (T4), runtime query switch + hack removal (T5), real-data verify + CHANGELOG (T6), pool tuning follow-up (T7) — all spec sections covered.
- **Placeholder scan:** none; SQL/Go given inline.
- **Type consistency:** `BuildDistributionClosure(ctx) error` used identically in port (T3), impl (T2), Open (T4), tests. `effRows`/`distributionClosureEmpty` defined once. `distribution_effective` columns consistent across schema/builder/queries.
- **Ambiguity:** the exact `Open` `*DB` construction and the `runIngest` final success point are localized "adapt to existing shape" notes, not logic gaps.
