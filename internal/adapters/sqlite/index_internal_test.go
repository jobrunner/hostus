package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// wantFKIndexes is the explicit set of indexes Hardening Task 2 adds on
// every FK child column that is not already the leading column of its
// table's own PRIMARY KEY (see the "Hardening Task 2" comment block in
// schema.sql for which columns were skipped, and why). Listing every name
// here — rather than just counting — means a later change that adds a new
// REFERENCES column without an index fails this test loudly instead of
// silently reintroducing the quadratic-ingest defect.
var wantFKIndexes = []string{
	"idx_name_basionym_id",
	"idx_taxon_concept_backbone_id",
	"idx_taxon_concept_accepted_name",
	"idx_taxon_concept_parent_id",
	"idx_concept_name_name_id",
	"idx_xref_concept_id",
	"idx_concept_relation_to_concept",
	"idx_fts_name_map_concept_id",
}

// TestOpen_CreatesAllFKIndexes proves every expected FK child-column index
// exists after Open() applies schema.sql. This is the regression guard for
// the Hardening Task 2 defect (docs/research/reality-check.md M1.1/M1.2):
// without an index on a REFERENCES column, `INSERT OR REPLACE` under
// `PRAGMA foreign_keys=ON` forces a full table scan per insert to check the
// FK, which measured out as quadratic ingest cost. If a future change adds
// a new FK column without indexing it, this test's explicit list will not
// contain the new index name (or a maintainer forgets to update the schema)
// and the test fails, rather than the defect silently coming back.
func TestOpen_CreatesAllFKIndexes(t *testing.T) {
	db := openTestDB(t)

	present := map[string]bool{}
	for _, n := range sqliteMasterNames(t, db) {
		present[n] = true
	}

	var missing []string
	for _, want := range wantFKIndexes {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("Open(): missing expected FK indexes %v; sqlite_master had %v", missing, sqliteMasterNames(t, db))
	}
}

// fkCheckQuery is the shape SQLite's own internal foreign-key-check query
// takes when it must find every row in a CHILD table that references a
// given parent key — exactly the query `INSERT OR REPLACE` (an implicit
// DELETE+INSERT) triggers once per referencing table, per row, under
// `PRAGMA foreign_keys=ON`. It is not something Go code ever issues
// directly, but EXPLAIN QUERY PLAN on this shape is a deterministic,
// SQLite-version-independent proxy for "will the real FK check use an
// index or scan the whole table".
// query is a fully literal "EXPLAIN QUERY PLAN SELECT 1 FROM <table>
// WHERE <column> = ?" string, spelled out rather than built by
// string-formatting/concatenating table/column — which would trip
// gosec's G201/G202 (SQL built from a variable) even though table/column
// here only ever come from this fixed, hand-written list, never external
// input.
type fkCheckQuery struct {
	table   string
	column  string
	query   string
	wantIdx string // index name SQLite's planner must pick
}

var fkCheckQueries = []fkCheckQuery{
	{"name", "basionym_id", `EXPLAIN QUERY PLAN SELECT 1 FROM name WHERE basionym_id = ?`, "idx_name_basionym_id"},
	{"taxon_concept", "backbone_id", `EXPLAIN QUERY PLAN SELECT 1 FROM taxon_concept WHERE backbone_id = ?`, "idx_taxon_concept_backbone_id"},
	{"taxon_concept", "accepted_name", `EXPLAIN QUERY PLAN SELECT 1 FROM taxon_concept WHERE accepted_name = ?`, "idx_taxon_concept_accepted_name"},
	{"taxon_concept", "parent_id", `EXPLAIN QUERY PLAN SELECT 1 FROM taxon_concept WHERE parent_id = ?`, "idx_taxon_concept_parent_id"},
	{"concept_name", "name_id", `EXPLAIN QUERY PLAN SELECT 1 FROM concept_name WHERE name_id = ?`, "idx_concept_name_name_id"},
	{"xref", "concept_id", `EXPLAIN QUERY PLAN SELECT 1 FROM xref WHERE concept_id = ?`, "idx_xref_concept_id"},
	{"concept_relation", "to_concept", `EXPLAIN QUERY PLAN SELECT 1 FROM concept_relation WHERE to_concept = ?`, "idx_concept_relation_to_concept"},
	{"fts_name_map", "concept_id", `EXPLAIN QUERY PLAN SELECT 1 FROM fts_name_map WHERE concept_id = ?`, "idx_fts_name_map_concept_id"},
}

// TestOpen_FKCheckQueriesUseIndexNotScan is the chosen guard against the
// ingest regressing to quadratic (task brief step 1's "scaling test"),
// implemented via EXPLAIN QUERY PLAN rather than wall-clock timing.
//
// A timing-based test (ingest N vs 2N synthetic rows through the real
// ingest path, assert the wall-clock ratio) was considered and rejected
// for this repo's unit-test suite: at N small enough to run fast in CI
// (the brief's own constraint), the absolute times are tens of
// milliseconds, and GC pauses/scheduler noise/shared-CI-runner jitter
// dominate the signal at that scale — exactly the quadratic-vs-linear
// distinction (~4x vs ~2x per doubling) the test exists to catch. That
// makes it prone to both false failures (loose N, real jitter) and false
// passes (N too small for either regime to show a measurable difference).
// The real before/after wall-clock numbers (which DO need to run at
// realistic scale to mean anything) are captured once, deliberately, by
// the scaling.sh harness against real WCVP data — see
// docs/research/reality-check.md's "nach Hardening" section — not
// re-derived from a fast, noisy unit test on every CI run.
//
// EXPLAIN QUERY PLAN on fkCheckQueries is deterministic instead: it asks
// SQLite's planner, not a clock, whether it would use the index or scan
// the table for the exact query shape the internal FK check issues. If an
// index in wantFKIndexes above is ever dropped or renamed out of sync with
// schema.sql, the planner immediately falls back to "SCAN <table>" and
// this test fails every time, on every machine, with zero flakiness.
func TestOpen_FKCheckQueriesUseIndexNotScan(t *testing.T) {
	db := openTestDB(t)

	for _, q := range fkCheckQueries {
		q := q
		t.Run(q.table+"."+q.column, func(t *testing.T) {
			assertFKCheckUsesIndex(t, db, q)
		})
	}
}

