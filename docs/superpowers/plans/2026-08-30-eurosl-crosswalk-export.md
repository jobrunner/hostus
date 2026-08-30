# EuroSL-Crosswalk-Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `hostus export-crosswalk --db <path> --out-dir <dir>`, a new CLI
command that writes two CSVs situs' file-based species ingest needs:
`eurosl_crosswalk.csv` (name→concept_id) and `aggregate_members.csv`
(aggregate→member edges).

**Architecture:** Three new SQL query methods on `*sqlite.DB`
(`internal/adapters/sqlite/crosswalk.go`) feed a composition-root function
`app.ExportCrosswalk` (`internal/app/export_crosswalk.go`) that detects
name collisions between the two crosswalk sources and writes both CSVs via
`encoding/csv`. A cobra command (`cmd/hostus/export_crosswalk.go`) wires
flags to it and prints the report, mirroring `hostus bundle`'s existing
three-layer shape (`cmd` → `app` → `sqlite`) exactly.

**Tech Stack:** Go 1.26, `modernc.org/sqlite`, `github.com/spf13/cobra`,
stdlib `encoding/csv`, stdlib `testing` (no testify in this repo).

**Spec:** `docs/superpowers/specs/2026-08-29-eurosl-crosswalk-export-design.md`

**Branch note:** This plan assumes `feature/eurosl-crosswalk-export` has
been rebased onto `feature/namensraum-klassifikation-aggregat-redesign`
(PR #80, open, base `master`) — confirmed done as of this plan's authoring.
PR #80 is what supplies `concept_aggregate`, the native-eurosl-backbone
Fall-B rows, and the `/v1/concept/{id}` `aggregateMembers` join the spec
references; none of those exist on plain `master`. Rebase onto PR #80's
current tip again before executing this plan if time has passed and PR #80
gained new commits.

**Important discovery for task authors:** the spec claims a Fall-A/Fall-B
name collision "should structurally never happen" (spec §2). Empirically,
against `internal/app/testdata/dataset-agreement.yaml` (this plan's own
test fixture — see Task 2), it happens for BOTH rows the fixture's native
eurosl ingest produces ("Festuca" and "Festuca ovina agg." each get a
Fall-A `name_space_entry` row AND their own Fall-B native concept). The
spec's error-handling table already covers this correctly (count + report,
never resolve automatically) — this plan just treats collision detection
as the everyday path to test, not a contrived edge case, and Task 2's test
asserts on this exact, real fixture data rather than a synthetic one.

## Global Constraints

- No redistribution gate on this export (spec, owner decision 2026-08-29) —
  unlike `hostus bundle`, `ExportCrosswalk` must never check any source's
  `redistribution` value.
- `eurosl_crosswalk.csv` columns: `name,concept_id`. `aggregate_members.csv`
  columns: `aggregate_concept_id,member_concept_id,member_name`. Standard
  comma-delimited CSV (the spec's `name|concept_id` notation is documenting
  column names, not a pipe delimiter).
- A name collision between Fall A and Fall B is counted and reported with
  BOTH concept ids — never silently resolved to one side. Both rows still
  land in `eurosl_crosswalk.csv`.
- `aggregate_members.csv`'s `member_name` is always the member concept's
  ACCEPTED name (`name.canonical` via `taxon_concept.accepted_name`), never
  a verbatim/alternate spelling.
- Missing `--db` or `--out-dir` aborts with an error (same UX as `hostus
  bundle`). An unopenable database aborts with a named error. An empty
  `concept_aggregate` table is not an error — `aggregate_members.csv` gets
  only its header row.
- Deterministic output ordering: every SQL query in this plan carries an
  explicit `ORDER BY`, and `writeCrosswalkCSV` concatenates Fall A then
  Fall B — never merges/re-sorts across sources — so collisions stay
  visible as adjacent-but-distinct rows, not coincidentally interleaved.

---

### Task 1: SQL query layer (`internal/adapters/sqlite`)

**Files:**
- Create: `internal/adapters/sqlite/crosswalk.go`
- Test: `internal/adapters/sqlite/crosswalk_test.go`

