package app

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
)

// Bundle opens the SQLite database at dbPath and exports an offline
// SQLite/FTS5 bundle to out per opts (see sqlite.ExportBundle for the
// bundle's exact contents). It is the entry point "hostus bundle" calls.
func Bundle(ctx context.Context, dbPath, out string, opts sqlite.BundleOpts) (sqlite.BundleReport, error) {
	src, err := sqlite.Open(dbPath)
	if err != nil {
		return sqlite.BundleReport{}, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = src.Close() }()

	return sqlite.ExportBundle(ctx, src, out, opts)
}
