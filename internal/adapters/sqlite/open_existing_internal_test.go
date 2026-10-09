package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenExisting_MissingFileFailsAndCreatesNothing pins the guarantee
// OpenExisting adds over Open: it never brings the database into being.
//
// Open deliberately creates a missing file and applies schema.sql — the
// ingest depends on exactly that. Every OTHER command expects a database that
// was already ingested, and for them that helpfulness is the defect behind
// issue #82: `hostus bundle --db <typo>` invented an empty database, exported
// a contentless bundle from it and exited 0.
//
// Enforcing it in the open itself rather than with a preceding os.Stat is
// what closes the gap a check-then-open leaves: between the two, the file can
// disappear, and a writable open would recreate it. Here the refusal and the
// open are one operation.
func TestOpenExisting_MissingFileFailsAndCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.sqlite") // dir exists, file does not

	db, err := OpenExisting(path)
	if err == nil {
		_ = db.Close()
		t.Fatalf("OpenExisting(%q) = nil error, want a refusal", path)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name %q", err.Error(), path)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Errorf("OpenExisting created %q — the one thing it must never do", path)
	}
}

// TestOpenExisting_ExistingDatabaseOpensAndReads pins the other half: on a
// database that IS there, OpenExisting behaves like Open — same schema
// application, same migrations, same readable rows. A guard that also broke
// the normal case would be no improvement.
func TestOpenExisting_ExistingDatabaseOpensAndReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostus.sqlite")

	created, err := Open(path)
	if err != nil {
		t.Fatalf("Open (to create the fixture): %v", err)
	}
	if _, err := created.sql.ExecContext(context.Background(),
		`INSERT INTO backbone_version (id, version, ingested_at, manifest_sha)
		 VALUES ('wcvp', 'v1', '2026-10-09T00:00:00Z', 'x')`); err != nil {
		t.Fatalf("seeding the fixture: %v", err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("closing the fixture: %v", err)
	}

	db, err := OpenExisting(path)
	if err != nil {
		t.Fatalf("OpenExisting on an existing database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var version string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT version FROM backbone_version WHERE id = 'wcvp'`).Scan(&version); err != nil {
		t.Fatalf("reading through OpenExisting: %v", err)
	}
	if version != "v1" {
		t.Errorf("version = %q, want %q", version, "v1")
	}
}