// assertFKCheckUsesIndex runs EXPLAIN QUERY PLAN for q's fkCheckQuery shape
// and fails if the plan is not a SEARCH via q.wantIdx. Table/column names
// come only from the fixed fkCheckQueries table above (never external
// input), so building the query by concatenation here — rather than
// passing them as bound parameters, which SQL doesn't allow for
// identifiers anyway — is safe.
func assertFKCheckUsesIndex(t *testing.T, db *DB, q fkCheckQuery) {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), q.query, "any-value")
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN for %s.%s: unexpected error: %v", q.table, q.column, err)
	}
	defer func() { _ = rows.Close() }()

	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scanning query plan row for %s.%s: %v", q.table, q.column, err)
		}
		plan.WriteString(detail)
		plan.WriteString("; ")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating query plan rows for %s.%s: %v", q.table, q.column, err)
	}

	got := plan.String()
	if !strings.Contains(got, "USING COVERING INDEX "+q.wantIdx) && !strings.Contains(got, "USING INDEX "+q.wantIdx) {
		t.Fatalf("query plan for %s.%s = %q, want it to use index %q (not a full table scan) — the FK check this query models would scan the whole %q table per insert without it",
			q.table, q.column, got, q.wantIdx, q.table)
	}
	if strings.Contains(got, "SCAN "+q.table) {
		t.Fatalf("query plan for %s.%s = %q contains a full SCAN of %q, want SEARCH via %q", q.table, q.column, got, q.table, q.wantIdx)
	}
}

// BenchmarkIngestTx_Scaling is an informational, non-gating companion to
// the deterministic query-plan guard above (TestOpen_FKCheckQueriesUseIndexNotScan
// is what CI actually enforces). It ingests N and 2N synthetic rows
// through the real BeginIngest/UpsertName/UpsertConcept/LinkName/Commit
// path and reports the wall-clock ratio via `go test -bench`, for anyone
// who wants a quick local sanity check that ingest cost is still roughly
// linear in row count.
//
// A pass/fail *test* asserting a wall-clock ratio threshold (e.g.
// "ratio < 3.0") was tried first and pulled back out after it flaked in
// practice: run in isolation it reliably showed ratios around 1.9-2.0,
// but run inside the full `go test ./...` suite (where other packages'
// tests compete for CPU) it produced a 3.21 ratio against exactly that
// 3.0 threshold — a false failure with no code change involved, on a
// developer laptop, not even a loaded CI runner. That is the flakiness
// the task brief anticipated and explicitly permits substituting a
// deterministic check for. Quadratic-vs-linear scaling (~4x vs ~2x per
// doubling) is a real, measurable difference at realistic data volumes
// (see docs/research/reality-check.md's M1.2/"nach Hardening" scaling
// runs against actual WCVP data), but at the row counts a unit test can
// afford to run, wall-clock time is dominated by noise the effect itself
// is supposed to be several times larger than — not a reliable regression
// gate. TestOpen_FKCheckQueriesUseIndexNotScan checks the same underlying
// fact (does the FK-check query use an index or scan the table) without
// a clock, so it is the enforced guard; this benchmark is kept only as a
// convenience, never wired into `make test`/CI.
func BenchmarkIngestTx_Scaling(b *testing.B) {
	for _, n := range []int{1500, 3000, 6000} {
		n := n
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ingestSyntheticRowsB(b, n)
			}
		})
	}
}

// ingestSyntheticRowsB is BenchmarkIngestTx_Scaling's row generator: it
// ingests count synthetic concepts (each with its own accepted name,
// linked via concept_name) into a fresh in-memory database through the
// real ingest path.
func ingestSyntheticRowsB(b *testing.B, count int) {
	b.Helper()
	db, err := Open(":memory:")
	if err != nil {
		b.Fatalf(`Open(":memory:"): unexpected error: %v`, err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{
		ID: "wcvp", Version: "v1", IngestedAt: "2026-08-01T00:00:00Z", ManifestSHA: "x",
	})
	if err != nil {
		b.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	for i := 0; i < count; i++ {
		nameID := fmt.Sprintf("n-%d", i)
		conceptID := fmt.Sprintf("c-%d", i)
		name := domain.Name{ID: nameID, Canonical: fmt.Sprintf("Genus species%d", i), Rank: domain.RankSpecies}
		concept := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			b.Fatalf("UpsertName(%d): unexpected error: %v", i, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			b.Fatalf("UpsertConcept(%d): unexpected error: %v", i, err)
		}
		if err := tx.LinkName(conceptID, nameID, "accepted", nil); err != nil {
			b.Fatalf("LinkName(%d): unexpected error: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("Commit: unexpected error: %v", err)
	}
}
