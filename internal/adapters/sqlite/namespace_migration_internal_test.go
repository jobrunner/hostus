package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// legacyNameSpaceEntryDDL is the table as it stood BEFORE rank and
// accepted_name existed: resolution and status are present (those migrations
// already shipped), the two new columns are not.
const legacyNameSpaceEntryDDL = `
	CREATE TABLE name_space_entry (
		space      TEXT NOT NULL,
		ext_id     TEXT NOT NULL,
		concept_id TEXT NOT NULL,
		name       TEXT NOT NULL,
		aggregate  INTEGER NOT NULL DEFAULT 0,
		resolution TEXT,
		status     TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (space, ext_id)
	)`

// TestOpen_MigratesLegacyNameSpaceEntryColumns pins the compatibility promise
// the two new migrations make, which was asserted in doc comments and a
// CHANGELOG note but never tested: an index built before rank/accepted_name
// existed must still OPEN (verifySchemaColumns would otherwise refuse it,
// since CREATE TABLE IF NOT EXISTS never adds a column to an existing table),
// its rows must survive untouched, and the two new columns must read back as
// the empty string — which domain.ResolveTargetSpace treats as "not recorded"
// and stands its rules down for, rather than as a claim about the data.
//
// Without this test the promise rested on inspection alone, and the failure it
// guards against is the worst kind: a service that will not start.
func TestOpen_MigratesLegacyNameSpaceEntryColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	seedLegacyNameSpaceEntry(t, path)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open on a pre-rank/accepted_name database: %v — the migration must heal it, not refuse it", err)
	}
	defer func() { _ = db.Close() }()

	var name, status, rank, acceptedName string
	if err := db.sql.QueryRowContext(context.Background(), `
		SELECT name, status, rank, accepted_name FROM name_space_entry
		WHERE space = 'eurosl' AND ext_id = 'legacy-1'`).
		Scan(&name, &status, &rank, &acceptedName); err != nil {
		t.Fatalf("reading the migrated row: %v", err)
	}
	if name != "Bromopsis erecta" || status != "accepted" {
		t.Errorf("legacy row = (%q, %q), want it preserved as (%q, %q)",
			name, status, "Bromopsis erecta", "accepted")
	}
	if rank != "" {
		t.Errorf("rank = %q, want \"\" — a legacy row records no rank, and inventing one would be a claim", rank)
	}
	if acceptedName != "" {
		t.Errorf("accepted_name = %q, want \"\" for the same reason", acceptedName)
	}
}

// TestOpen_LegacyNameSpaceEntryMigrationIsIdempotent pins that opening twice
// is not an error. Open applies the migrations unconditionally on every call —
// `hostus ingest` reopens the very path `serve` already opened — so an
// ALTER TABLE that ran a second time would turn a routine restart into a
// startup failure.
func TestOpen_LegacyNameSpaceEntryMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-twice.sqlite")
	seedLegacyNameSpaceEntry(t, path)

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("closing after the first Open: %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open on an already-migrated database: %v", err)
	}
	defer func() { _ = second.Close() }()

	var n int
	if err := second.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM name_space_entry`).Scan(&n); err != nil {
		t.Fatalf("counting rows after the second Open: %v", err)
	}
	if n != 1 {
		t.Errorf("row count after reopening = %d, want the one seeded row to survive", n)
	}
}

// seedLegacyNameSpaceEntry writes a database whose name_space_entry predates
// the rank/accepted_name columns, with one row in it. The table is created
// RAW — the embedded schema would of course create the current shape.
func seedLegacyNameSpaceEntry(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(raw): %v", err)
	}
	if _, err := raw.ExecContext(ctx, legacyNameSpaceEntryDDL); err != nil {
		_ = raw.Close()
		t.Fatalf("creating the legacy name_space_entry: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO name_space_entry (space, ext_id, concept_id, name, aggregate, resolution, status)
		VALUES ('eurosl', 'legacy-1', 'wcvp:concept:400948', 'Bromopsis erecta', 0, NULL, 'accepted')`); err != nil {
		_ = raw.Close()
		t.Fatalf("seeding the legacy row: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing the raw handle: %v", err)
	}
}
