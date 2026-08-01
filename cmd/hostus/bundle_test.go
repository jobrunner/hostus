package main

import (
	"bytes"
	"context"
	"database/sql"
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

// ingestRestrictedFixtureDB runs "hostus ingest" against
// testdata/dataset-restricted.yaml (eive pinned redistribution: unknown)
// into a fresh temp-file database, so bundle tests can exercise the
// redistribution gate against a database that genuinely has a non-allowed
// contributing source.
func ingestRestrictedFixtureDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset-restricted.yaml", "--db=" + dbPath})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ingesting restricted fixture: unexpected error: %v", err)
	}
	return dbPath
}

// TestBundleCommand_RestrictedSource_FailsByDefaultThenSucceedsWithForce is
// the real CLI smoke test the redistribution gate exists for: "hostus
// bundle" against a database whose eive trait vocabulary is pinned
// redistribution: unknown must FAIL by default, naming "eive" and its
// redistribution value; the identical invocation with
// --force-include-restricted must then SUCCEED, and the resulting bundle's
// bundle_meta.restricted_sources must record exactly "eive" — proving the
// bundle can never silently carry unclearable data even when the operator
// overrides the gate.
func TestBundleCommand_RestrictedSource_FailsByDefaultThenSucceedsWithForce(t *testing.T) {
	dbPath := ingestRestrictedFixtureDB(t)

	failOut := filepath.Join(t.TempDir(), "bundle-refused.sqlite")
	failCmd := newBundleCmd()
	var failStdout bytes.Buffer
	failCmd.SetOut(&failStdout)
	failCmd.SetArgs([]string{"--db=" + dbPath, "--out=" + failOut, "--snapshot=v1"})
	err := failCmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("Execute (no --force-include-restricted): want an error, got nil")
	}
	if !strings.Contains(err.Error(), "eive") {
		t.Errorf("error = %q, want it to name the offending source %q", err, "eive")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error = %q, want it to state the redistribution value %q", err, "unknown")
	}
	if _, statErr := os.Stat(failOut); statErr == nil {
		t.Errorf("bundle refused, but %q was still created", failOut)
	}

	forceOut := filepath.Join(t.TempDir(), "bundle-forced.sqlite")
	forceCmd := newBundleCmd()
	var forceStdout bytes.Buffer
	forceCmd.SetOut(&forceStdout)
	forceCmd.SetArgs([]string{"--db=" + dbPath, "--out=" + forceOut, "--snapshot=v1", "--force-include-restricted"})
	if err := forceCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute (--force-include-restricted): unexpected error: %v", err)
	}

	raw, err := sql.Open("sqlite", forceOut)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", forceOut, err)
	}
	defer func() { _ = raw.Close() }()
	var restrictedSources string
	if err := raw.QueryRow(`SELECT restricted_sources FROM bundle_meta`).Scan(&restrictedSources); err != nil {
		t.Fatalf("reading bundle_meta.restricted_sources: %v", err)
	}
	if restrictedSources != "eive" {
		t.Errorf("bundle_meta.restricted_sources = %q, want %q", restrictedSources, "eive")
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
