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

// TestOpenExisting_PathWithURIMetacharacters pins the hole the first mode=rw
// implementation left: the path was pasted into a "file:" URI unescaped.
//
// SQLite parses such a URI itself, so a '?' in a legitimate filename ends the
// path and starts the query string — "file:/dir/frage?zeichen.sqlite?mode=rw"
// opens "/dir/frage", with "zeichen.sqlite" as a nonsense parameter and NO
// mode=rw in force. Measured before the fix: the open succeeded and created
// "/dir/frage", which is precisely the empty-database behavior this guard
// exists to prevent. '#' does the same via fragment syntax, and a literal '%'
// would be read as the start of an escape.
//
// Filenames like these are unusual but perfectly legal on every filesystem
// hostus runs on, and the failure is silent — the worst combination.
func TestOpenExisting_PathWithURIMetacharacters(t *testing.T) {
	for _, name := range []string{
		"frage?zeichen.sqlite",
		"raute#test.sqlite",
		"prozent%3Fliteral.sqlite",
		"alle?drei#zusammen%.sqlite",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, name)

			assertRefusesAndCreatesNothing(t, dir, path)
			assertOpensTheSameFile(t, path)
		})
	}
}

// TestOpenExisting_RejectsInMemory pins that ":memory:" is refused rather than
// quietly honored. Through a "file:" URI it opens a fresh in-memory database
// — no file, no error, and nothing of the ingested data the caller asked to
// bundle. "Open an existing database" has no meaning for a target that is
// created empty by definition, so saying so beats answering with an empty one.
func TestOpenExisting_RejectsInMemory(t *testing.T) {
	db, err := OpenExisting(":memory:")
	if err == nil {
		_ = db.Close()
		t.Fatal("OpenExisting(\":memory:\") = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), ":memory:") {
		t.Errorf("error = %q, want it to name the rejected target", err.Error())
	}
}

// assertRefusesAndCreatesNothing is the missing-file half: the open must fail
// and the directory must stay empty — not even a truncated path may appear,
// which is what a split URI would produce.
func assertRefusesAndCreatesNothing(t *testing.T, dir, path string) {
	t.Helper()
	db, err := OpenExisting(path)
	if err == nil {
		_ = db.Close()
		t.Fatalf("OpenExisting(%q) = nil error, want a refusal", path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %q: %v", dir, err)
	}
	if len(entries) == 0 {
		return
	}
	created := make([]string, 0, len(entries))
	for _, e := range entries {
		created = append(created, e.Name())
	}
	t.Errorf("OpenExisting created %v in %q — the URI was split and mode=rw lost", created, dir)
}

// assertOpensTheSameFile is the present-file half: escaping has to address the
// file the caller named, not an encoded neighbor of it.
func assertOpensTheSameFile(t *testing.T, path string) {
	t.Helper()
	seed, err := Open(path)
	if err != nil {
		t.Fatalf("Open (to create the fixture): %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("closing the fixture: %v", err)
	}
	reopened, err := OpenExisting(path)
	if err != nil {
		t.Fatalf("OpenExisting on the existing %q: %v", path, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
}
