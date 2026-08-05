package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/app"
)

// TestBundle_ExportsFromIngestedDatabase covers app.Bundle, the composition
// root's entry point for "hostus bundle", in the DEFAULT (untagged) suite.
//
// It deliberately uses dataset-no-namespace.yaml — the same manifest as
// dataset.yaml minus the FloraVeg name space — because this test is about
// app.Bundle's happy path, and every source it pins is redistribution=allowed.
// The redistribution gate's behavior on a NON-allowed source is pinned
// separately by TestBundle_RefusesNameSpaceByDefault below.
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

	if _, err := app.Ingest(ctx, "testdata/dataset-no-namespace.yaml", dbPath); err != nil {
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

// TestBundle_RefusesNameSpaceByDefault drives the redistribution gate through
// the WHOLE composition root — manifest parse, real ingest, real export — for
// the SP9 name space, rather than trusting the adapter-level test alone.
//
// That distinction is the point. SP4's review found the gate had a hole
// precisely because every task was locally correct: each new kind of source
// was gated in its own unit test while nothing checked that "hostus ingest"
// followed by "hostus bundle" actually refused. FloraVeg's redistribution is
// "unknown", so this must refuse by default and must succeed — recording the
// source — under --force-include-restricted.
func TestBundle_RefusesNameSpaceByDefault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")

	if _, err := app.Ingest(ctx, "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	refusedPath := filepath.Join(dir, "refused.sqlite")
	_, err := app.Bundle(ctx, dbPath, refusedPath, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err == nil {
		t.Fatal("app.Bundle: want a refusal for the redistribution=unknown FloraVeg name space, got nil")
	}
	if !strings.Contains(err.Error(), "floraveg (redistribution=unknown)") {
		t.Errorf("app.Bundle error = %q, want it to name the offending name space and its value", err)
	}
	if _, statErr := os.Stat(refusedPath); statErr == nil {
		t.Errorf("app.Bundle refused, but %q was still created", refusedPath)
	}

	forcedPath := filepath.Join(dir, "forced.sqlite")
	if _, err := app.Bundle(ctx, dbPath, forcedPath, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", AllowRestricted: true,
	}); err != nil {
		t.Fatalf("app.Bundle(AllowRestricted): unexpected error: %v", err)
	}

	bundle, err := sqlite.Open(forcedPath)
	if err != nil {
		t.Fatalf("sqlite.Open(%q): unexpected error: %v", forcedPath, err)
	}
	defer func() { _ = bundle.Close() }()
	spaces, err := bundle.NameSpaces(ctx)
	if err != nil {
		t.Fatalf("NameSpaces: unexpected error: %v", err)
	}
	if len(spaces) != 1 || spaces[0].ID != "floraveg" {
		t.Errorf("forced bundle NameSpaces = %+v, want exactly floraveg", spaces)
	}
}
