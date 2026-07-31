package sqlite

import (
	"context"
	"sort"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// wantTables lists every table/virtual-table spec §4.3 requires, plus the
// rowid<->concept mapping table this schema adds for fts_name.
var wantTables = []string{
	"backbone_version",
	"name",
	"taxon_concept",
	"concept_name",
	"xref",
	"vernacular",
	"distribution",
	"trait_value",
	"concept_relation",
	"fts_name_map",
	"fts_name",
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf(`Open(":memory:"): unexpected error: %v`, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sqliteMasterNames(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.Query(`SELECT name FROM sqlite_master`)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scanning sqlite_master row: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating sqlite_master rows: %v", err)
	}
	return names
}

func TestOpen_CreatesAllSchemaTables(t *testing.T) {
	db := openTestDB(t)

	present := map[string]bool{}
	for _, n := range sqliteMasterNames(t, db) {
		present[n] = true
	}

	var missing []string
	for _, want := range wantTables {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("Open(): missing expected tables %v; sqlite_master had %v", missing, sqliteMasterNames(t, db))
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db := openTestDB(t)
	var on int
	if err := db.sql.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
		t.Fatalf("querying foreign_keys pragma: %v", err)
	}
	if on != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1 (on)", on)
	}
}

func TestOpen_SchemaIsIdempotent(t *testing.T) {
	// Re-applying the embedded schema against an already-initialized
	// connection must not error: every DDL statement is IF NOT EXISTS.
	db := openTestDB(t)
	if _, err := db.sql.Exec(schemaSQL); err != nil {
		t.Fatalf("re-applying schema: expected no error (idempotent DDL), got: %v", err)
	}
}

func TestBackboneVersion_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	bv := domain.BackboneVersion{
		ID:          "wcvp",
		Version:     "2026-06-15",
		License:     "CC-BY-4.0",
		SourceURL:   "https://example.org/wcvp.zip",
		IngestedAt:  "2026-07-31T00:00:00Z",
		ManifestSHA: "deadbeef",
	}

	tx, err := db.BeginIngest(ctx, bv)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	got, err := db.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("BackboneVersions() = %v, want exactly 1 entry", got)
	}
	if got[0] != bv {
		t.Fatalf("BackboneVersions()[0] = %+v, want %+v", got[0], bv)
	}
}

func TestBeginIngest_RollbackDiscardsBackboneVersion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	bv := domain.BackboneVersion{ID: "wcvp", Version: "2026-06-15", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "deadbeef"}

	tx, err := db.BeginIngest(ctx, bv)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: unexpected error: %v", err)
	}

	got, err := db.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("BackboneVersions() after rollback = %v, want empty", got)
	}
}

func TestBackboneVersions_OrdersByID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, id := range []string{"wfo", "colxr", "wcvp"} {
		tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: id, Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
		if err != nil {
			t.Fatalf("BeginIngest(%q): unexpected error: %v", id, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(%q): unexpected error: %v", id, err)
		}
	}

	got, err := db.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	ids := make([]string, 0, len(got))
	for _, bv := range got {
		ids = append(ids, bv.ID)
	}
	want := []string{"colxr", "wcvp", "wfo"}
	if len(ids) != len(want) {
		t.Fatalf("BackboneVersions() ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("BackboneVersions() ids = %v, want %v", ids, want)
		}
	}
}

func rowCount(t *testing.T, db *DB, table string) int {
	t.Helper()
	var n int
	if err := db.sql.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("counting rows in %q: %v", table, err)
	}
	return n
}

func TestIngestTx_UpsertNameConceptLinkXrefDistribution(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	bv := domain.BackboneVersion{ID: "wcvp", Version: "2026-06-15", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "deadbeef"}
	tx, err := db.BeginIngest(ctx, bv)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	accepted := domain.Name{ID: "n1", Canonical: "corynephorus canescens", Authorship: "(L.) P.Beauv.", Rank: domain.RankSpecies}
	synonym := domain.Name{ID: "n2", Canonical: "weingaertneria canescens", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "c1", BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}

	if err := tx.UpsertName(accepted); err != nil {
		t.Fatalf("UpsertName(accepted): unexpected error: %v", err)
	}
	if err := tx.UpsertName(synonym); err != nil {
		t.Fatalf("UpsertName(synonym): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	homotypic := true
	if err := tx.LinkName(concept.ID, accepted.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(accepted): unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, synonym.ID, "synonym", &homotypic); err != nil {
		t.Fatalf("LinkName(synonym): unexpected error: %v", err)
	}
	if err := tx.AddXref(concept.ID, domain.Xref{Authority: "powo", ExtID: "396681-1"}); err != nil {
		t.Fatalf("AddXref: unexpected error: %v", err)
	}
	if err := tx.AddDistribution(concept.ID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: "GER"}); err != nil {
		t.Fatalf("AddDistribution: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	// Task 3 implements the read side; assert the rows landed by counting
	// directly.
	want := map[string]int{"name": 2, "taxon_concept": 1, "concept_name": 2, "xref": 1, "distribution": 1}
	for table, n := range want {
		if got := rowCount(t, db, table); got != n {
			t.Errorf("row count for %q = %d, want %d", table, got, n)
		}
	}
}

func TestIngestTx_UpsertNameWithDanglingBasionymFKFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	n := domain.Name{ID: "n1", Canonical: "x", Rank: domain.RankSpecies, BasionymID: "does-not-exist"}
	if err := tx.UpsertName(n); err == nil {
		t.Fatal("UpsertName with a dangling basionym_id: expected a foreign-key error, got nil")
	}
}

// Concept, ConceptByXref, and MatchExact are exercised in read_test.go
// against the real database, seeded from testdata/seed.sql (Task 3).
