package app

import (
	"context"
	"fmt"
	"os"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
)

// Bundle opens the SQLite database at dbPath and exports an offline
// SQLite/FTS5 bundle to out per opts (see sqlite.ExportBundle for the
// bundle's exact contents). It is the entry point "hostus bundle" calls.
func Bundle(ctx context.Context, dbPath, out string, opts sqlite.BundleOpts) (sqlite.BundleReport, error) {
	// sqlite.Open CREATES the file (and applies schema.sql) when dbPath does
	// not exist yet — see its doc comment — so a typo'd --db is never
	// "unopenable" from Open's point of view, only silently empty. Stat it
	// first, or the command exports a technically valid but contentless
	// bundle from a database it just invented, and exits 0 (issue #82).
	//
	// ExportCrosswalk carries the identical guard for the identical reason;
	// Ingest deliberately does NOT, because creating the database is exactly
	// what that command is for.
	if _, err := os.Stat(dbPath); err != nil {
		return sqlite.BundleReport{}, fmt.Errorf("app: database %q does not exist: %w", dbPath, err)
	}

	src, err := sqlite.Open(dbPath)
	if err != nil {
		return sqlite.BundleReport{}, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = src.Close() }()

	return sqlite.ExportBundle(ctx, src, out, opts)
}
