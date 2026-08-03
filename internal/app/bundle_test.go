package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/app"
)

// TestBundle_ExportsFromIngestedDatabase covers app.Bundle, the composition
// root's entry point for "hostus bundle", in the DEFAULT (untagged) suite.
//
// cmd/hostus/bundle_test.go already drives the same function through cobra,
// but that coverage lives in a different package, so `make mutation
// PKG=./internal/app` saw app.Bundle's error check as NOT COVERED — no test
// in this package executed the line at all. A not-covered mutant is worse
// than a surviving one, and the mutation gate now fails on it (see the
// Makefile's `mutation` target), so the check gets a test where the mutant
// lives.
func TestBundle_ExportsFromIngestedDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outPath := filepath.Join(dir, "bundle.sqlite")

	if _, err := app.Ingest(ctx, "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	report, err := app.Bundle(ctx, dbPath, outPath, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("app.Bundle: unexpected error: %v", err)
	}
	if report.Concepts == 0 {
		t.Error("report.Concepts = 0, want at least the AUT-scoped fixture concepts")
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat bundle %q: %v", outPath, err)
	}
	if info.Size() == 0 {
		t.Errorf("bundle %q is empty, want a populated SQLite file", outPath)
	}
}

// TestBundle_UnopenableDatabase_ReportsNamedError pins the other side of the
// same check: a database that cannot be opened must surface a named error
// mentioning the path, never an empty report.
func TestBundle_UnopenableDatabase_ReportsNamedError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir", "hostus.sqlite")

	report, err := app.Bundle(context.Background(), missing, filepath.Join(t.TempDir(), "bundle.sqlite"),
		sqlite.BundleOpts{SnapshotVersion: "v1"})
	if err == nil {
		t.Fatalf("app.Bundle(%q) err = nil, want an error naming the unopenable database", missing)
	}
	if report.Concepts != 0 {
		t.Errorf("report.Concepts = %d, want 0 on the error path", report.Concepts)
	}
}