**Interfaces:**
- Consumes: `db.sql *sql.DB` (the package's private field, `DB` methods
  only — see `internal/adapters/sqlite/db.go:22-24`), `openTestDB(t)` /
  `seedBackboneVersion` (`internal/adapters/sqlite/db_internal_test.go`),
  `output.IngestTx` methods `UpsertNameSpace`, `AddNameSpaceEntry`,
  `UpsertName`, `UpsertConcept`, `LinkName`, `AddAggregateMember`,
  `Finalize`, `Commit` (`internal/ports/output/repository.go:345-412`).
- Produces (used by Task 2): `sqlite.CrosswalkEntry{Name, ConceptID string}`,
  `sqlite.AggregateMemberRow{AggregateConceptID, MemberConceptID,
  MemberName string}`, `(*sqlite.DB).EuroslCrosswalkEntries(ctx)
  ([]CrosswalkEntry, error)`, `(*sqlite.DB).NativeEuroslConcepts(ctx)
  ([]CrosswalkEntry, error)`, `(*sqlite.DB).AllAggregateMembers(ctx)
  ([]AggregateMemberRow, error)`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/adapters/sqlite/crosswalk_test.go
package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// seedEuroslCrosswalkFixture builds, on top of openSeededDB's WCVP concept
// (corynephorusID), exactly one Fall-A row, one Fall-B native concept, and
// one concept_aggregate edge — enough for all three new query methods to
// each return exactly one row.
func seedEuroslCrosswalkFixture(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{
		ID: "eurosl", Version: "2026-08-27", IngestedAt: "2026-08-27T00:00:00Z",
		ManifestSHA: "deadbeef",
	})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2026-08-27", ManifestSHA: "deadbeef",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	// Fall A: a plain eurosl spelling of the already-seeded WCVP concept.
	if err := tx.AddNameSpaceEntry(corynephorusID, domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e1", Name: "Corynephorus canescens",
	}); err != nil {
		t.Fatalf("AddNameSpaceEntry: unexpected error: %v", err)
	}
	// Fall B: a native eurosl aggregate concept, own name, own id.
	aggName := domain.Name{
		ID: "eurosl:name:agg1", Canonical: "Corynephorus canescens agg.",
		Rank: domain.RankSpeciesAggregate,
	}
	aggConcept := domain.Concept{
		ID: "eurosl:concept:agg1", BackboneID: "eurosl", AcceptedName: aggName,
		Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted,
	}
	if err := tx.UpsertName(aggName); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(aggConcept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(aggConcept.ID, aggName.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	// concept_aggregate edge: the native aggregate's WCVP member.
	if err := tx.AddAggregateMember(aggConcept.ID, corynephorusID); err != nil {
		t.Fatalf("AddAggregateMember: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

func TestEuroslCrosswalkEntries_ReturnsFallARows(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)

	got, err := db.EuroslCrosswalkEntries(context.Background())
	if err != nil {
		t.Fatalf("EuroslCrosswalkEntries: unexpected error: %v", err)
	}
	want := []CrosswalkEntry{{Name: "Corynephorus canescens", ConceptID: corynephorusID}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("EuroslCrosswalkEntries = %+v, want %+v", got, want)
	}
}

func TestEuroslCrosswalkEntries_NoEuroslEntries_ReturnsEmptyNotNil(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.EuroslCrosswalkEntries(context.Background())
	if err != nil {
		t.Fatalf("EuroslCrosswalkEntries: unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("EuroslCrosswalkEntries = %+v, want an empty, non-nil slice", got)
	}
}

func TestNativeEuroslConcepts_ReturnsFallBRows(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)

	got, err := db.NativeEuroslConcepts(context.Background())
	if err != nil {
		t.Fatalf("NativeEuroslConcepts: unexpected error: %v", err)
	}
	want := []CrosswalkEntry{{Name: "Corynephorus canescens agg.", ConceptID: "eurosl:concept:agg1"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("NativeEuroslConcepts = %+v, want %+v", got, want)
	}
}

func TestAllAggregateMembers_ReturnsJoinedMemberName(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)

	got, err := db.AllAggregateMembers(context.Background())
	if err != nil {
		t.Fatalf("AllAggregateMembers: unexpected error: %v", err)
	}
	want := []AggregateMemberRow{{
		AggregateConceptID: "eurosl:concept:agg1",
		MemberConceptID:    corynephorusID,
		MemberName:         "Corynephorus canescens",
	}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("AllAggregateMembers = %+v, want %+v", got, want)
	}
}

func TestAllAggregateMembers_NoAggregates_ReturnsEmptyNotNil(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.AllAggregateMembers(context.Background())
	if err != nil {
		t.Fatalf("AllAggregateMembers: unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("AllAggregateMembers = %+v, want an empty, non-nil slice", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/sqlite/... -run 'TestEuroslCrosswalkEntries|TestNativeEuroslConcepts|TestAllAggregateMembers' -v`
Expected: FAIL — `db.EuroslCrosswalkEntries undefined`, `db.NativeEuroslConcepts undefined`, `db.AllAggregateMembers undefined`, `CrosswalkEntry`/`AggregateMemberRow` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/adapters/sqlite/crosswalk.go
package sqlite

import (
	"context"
	"fmt"
)

// CrosswalkEntry is one name->concept_id row for hostus export-crosswalk's
// eurosl_crosswalk.csv (spec docs/superpowers/specs/2026-08-29-eurosl-
// crosswalk-export-design.md §2) — either Fall A (an eurosl name_space_entry
// spelling of an existing WCVP concept) or Fall B (a native eurosl
// concept's own accepted name).
type CrosswalkEntry struct {
	Name      string
	ConceptID string
}

// EuroslCrosswalkEntries returns Fall A of the eurosl crosswalk: every
// name_space_entry row pinned to space "eurosl", ordered by (name,
// concept_id) for a deterministic export. An eurosl ingest that resolved
// no rows returns an empty, non-error slice.
func (db *DB) EuroslCrosswalkEntries(ctx context.Context) ([]CrosswalkEntry, error) {
	return db.queryCrosswalkEntries(ctx,
		`SELECT name, concept_id FROM name_space_entry WHERE space = 'eurosl' ORDER BY name, concept_id`)
}

// NativeEuroslConcepts returns Fall B of the eurosl crosswalk: every
// taxon_concept native to the eurosl backbone (aggregates, sections,
// families, ...; Task 5/6 of the namensraum redesign), keyed by its OWN
// accepted name, ordered by (name, concept_id) for a deterministic export.
// A database with no eurosl-native ingest returns an empty, non-error
// slice.
func (db *DB) NativeEuroslConcepts(ctx context.Context) ([]CrosswalkEntry, error) {
	return db.queryCrosswalkEntries(ctx, `
		SELECT n.canonical, tc.id FROM taxon_concept tc
		JOIN name n ON n.id = tc.accepted_name
		WHERE tc.backbone_id = 'eurosl'
		ORDER BY n.canonical, tc.id`)
}

// queryCrosswalkEntries is the shared scan loop EuroslCrosswalkEntries/
// NativeEuroslConcepts both run: a fixed two-column query with no
// caller-supplied arguments (both callers pass a literal SELECT), scanned
// into CrosswalkEntry.
func (db *DB) queryCrosswalkEntries(ctx context.Context, query string) ([]CrosswalkEntry, error) {
	rows, err := db.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying eurosl crosswalk entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []CrosswalkEntry{}
	for rows.Next() {
		var e CrosswalkEntry
		if err := rows.Scan(&e.Name, &e.ConceptID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning eurosl crosswalk entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating eurosl crosswalk entries: %w", err)
	}
	return out, nil
}

// AggregateMemberRow is one concept_aggregate edge joined to the member's
// accepted canonical name, for hostus export-crosswalk's
// aggregate_members.csv.
type AggregateMemberRow struct {
	AggregateConceptID string
	MemberConceptID    string
	MemberName         string
}

// AllAggregateMembers returns every concept_aggregate edge across every
// native name space (not eurosl-only — germansl aggregates are included
// too, matching /v1/concept/{id}'s own aggregateMembers join in
// internal/adapters/http/taxa.go), joined to the member's accepted
// canonical name, ordered by (aggregate_concept_id, member_concept_id) for
// a deterministic export. An empty concept_aggregate table (no Fall-B
// ingest ran) returns an empty, non-error slice — never an error.
func (db *DB) AllAggregateMembers(ctx context.Context) ([]AggregateMemberRow, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT ca.aggregate_concept_id, ca.member_concept_id, n.canonical
		FROM concept_aggregate ca
		JOIN taxon_concept tc ON tc.id = ca.member_concept_id
		JOIN name n ON n.id = tc.accepted_name
		ORDER BY ca.aggregate_concept_id, ca.member_concept_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying concept_aggregate members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []AggregateMemberRow{}
	for rows.Next() {
		var r AggregateMemberRow
		if err := rows.Scan(&r.AggregateConceptID, &r.MemberConceptID, &r.MemberName); err != nil {
			return nil, fmt.Errorf("sqlite: scanning concept_aggregate member row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating concept_aggregate member rows: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/sqlite/... -run 'TestEuroslCrosswalkEntries|TestNativeEuroslConcepts|TestAllAggregateMembers' -v`
Expected: PASS, all 5 tests.

- [ ] **Step 5: Run the full package suite (no regressions)**

Run: `go test ./internal/adapters/sqlite/...`
Expected: PASS (ok, no FAIL lines).

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/sqlite/crosswalk.go internal/adapters/sqlite/crosswalk_test.go
git commit -m "feat(sqlite): eurosl crosswalk + aggregate member query methods"
```

---

### Task 2: Composition root (`internal/app`)

**Files:**
- Create: `internal/app/export_crosswalk.go`
- Test: `internal/app/export_crosswalk_test.go`

**Interfaces:**
- Consumes: `sqlite.Open(path string) (*sqlite.DB, error)`
  (`internal/adapters/sqlite/db.go:32`), `(*sqlite.DB).Close() error`,
  `(*sqlite.DB).EuroslCrosswalkEntries`, `(*sqlite.DB).NativeEuroslConcepts`,
  `(*sqlite.DB).AllAggregateMembers`, `sqlite.CrosswalkEntry`,
  `sqlite.AggregateMemberRow` (all Task 1), `app.Ingest(ctx, manifestPath,
  dbPath string) (app.IngestReport, error)` (existing, used only by tests).
- Produces (used by Task 3): `app.ExportCrosswalkReport{CrosswalkRows,
  MemberRows int; NameCollisions []app.CrosswalkCollision}`,
  `app.CrosswalkCollision{Name, FallAConceptID, FallBConceptID string}`,
  `app.ExportCrosswalk(ctx context.Context, dbPath, outDir string)
  (ExportCrosswalkReport, error)`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/app/export_crosswalk_test.go
package app_test

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
)

// readCSV reads path as a parsed CSV, failing the test on any error.
func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %q: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("parsing %q: %v", path, err)
	}
	return rows
}

// TestExportCrosswalk_WritesBothCSVsAndReportsCollisions drives
// app.ExportCrosswalk against testdata/dataset-agreement.yaml — the SAME
// fixture internal/application's Critical-1 regression test uses for the
// Fall-B-/agreement-ingest path (see that file's own comment). It doubles
// as this feature's collision fixture: after ingest, "Festuca" and
// "Festuca ovina agg." each carry BOTH a Fall-A name_space_entry row
// (auto-resolved against WCVP by canonical name) AND their own native
// Fall-B eurosl concept — a genuine collision is the EXPECTED outcome
// here, not a contrived edge case. Concept ids below were read directly
// off a real ingest of this fixture, not guessed.
func TestExportCrosswalk_WritesBothCSVsAndReportsCollisions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outDir := filepath.Join(dir, "out")

	if _, err := app.Ingest(ctx, "testdata/dataset-agreement.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	report, err := app.ExportCrosswalk(ctx, dbPath, outDir)
	if err != nil {
		t.Fatalf("app.ExportCrosswalk: unexpected error: %v", err)
	}

	if report.CrosswalkRows != 5 {
		t.Errorf("report.CrosswalkRows = %d, want 5 (3 Fall A + 2 Fall B)", report.CrosswalkRows)
	}
	if report.MemberRows != 2 {
		t.Errorf("report.MemberRows = %d, want 2 (eurosl + germansl aggregate edges)", report.MemberRows)
	}
	if len(report.NameCollisions) != 2 {
		t.Fatalf("len(report.NameCollisions) = %d, want 2, got %+v", len(report.NameCollisions), report.NameCollisions)
	}
	wantCollisions := map[string][2]string{
		"Festuca":            {"wcvp:concept:451511", "eurosl:concept:e-gen1"},
		"Festuca ovina agg.": {"wcvp:concept:415853", "eurosl:concept:e-agg1"},
	}
	for _, c := range report.NameCollisions {
		want, ok := wantCollisions[c.Name]
		if !ok {
			t.Errorf("unexpected collision name %q", c.Name)
			continue
		}
		if c.FallAConceptID != want[0] || c.FallBConceptID != want[1] {
			t.Errorf("collision %q = (%q, %q), want (%q, %q)", c.Name, c.FallAConceptID, c.FallBConceptID, want[0], want[1])
		}
	}

	rows := readCSV(t, filepath.Join(outDir, "eurosl_crosswalk.csv"))
	if len(rows) != 6 { // header + 5 data rows
		t.Fatalf("eurosl_crosswalk.csv has %d rows (incl. header), want 6: %+v", len(rows), rows)
	}
	if rows[0][0] != "name" || rows[0][1] != "concept_id" {
		t.Errorf("eurosl_crosswalk.csv header = %v, want [name concept_id]", rows[0])
	}

	memberRows := readCSV(t, filepath.Join(outDir, "aggregate_members.csv"))
	if len(memberRows) != 3 { // header + 2 data rows
		t.Fatalf("aggregate_members.csv has %d rows (incl. header), want 3: %+v", len(memberRows), memberRows)
	}
	if memberRows[0][0] != "aggregate_concept_id" || memberRows[0][1] != "member_concept_id" || memberRows[0][2] != "member_name" {
		t.Errorf("aggregate_members.csv header = %v, want [aggregate_concept_id member_concept_id member_name]", memberRows[0])
	}
	foundEurosl := false
	for _, r := range memberRows[1:] {
		if r[0] == "eurosl:concept:e-agg1" {
			foundEurosl = true
			if r[1] != "wcvp:concept:415853" || r[2] != "Festuca ovina" {
				t.Errorf("eurosl aggregate row = %v, want member wcvp:concept:415853/Festuca ovina", r)
			}
		}
	}
	if !foundEurosl {
		t.Errorf("aggregate_members.csv = %+v, want a row for eurosl:concept:e-agg1", memberRows)
	}
}

// TestExportCrosswalk_CreatesMissingOutDir confirms --out-dir need not
// already exist.
func TestExportCrosswalk_CreatesMissingOutDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outDir := filepath.Join(dir, "does", "not", "exist", "yet")

	if _, err := app.Ingest(ctx, "testdata/dataset-no-namespace.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if _, err := app.ExportCrosswalk(ctx, dbPath, outDir); err != nil {
		t.Fatalf("app.ExportCrosswalk: unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "eurosl_crosswalk.csv")); err != nil {
		t.Errorf("eurosl_crosswalk.csv not created in new out-dir: %v", err)
	}
}

// TestExportCrosswalk_EmptyAggregateData_WritesHeaderOnly pins the spec's
// error table: dataset-no-namespace.yaml declares no name_spaces at all,
// so concept_aggregate stays empty -> aggregate_members.csv gets only its
// header row, never an error.
func TestExportCrosswalk_EmptyAggregateData_WritesHeaderOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outDir := filepath.Join(dir, "out")

	if _, err := app.Ingest(ctx, "testdata/dataset-no-namespace.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	report, err := app.ExportCrosswalk(ctx, dbPath, outDir)
	if err != nil {
		t.Fatalf("app.ExportCrosswalk: unexpected error: %v", err)
	}
	if report.MemberRows != 0 {
		t.Errorf("report.MemberRows = %d, want 0", report.MemberRows)
	}
	if len(report.NameCollisions) != 0 {
		t.Errorf("report.NameCollisions = %+v, want none", report.NameCollisions)
	}
	rows := readCSV(t, filepath.Join(outDir, "aggregate_members.csv"))
	if len(rows) != 1 {
		t.Errorf("aggregate_members.csv has %d rows, want 1 (header only): %+v", len(rows), rows)
	}
}

// TestExportCrosswalk_UnopenableDatabase_ReportsNamedError mirrors
// app.Bundle's TestBundle_UnopenableDatabase_ReportsNamedError.
func TestExportCrosswalk_UnopenableDatabase_ReportsNamedError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir", "hostus.sqlite")
	report, err := app.ExportCrosswalk(context.Background(), missing, t.TempDir())
	if err == nil {
		t.Fatalf("app.ExportCrosswalk(%q): want an error naming the unopenable database", missing)
	}
	if report.CrosswalkRows != 0 {
		t.Errorf("report.CrosswalkRows = %d, want 0 on the error path", report.CrosswalkRows)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/... -run TestExportCrosswalk -v`
Expected: FAIL with `undefined: app.ExportCrosswalk` (and `app.ExportCrosswalkReport` etc.).

- [ ] **Step 3: Write the implementation**

```go
// internal/app/export_crosswalk.go
package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
)

// ExportCrosswalkReport summarizes one "hostus export-crosswalk" run: how
// many rows each output CSV received, and every name collision found
// between the eurosl crosswalk's two sources.
type ExportCrosswalkReport struct {
	CrosswalkRows  int
	MemberRows     int
	NameCollisions []CrosswalkCollision
}

// CrosswalkCollision names one eurosl name that resolves to two different
// concept ids depending on source: Fall A's name_space_entry crosswalk vs.
// Fall B's own native eurosl concept. Reported, never silently resolved to
// one side (spec's "Prüfbare Zusagen") — both rows still land in
// eurosl_crosswalk.csv, and situs' own ingest decides what to do with an
// ambiguous name.
type CrosswalkCollision struct {
	Name           string
	FallAConceptID string
	FallBConceptID string
}

// ExportCrosswalk writes eurosl_crosswalk.csv and aggregate_members.csv
// into outDir (created if missing) from dbPath's ingested database (spec
// docs/superpowers/specs/2026-08-29-eurosl-crosswalk-export-design.md).
// Unlike Bundle, there is no redistribution gate: this is a local pipeline
// handoff between two services run by the same operator, not a
// distribution to a third party (spec, owner decision 2026-08-29).
func ExportCrosswalk(ctx context.Context, dbPath, outDir string) (ExportCrosswalkReport, error) {
	src, err := sqlite.Open(dbPath)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: creating output directory %q: %w", outDir, err)
	}

	fallA, err := src.EuroslCrosswalkEntries(ctx)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: reading eurosl crosswalk Fall A: %w", err)
	}
	fallB, err := src.NativeEuroslConcepts(ctx)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: reading eurosl crosswalk Fall B: %w", err)
	}
	collisions := detectCrosswalkCollisions(fallA, fallB)

	crosswalkPath := filepath.Join(outDir, "eurosl_crosswalk.csv")
	if err := writeCrosswalkCSV(crosswalkPath, fallA, fallB); err != nil {
		return ExportCrosswalkReport{}, err
	}

	members, err := src.AllAggregateMembers(ctx)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: reading aggregate members: %w", err)
	}
	membersPath := filepath.Join(outDir, "aggregate_members.csv")
	if err := writeAggregateMembersCSV(membersPath, members); err != nil {
		return ExportCrosswalkReport{}, err
	}

	return ExportCrosswalkReport{
		CrosswalkRows:  len(fallA) + len(fallB),
		MemberRows:     len(members),
		NameCollisions: collisions,
	}, nil
}

// detectCrosswalkCollisions finds every name present in BOTH fallA and
// fallB — see CrosswalkCollision's doc comment. Never resolved
// automatically: every match is reported, and both sides' rows still get
// written to eurosl_crosswalk.csv by writeCrosswalkCSV.
func detectCrosswalkCollisions(fallA, fallB []sqlite.CrosswalkEntry) []CrosswalkCollision {
	byName := make(map[string]string, len(fallA))
	for _, e := range fallA {
		byName[e.Name] = e.ConceptID
	}
	var collisions []CrosswalkCollision
	for _, e := range fallB {
		if aID, ok := byName[e.Name]; ok {
			collisions = append(collisions, CrosswalkCollision{
				Name: e.Name, FallAConceptID: aID, FallBConceptID: e.ConceptID,
			})
		}
	}
	return collisions
}

// writeCrosswalkCSV writes eurosl_crosswalk.csv: a header row, then every
// Fall A row, then every Fall B row — a plain concatenation (spec's
// UNION), never a merge that would hide a collision.
func writeCrosswalkCSV(path string, fallA, fallB []sqlite.CrosswalkEntry) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("app: creating %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"name", "concept_id"}); err != nil {
		return fmt.Errorf("app: writing %q header: %w", path, err)
	}
	for _, e := range fallA {
		if err := w.Write([]string{e.Name, e.ConceptID}); err != nil {
			return fmt.Errorf("app: writing %q row: %w", path, err)
		}
	}
	for _, e := range fallB {
		if err := w.Write([]string{e.Name, e.ConceptID}); err != nil {
			return fmt.Errorf("app: writing %q row: %w", path, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("app: flushing %q: %w", path, err)
	}
	return nil
}

// writeAggregateMembersCSV writes aggregate_members.csv: a header row,
// then every concept_aggregate row. An empty members slice (no Fall-B
// aggregate ingest ran) writes only the header — not an error (spec's
// error table).
func writeAggregateMembersCSV(path string, members []sqlite.AggregateMemberRow) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("app: creating %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"aggregate_concept_id", "member_concept_id", "member_name"}); err != nil {
		return fmt.Errorf("app: writing %q header: %w", path, err)
	}
	for _, m := range members {
		if err := w.Write([]string{m.AggregateConceptID, m.MemberConceptID, m.MemberName}); err != nil {
			return fmt.Errorf("app: writing %q row: %w", path, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("app: flushing %q: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/... -run TestExportCrosswalk -v`
Expected: PASS, all 4 tests.

- [ ] **Step 5: Run the full package suite (no regressions)**

Run: `go test ./internal/app/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/export_crosswalk.go internal/app/export_crosswalk_test.go
git commit -m "feat(app): ExportCrosswalk composition-root function"
```

---

### Task 3: CLI command (`cmd/hostus`) + CHANGELOG

**Files:**
- Create: `cmd/hostus/export_crosswalk.go`
- Modify: `cmd/hostus/root.go` (register the command)
- Test: `cmd/hostus/export_crosswalk_test.go`
- Modify: `CHANGELOG.md` (`[Unreleased]` section)

**Interfaces:**
- Consumes: `app.ExportCrosswalk(ctx, dbPath, outDir string)
  (app.ExportCrosswalkReport, error)`, `app.ExportCrosswalkReport`,
  `app.CrosswalkCollision` (Task 2); cobra's `*cobra.Command`,
  `cmd.Flags().GetString`, `cmd.OutOrStdout()`, `cmd.Context()`
  (established pattern, `cmd/hostus/bundle.go`).
- Produces: `newExportCrosswalkCmd() *cobra.Command`,
  `exportCrosswalkCmdName = "export-crosswalk"` (for
  `TestExportCrosswalkCommand_RegisteredOnRoot`, mirroring
  `bundleCmdName`).

- [ ] **Step 1: Write the failing tests**

```go
// cmd/hostus/export_crosswalk_test.go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportCrosswalkCommand_WritesBothFilesAndPrintsReport drives
// "hostus export-crosswalk --db <fixture> --out-dir <dir>" end to end
// through the real cobra wiring. dataset-no-namespace.yaml has no
// name_spaces at all, so this is a minimal but fully deterministic happy
// path (crosswalk_rows=0 member_rows=0 collisions=0) — the richer
// collision-bearing case is already covered at the app layer
// (internal/app/export_crosswalk_test.go), so this test only proves the
// CLI wiring, not the business logic again.
func TestExportCrosswalkCommand_WritesBothFilesAndPrintsReport(t *testing.T) {
	dbPath := ingestFixtureDB(t)
	outDir := filepath.Join(t.TempDir(), "out")

	cmd := newExportCrosswalkCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db=" + dbPath, "--out-dir=" + outDir})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	for _, name := range []string{"eurosl_crosswalk.csv", "aggregate_members.csv"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("stat %q: %v", name, err)
		}
	}

	got := out.String()
	if !strings.Contains(got, outDir) {
		t.Errorf("report %q, want it to mention the output directory %q", got, outDir)
	}
	if !strings.Contains(got, "crosswalk_rows=0") {
		t.Errorf("report %q, want it to mention crosswalk_rows=0", got)
	}
}

// TestExportCrosswalkCommand_MissingDBFlag_ReturnsError confirms --db is
// required.
func TestExportCrosswalkCommand_MissingDBFlag_ReturnsError(t *testing.T) {
	cmd := newExportCrosswalkCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--out-dir=" + filepath.Join(t.TempDir(), "out")})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --db is missing, got nil")
	}
}

// TestExportCrosswalkCommand_MissingOutDirFlag_ReturnsError confirms
// --out-dir is likewise required.
func TestExportCrosswalkCommand_MissingOutDirFlag_ReturnsError(t *testing.T) {
	dbPath := ingestFixtureDB(t)

	cmd := newExportCrosswalkCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --out-dir is missing, got nil")
	}
}

// TestExportCrosswalkCommand_RegisteredOnRoot confirms "hostus
// export-crosswalk" is wired into the command tree, not just constructible
// in isolation.
func TestExportCrosswalkCommand_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{exportCrosswalkCmdName})
	if err != nil {
		t.Fatalf("Find(export-crosswalk): %v", err)
	}
	if cmd.Use != exportCrosswalkCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, exportCrosswalkCmdName)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/hostus/... -run TestExportCrosswalkCommand -v`
Expected: FAIL — `newExportCrosswalkCmd`/`exportCrosswalkCmdName` undefined
(`ingestFixtureDB` and `newRootCmd` already exist from `bundle_test.go` /
`root.go`, so those calls resolve fine once the new symbols exist).

- [ ] **Step 3: Write the command implementation**

```go
// cmd/hostus/export_crosswalk.go
package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/jobrunner/hostus/internal/app"
)

const exportCrosswalkCmdName = "export-crosswalk"

func newExportCrosswalkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   exportCrosswalkCmdName,
		Short: "Export eurosl_crosswalk.csv + aggregate_members.csv for situs' file-based species ingest",
		RunE:  runExportCrosswalk,
	}
	cmd.Flags().String("db", "", "path to the source SQLite database")
	cmd.Flags().String("out-dir", "", "output directory for eurosl_crosswalk.csv and aggregate_members.csv")
	return cmd
}

func runExportCrosswalk(cmd *cobra.Command, _ []string) error {
	dbPath, err := cmd.Flags().GetString("db")
	if err != nil {
		return err
	}
	if dbPath == "" {
		return errors.New("export-crosswalk: --db is required")
	}

	outDir, err := cmd.Flags().GetString("out-dir")
	if err != nil {
		return err
	}
	if outDir == "" {
		return errors.New("export-crosswalk: --out-dir is required")
	}

	report, err := app.ExportCrosswalk(cmd.Context(), dbPath, outDir)
	if err != nil {
		return err
	}

	printExportCrosswalkReport(cmd.OutOrStdout(), outDir, report)
	return nil
}

func printExportCrosswalkReport(w io.Writer, outDir string, report app.ExportCrosswalkReport) {
	_, _ = fmt.Fprintf(w, "Crosswalk export complete: %s (crosswalk_rows=%d member_rows=%d collisions=%d)\n",
		outDir, report.CrosswalkRows, report.MemberRows, len(report.NameCollisions))
	for _, c := range report.NameCollisions {
		_, _ = fmt.Fprintf(w, "  collision: %q -> fall_a=%s fall_b=%s\n", c.Name, c.FallAConceptID, c.FallBConceptID)
	}
}
```

- [ ] **Step 4: Register the command on root**

In `cmd/hostus/root.go`, add one line after `root.AddCommand(newBundleCmd())`:

```go
	root.AddCommand(newBundleCmd())
	root.AddCommand(newExportCrosswalkCmd())
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/hostus/... -run TestExportCrosswalkCommand -v`
Expected: PASS, all 4 tests.

- [ ] **Step 6: Run the full package suite (no regressions)**

Run: `go test ./cmd/hostus/...`
Expected: PASS.

- [ ] **Step 7: Update CHANGELOG.md**

In `CHANGELOG.md`, insert a new `### Added` section as the FIRST subsection
under `## [Unreleased]` (currently `### Changed` is first):

```markdown
## [Unreleased]

### Added

* **`hostus export-crosswalk --db <path> --out-dir <dir>`:** neuer CLI-
  Befehl, schreibt `eurosl_crosswalk.csv` (name→concept_id, Fall-A-
  Namensraum-Crosswalk + Fall-B-native-eurosl-Konzepte) und
  `aggregate_members.csv` (aggregate_concept_id→member_concept_id→
  member_name aus `concept_aggregate`) für situs' dateibasierten Species-
  Ingest. Kein Redistributions-Gate (lokaler Pipeline-Handoff, keine
  Weitergabe an Dritte). Eine Namens-Kollision zwischen den beiden Quellen
  wird gezählt und mit beiden Concept-IDs gemeldet, nie automatisch
  aufgelöst.

### Changed
```

- [ ] **Step 8: Run the canonical green-check**

Run: `make verify`
Expected: PASS (fmt-check, vet, lint, test, arch, debt-guard, compile all
green).

- [ ] **Step 9: Commit**

```bash
git add cmd/hostus/export_crosswalk.go cmd/hostus/root.go cmd/hostus/export_crosswalk_test.go CHANGELOG.md
git commit -m "feat(cli): hostus export-crosswalk command"
```
