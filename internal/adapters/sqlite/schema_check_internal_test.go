package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpen_RejectsSchemaDriftMissingColumn pins the fix for the SP8 known gap:
// an index built by an older hostus that lacks a column the current schema
// declares — here vernacular.preferred — must make Open fail LOUDLY, naming
// the table and column, instead of opening cleanly and letting the first
// query selecting it 500 at runtime while /health/ready still reports 200.
// Open applies schema.sql with CREATE TABLE IF NOT EXISTS, which never adds
// a column to an already-existing table, so this is the exact drift a real
// legacy database is in. vernacular is deliberately chosen because it has
// no ad-hoc migrateXxx counterpart (unlike xref.source, concept_relation's
// widened PK, name_space_entry.status or taxon_concept's classification
// columns) — those columns are auto-healed on Open before this generic
// check ever runs, so a fixture built around one of them would not exercise
// this path.
func TestOpen_RejectsSchemaDriftMissingColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.sqlite")

	// Pre-create vernacular in its pre-`preferred` shape on a raw
	// connection, then close it. Open's CREATE TABLE IF NOT EXISTS will
	// then leave this table exactly as-is, reproducing the legacy drift.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(raw): %v", err)
	}
	if _, err := raw.ExecContext(context.Background(), `
		CREATE TABLE vernacular (
			concept_id TEXT NOT NULL,
			lang       TEXT NOT NULL,
			name       TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("creating legacy vernacular: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing raw: %v", err)
	}

	db, err := Open(path)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open on a schema-drifted database returned nil error; want a loud startup failure")
	}
	// Assert the exact table+column phrasing, not just that both words appear
	// somewhere: a check that flagged the whole table (every column "missing")
	// would satisfy two independent Contains calls but be a different, wronger
	// diagnosis.
	if !strings.Contains(err.Error(), "vernacular (missing preferred)") {
		t.Errorf("Open error = %q, want it to name %q", err.Error(), "vernacular (missing preferred)")
	}
}

// TestOpen_RejectsSchemaDriftInAnyTable proves the check is general across
// tables, not special-cased to taxon_concept: dropping resolution from a
// DIFFERENT table (name_space_entry, whose column arrived in SP9) must be
// caught and named too. Without this, a check narrowed to taxon_concept — or
// one that stopped at the first table — would reopen the silent-500 bug on
// every other table.
func TestOpen_RejectsSchemaDriftInAnyTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old2.sqlite")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(raw): %v", err)
	}
	// Pre-SP9 name_space_entry: everything except `resolution`.
	if _, err := raw.ExecContext(context.Background(), `
		CREATE TABLE name_space_entry (
			space      TEXT NOT NULL,
			ext_id     TEXT NOT NULL,
			concept_id TEXT NOT NULL,
			name       TEXT NOT NULL,
			aggregate  INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		t.Fatalf("creating legacy name_space_entry: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing raw: %v", err)
	}

	db, err := Open(path)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open on a name_space_entry-drifted database returned nil error; want a loud startup failure")
	}
	if !strings.Contains(err.Error(), "name_space_entry (missing resolution)") {
		t.Errorf("Open error = %q, want it to name %q", err.Error(), "name_space_entry (missing resolution)")
	}
}

// TestOpen_ToleratesExtraColumn pins the one-directional contract the doc
// comment and CHANGELOG promise: a database with MORE columns than the current
// schema knows — i.e. one written by a NEWER hostus — must still open. A check
// that compared column counts or rejected unknown columns would pass every
// other test here yet break this forward-compatibility guarantee.
func TestOpen_ToleratesExtraColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newer.sqlite")

	// A full current-schema database, then a column the schema does NOT declare.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(fresh): %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(raw): %v", err)
	}
	if _, err := raw.ExecContext(context.Background(), `ALTER TABLE taxon_concept ADD COLUMN future_col TEXT`); err != nil {
		t.Fatalf("adding extra column: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing raw: %v", err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("Open on a database with an extra column errored = %v, want nil (extra columns must be tolerated)", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("Close(db2): %v", err)
	}
}

// TestOpen_AcceptsCurrentSchema guards against a false positive: a freshly
// created database (every current column present) must open without error,
// and reopening it (Open is idempotent) must too.
func TestOpen_AcceptsCurrentSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.sqlite")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(fresh) error = %v, want nil", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error = %v, want nil (drift check must not false-positive)", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("Close(reopen): %v", err)
	}
}
