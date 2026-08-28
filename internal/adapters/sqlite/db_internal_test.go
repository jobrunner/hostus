package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
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
	"xref_source",
	"vernacular",
	"distribution",
	"trait_value",
	"trait_vocabulary",
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

// TestOpen_ForeignKeysEnabled proves FK enforcement is genuinely active on
// the connection Open() returns, not just that the pragma value reads back
// as 1 (which modernc.org/sqlite would also report on a connection where
// the pragma silently failed to take effect, or on a *different* pooled
// connection than the one that ran it — foreign_keys is per-connection).
// It does so by attempting a write that violates taxon_concept's
// backbone_id FK and asserting SQLite rejects it.
func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, status)
		VALUES ('c-orphan', 'does-not-exist', 'n-does-not-exist', 'SPECIES', 'ACCEPTED')`)
	if err == nil {
		t.Fatal("insert referencing a non-existent backbone_id: expected a foreign-key constraint error, got nil (PRAGMA foreign_keys is not effectively ON)")
	}
}

// TestOpen_SingleConnectionPool pins down the fix for the FK-pragma bug:
// Open must cap the pool at exactly one physical connection. foreign_keys
// is a per-connection SQLite pragma and path=":memory:" gives each
// physical connection its own private database, so any pool size above 1
// would let some connections silently bypass FK enforcement (or see an
// empty database) depending on which connection database/sql happened to
// hand out.
func TestOpen_SingleConnectionPool(t *testing.T) {
	db := openTestDB(t)
	if got := db.sql.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("Stats().MaxOpenConnections = %d, want 1 (single-writer; FK pragma and :memory: isolation are per-connection)", got)
	}
}

// TestOpen_MemoryDatabaseIsSharedAcrossQueries proves a ":memory:" DB
// behaves as one shared database, not one private database per pooled
// connection: a write via BeginIngest/Commit must be visible to a
// completely separate subsequent query against the same *DB.
func TestOpen_MemoryDatabaseIsSharedAcrossQueries(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"}
	tx, err := db.BeginIngest(ctx, bv)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	// A fresh query, independent of the transaction above: if :memory:
	// were not shared across the pool, this could hit a different,
	// empty private database and see nothing.
	got, err := db.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wcvp" {
		t.Fatalf("BackboneVersions() = %+v, want exactly one entry for %q (write from an earlier transaction must be visible on a later, independent query)", got, "wcvp")
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
	if err := tx.AddXref(concept.ID, domain.Xref{Authority: "powo", ExtID: "396681-1"}, ""); err != nil {
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

// TestIngestTx_UpsertNameConceptWithOtherRank_PersistsRankVerbatim is the
// fix-round-1 regression: an OTHER-ranked Name/Concept (WCVP's "lusus",
// via domain.ParseRankLenient) must land its RankVerbatim in the raw
// name.rank_verbatim/taxon_concept.rank_verbatim columns, not just live in
// the in-process domain.Name/Concept for the duration of the ingest run —
// otherwise the moment the process exits, hostus can no longer tell
// "lusus" from "stirps", only "OTHER" (spec §A.1: a nomenclature service
// must not lose which name/rank something actually is).
func TestIngestTx_UpsertNameConceptWithOtherRank_PersistsRankVerbatim(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	name := domain.Name{ID: "n-lusus", Canonical: "Paeonia corallina lusus ovatifolia", Rank: domain.RankOther, RankVerbatim: "lusus"}
	concept := domain.Concept{ID: "c-lusus", BackboneID: "wcvp", AcceptedName: name, Rank: domain.RankOther, RankVerbatim: "lusus", Status: domain.StatusSynonym}

	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	var nameVerbatim, conceptVerbatim string
	if err := db.sql.QueryRow(`SELECT rank_verbatim FROM name WHERE id = ?`, "n-lusus").Scan(&nameVerbatim); err != nil {
		t.Fatalf("reading name.rank_verbatim: unexpected error: %v", err)
	}
	if nameVerbatim != "lusus" {
		t.Errorf("name.rank_verbatim = %q, want %q", nameVerbatim, "lusus")
	}
	if err := db.sql.QueryRow(`SELECT rank_verbatim FROM taxon_concept WHERE id = ?`, "c-lusus").Scan(&conceptVerbatim); err != nil {
		t.Fatalf("reading taxon_concept.rank_verbatim: unexpected error: %v", err)
	}
	if conceptVerbatim != "lusus" {
		t.Errorf("taxon_concept.rank_verbatim = %q, want %q", conceptVerbatim, "lusus")
	}
}

// TestIngestTx_UpsertNameWithCanonicalRank_LeavesRankVerbatimNull proves the
// other half: a canonically-ranked Name/Concept must leave rank_verbatim
// NULL (not an empty string, and never the canonical spelling itself) —
// nullString's job (see db.go) — so a later SELECT ... WHERE rank_verbatim
// IS NOT NULL reliably finds only genuine OTHER-rank rows.
func TestIngestTx_UpsertNameWithCanonicalRank_LeavesRankVerbatimNull(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	name := domain.Name{ID: "n-species", Canonical: "Corynephorus canescens", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "c-species", BackboneID: "wcvp", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	var nameVerbatim, conceptVerbatim sql.NullString
	if err := db.sql.QueryRow(`SELECT rank_verbatim FROM name WHERE id = ?`, "n-species").Scan(&nameVerbatim); err != nil {
		t.Fatalf("reading name.rank_verbatim: unexpected error: %v", err)
	}
	if nameVerbatim.Valid {
		t.Errorf("name.rank_verbatim = %+v, want SQL NULL", nameVerbatim)
	}
	if err := db.sql.QueryRow(`SELECT rank_verbatim FROM taxon_concept WHERE id = ?`, "c-species").Scan(&conceptVerbatim); err != nil {
		t.Fatalf("reading taxon_concept.rank_verbatim: unexpected error: %v", err)
	}
	if conceptVerbatim.Valid {
		t.Errorf("taxon_concept.rank_verbatim = %+v, want SQL NULL", conceptVerbatim)
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

// TestOpen_WALModeOnFileDB proves Open actually turns journal_mode to WAL
// on a real (file-backed) database. A ":memory:" database always reports
// "memory" regardless of the requested journal_mode — SQLite has no WAL
// concept for an in-memory database — so this must run against a temp
// file to mean anything.
func TestOpen_WALModeOnFileDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): unexpected error: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var mode string
	if err := db.sql.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", mode, "wal")
	}
}

// TestOpen_BusyTimeoutIsSet proves Open sets a non-zero busy_timeout, so a
// lock wait blocks briefly instead of failing immediately with
// SQLITE_BUSY.
func TestOpen_BusyTimeoutIsSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): unexpected error: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var ms int
	if err := db.sql.QueryRow(`PRAGMA busy_timeout`).Scan(&ms); err != nil {
		t.Fatalf("querying busy_timeout: %v", err)
	}
	if ms != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", ms)
	}
}

// TestOpen_ReaderSeesCommittedRowAlongsideOpenWriter proves the whole point
// of WAL for hostus: a SECOND Open (modeling serve's independent reader
// connection) can read a row committed by a FIRST Open (modeling an
// ingest writer) even while the writer's *DB still holds a write
// transaction open. Under the default rollback-journal mode, that open
// write transaction would keep an exclusive lock for its own duration and
// the reader would block (bounded only by busy_timeout, i.e. eventually
// fail here) instead of proceeding immediately. This is a deterministic
// committed-row read, not a sleep-based race.
func TestOpen_ReaderSeesCommittedRowAlongsideOpenWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	ctx := context.Background()

	writer, err := Open(path)
	if err != nil {
		t.Fatalf("Open(writer): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	committed := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"}
	tx, err := writer.BeginIngest(ctx, committed)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	// A second, independent write transaction on the writer connection,
	// left open for the duration of the reader's query below.
	holdTx, err := writer.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx (holding writer): unexpected error: %v", err)
	}
	if _, err := holdTx.ExecContext(ctx, `
		INSERT OR REPLACE INTO backbone_version (id, version, license, source_url, ingested_at, manifest_sha)
		VALUES ('wfo', 'v1', '', '', '2026-07-31T00:00:00Z', 'y')`); err != nil {
		_ = holdTx.Rollback()
		t.Fatalf("writing inside held transaction: unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = holdTx.Rollback() })

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reader): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	got, err := reader.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions (reader, writer tx still open): unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wcvp" {
		t.Fatalf("BackboneVersions() = %+v, want exactly the committed %q row (uncommitted 'wfo' must stay invisible)", got, "wcvp")
	}
}

// TestBeginTraitIngest_WritesNoBackboneVersion pins the adapter half of the
// trait/backbone separation: BeginIngest records its argument in
// backbone_version, BeginTraitIngest must record nothing there — a trait
// vocabulary is not a taxonomic backbone, and backbone_version is served as
// API provenance and gates /health/ready.
func TestBeginTraitIngest_WritesNoBackboneVersion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	meta := domain.TraitVocabMeta{Vocab: domain.VocabEIVE, Version: "1.0", Taxonomy: "euromed-via-eurosl", License: "CC-BY-4.0"}
	if err := tx.UpsertTraitVocabulary(meta); err != nil {
		t.Fatalf("UpsertTraitVocabulary: unexpected error: %v", err)
	}
	// Finalize must be a harmless no-op here: this transaction has no
	// backbone, so there are no concepts of its own to FTS-index.
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	got, err := db.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("BackboneVersions() = %v, want empty after a trait-only ingest", got)
	}

	vocabs, err := db.TraitVocabularies(ctx)
	if err != nil {
		t.Fatalf("TraitVocabularies: unexpected error: %v", err)
	}
	if len(vocabs) != 1 {
		t.Fatalf("len(TraitVocabularies) = %d, want 1 — the vocabulary itself must still be recorded", len(vocabs))
	}
}

// TestBeginTraitIngest_FinalizeDoesNotTouchAnotherBackbonesFTSIndex pins
// why the empty backboneID is safe: Finalize filters on tc.backbone_id, so
// a trait transaction can never re-index (and thus duplicate) the rows a
// real backbone's own ingest already wrote.
func TestBeginTraitIngest_FinalizeDoesNotTouchAnotherBackbonesFTSIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	seedOneConcept(t, db)
	before := ftsRowCount(t, db)
	if before == 0 {
		t.Fatal("fts_name_map is empty after the backbone ingest; the fixture must index something")
	}

	tx, err := db.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	if after := ftsRowCount(t, db); after != before {
		t.Errorf("fts_name_map rows = %d after a trait ingest, want %d (unchanged)", after, before)
	}
}

func seedOneConcept(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-01T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "n-1", Canonical: "Festuca ovina", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "c-1", BackboneID: "wcvp", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

func ftsRowCount(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM fts_name_map`).Scan(&n); err != nil {
		t.Fatalf("counting fts_name_map: %v", err)
	}
	return n
}

