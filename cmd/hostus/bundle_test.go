package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ingestFixtureDB runs "hostus ingest" against the shared testdata fixture
// into a fresh temp-file database, returning its path, so bundle tests
// exercise a real, previously-ingested database rather than an empty one.
func ingestFixtureDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml", "--db=" + dbPath})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ingesting fixture: unexpected error: %v", err)
	}
	return dbPath
}

// TestBundleCommand_WritesNonEmptyBundleAndPrintsReport drives "hostus
// bundle --db <fixture> --area AUT --out <path>" end to end through the
// real cobra wiring: the output file must exist and be non-empty, and the
// printed report must mention the counts an operator (or downstream
// automation) reads.
func TestBundleCommand_WritesNonEmptyBundleAndPrintsReport(t *testing.T) {
	dbPath := ingestFixtureDB(t)
	outPath := filepath.Join(t.TempDir(), "bundle.sqlite")

	cmd := newBundleCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db=" + dbPath, "--area=AUT", "--out=" + outPath, "--snapshot=v1"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat bundle output %q: %v", outPath, err)
	}
	if info.Size() == 0 {
		t.Fatalf("bundle output %q is empty, want a populated SQLite file", outPath)
	}

	got := out.String()
	if !strings.Contains(got, outPath) {
		t.Errorf("report %q, want it to mention the output path %q", got, outPath)
	}
	// The fixture has exactly 3 accepted concepts with an AUT distribution
	// row (Corynephorus canescens 405825, Festuca ovina 415853, Jacobaea
	// vulgaris 3082777 — see internal/adapters/wcvp/testdata/wcvp-sample/wcvp_distribution.csv),
	// so the report's concept count is exactly 3.
	if !strings.Contains(got, "concepts="+strconv.Itoa(3)) {
		t.Errorf("report %q, want it to mention concepts=3", got)
	}
}

// TestBundleCommand_MissingDBFlag_ReturnsError confirms --db is required:
// bundle must never silently pick an implicit source database.
func TestBundleCommand_MissingDBFlag_ReturnsError(t *testing.T) {
	cmd := newBundleCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--out=" + filepath.Join(t.TempDir(), "bundle.sqlite")})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --db is missing, got nil")
	}
}

// TestBundleCommand_MissingOutFlag_ReturnsError confirms --out is likewise
// required.
func TestBundleCommand_MissingOutFlag_ReturnsError(t *testing.T) {
	dbPath := ingestFixtureDB(t)

	cmd := newBundleCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --out is missing, got nil")
	}
}

// TestBundleCommand_RegisteredOnRoot confirms "hostus bundle" is wired into
// the command tree, not just constructible in isolation.
func TestBundleCommand_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{bundleCmdName})
	if err != nil {
		t.Fatalf("Find(bundle): %v", err)
	}
	if cmd.Use != bundleCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, bundleCmdName)
	}
}
