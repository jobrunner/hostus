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
// declares — here trait_value.resolution, added in SP3 — must make Open fail
// LOUDLY, naming the table and column, instead of opening cleanly and letting
// the first /v1/concept/{id}/traits query 500 at runtime while /health/ready
// still reports 200. Open applies schema.sql with CREATE TABLE IF NOT EXISTS,
// which never adds a column to an already-existing table, so this is the exact
// drift a real legacy database is in.
func TestOpen_RejectsSchemaDriftMissingColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.sqlite")

	// Pre-create trait_value in its pre-SP3 shape (no `resolution` column) on a
	// raw connection, then close it. Open's CREATE TABLE IF NOT EXISTS will then
	// leave this table exactly as-is, reproducing the legacy drift.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(raw): %v", err)
	}
	if _, err := raw.ExecContext(context.Background(), `
		CREATE TABLE trait_value (
			concept_id    TEXT NOT NULL,
			vocab         TEXT NOT NULL,
			vocab_version TEXT NOT NULL,
			dim           TEXT NOT NULL,
			value         TEXT NOT NULL,
			niche_width   REAL,
			n_systems     INTEGER
		)`); err != nil {
		t.Fatalf("creating legacy trait_value: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing raw: %v", err)
	}

	db, err := Open(path)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open on a schema-drifted database returned nil error; want a loud startup failure")
	}
	for _, want := range []string{"trait_value", "resolution"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Open error = %q, want it to name %q", err.Error(), want)
		}
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