// TestIngestTx_UpsertXrefSource_PersistsProvenanceAndAttributesXrefs is the
// C1b regression: before xref_source existed, an ingested database could not
// answer "which harvest are these xrefs from?" — the source's version,
// license and manifest_sha were report-only, and no xref row said where it
// came from. Both must now round-trip.
func TestIngestTx_UpsertXrefSource_PersistsProvenanceAndAttributesXrefs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	seedOneConcept(t, db) // concept id "c-1"

	tx, err := db.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	meta := domain.XrefSourceMeta{
		ID: "wikidata", Version: "2026-08-02", License: "CC0",
		SourceURL:   "https://query.wikidata.org/sparql",
		ManifestSHA: "cafebabe", Redistribution: domain.RedistributionAllowed,
	}
	if err := tx.UpsertXrefSource(meta); err != nil {
		t.Fatalf("UpsertXrefSource: unexpected error: %v", err)
	}
	if err := tx.AddXref("c-1", domain.Xref{Authority: "inat", ExtID: "160927"}, meta.ID); err != nil {
		t.Fatalf("AddXref: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	var version, license, sha, redistribution string
	if err := db.sql.QueryRowContext(ctx, `
		SELECT version, license, manifest_sha, redistribution FROM xref_source WHERE id = ?`, meta.ID,
	).Scan(&version, &license, &sha, &redistribution); err != nil {
		t.Fatalf("reading xref_source: unexpected error: %v", err)
	}
	if version != "2026-08-02" || license != "CC0" || sha != "cafebabe" || redistribution != "allowed" {
		t.Errorf("xref_source row = (%q, %q, %q, %q), want (%q, %q, %q, %q)",
			version, license, sha, redistribution, "2026-08-02", "CC0", "cafebabe", "allowed")
	}

	var source sql.NullString
	if err := db.sql.QueryRowContext(ctx, `SELECT source FROM xref WHERE authority = 'inat' AND ext_id = '160927'`).Scan(&source); err != nil {
		t.Fatalf("reading xref.source: unexpected error: %v", err)
	}
	if !source.Valid || source.String != meta.ID {
		t.Errorf("xref.source = %v, want %q", source, meta.ID)
	}
}

// TestIngestTx_AddXref_BackboneDerivedRowHasNullSource pins the deliberate
// asymmetry documented in schema.sql: an xref the BACKBONE ingest derives
// from a taxon row carries source NULL (it is gated by the backbone's own
// redistribution value), never the empty string — an empty string would be a
// foreign-key violation against xref_source.
func TestIngestTx_AddXref_BackboneDerivedRowHasNullSource(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	seedOneConcept(t, db) // concept id "c-1"

	tx, err := db.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	if err := tx.AddXref("c-1", domain.Xref{Authority: "powo", ExtID: "396681-1"}, ""); err != nil {
		t.Fatalf("AddXref: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	var isNull bool
	if err := db.sql.QueryRowContext(ctx, `SELECT source IS NULL FROM xref WHERE authority = 'powo'`).Scan(&isNull); err != nil {
		t.Fatalf("reading xref.source: unexpected error: %v", err)
	}
	if !isNull {
		t.Error("xref.source for a backbone-derived row is not NULL, want NULL")
	}
}

// TestOpen_MigratesXrefSourceColumnOntoAPreExistingDatabase proves the
// SP1–SP3 migration path: a database whose xref table was created WITHOUT the
// source column (schema.sql's CREATE TABLE is IF NOT EXISTS, so it would
// leave such a table untouched) must gain the column on the next Open, with
// its existing rows preserved and their source NULL.
func TestOpen_MigratesXrefSourceColumnOntoAPreExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: unexpected error: %v", err)
	}
	if _, err := legacy.Exec(`
		CREATE TABLE xref (
		  concept_id TEXT NOT NULL,
		  authority  TEXT NOT NULL,
		  ext_id     TEXT NOT NULL,
		  PRIMARY KEY (authority, ext_id)
		);
		INSERT INTO xref (concept_id, authority, ext_id) VALUES ('c-1', 'powo', '396681-1');`); err != nil {
		t.Fatalf("creating pre-migration xref table: unexpected error: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("closing pre-migration database: unexpected error: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(legacy): unexpected error: %v", err)
	}
	defer func() { _ = db.Close() }()

	var conceptID string
	var source sql.NullString
	if err := db.sql.QueryRow(`SELECT concept_id, source FROM xref WHERE ext_id = '396681-1'`).Scan(&conceptID, &source); err != nil {
		t.Fatalf("reading migrated xref row: unexpected error: %v", err)
	}
	if conceptID != "c-1" {
		t.Errorf("migrated xref.concept_id = %q, want %q (existing rows must survive)", conceptID, "c-1")
	}
	if source.Valid {
		t.Errorf("migrated xref.source = %q, want NULL", source.String)
	}

	// Idempotent: a second Open must not try to add the column again.
	again, err := Open(path)
	if err != nil {
		t.Fatalf("Open(already-migrated): unexpected error: %v", err)
	}
	_ = again.Close()
}
