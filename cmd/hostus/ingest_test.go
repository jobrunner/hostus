package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestIngestCommand_FixtureManifest_PrintsReport drives "hostus ingest
// --dataset <fixture> --db <temp file>" end to end through the real cobra
// wiring and asserts it reports per-backbone counts a human (or T9's
// downstream automation) can read, rather than just silently succeeding.
func TestIngestCommand_FixtureManifest_PrintsReport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml", "--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "wcvp") {
		t.Errorf("report %q, want it to mention backbone %q", got, "wcvp")
	}
	// 20 taxon rows in the wcvp-sample fixture, every one gets a Name row
	// (see internal/application/ingest_test.go's TestIngest_WCVPFixture_ReportCounts,
	// which pins the same fixture's exact counts).
	if !strings.Contains(got, strconv.Itoa(20)) {
		t.Errorf("report %q, want it to mention the Names count %d", got, 20)
	}

	// The fixture manifest's trait_vocabularies section (eive, tichy2023)
	// must also print a report, including the unmatched sample — the
	// crosswalk loss must be visible on the terminal, not just in the DB.
	if !strings.Contains(got, "eive") {
		t.Errorf("report %q, want it to mention trait vocabulary %q", got, "eive")
	}
	if !strings.Contains(got, "tichy2023") {
		t.Errorf("report %q, want it to mention trait vocabulary %q", got, "tichy2023")
	}
	if !strings.Contains(got, "unmatched sample") {
		t.Errorf("report %q, want it to print the unmatched sample (Abies alba/Quercus robur are absent from the wcvp fixture)", got)
	}
	if !strings.Contains(got, "Abies alba") {
		t.Errorf("report %q, want the unmatched sample to name the specific lost taxa", got)
	}
}

// TestIngestCommand_RestrictedVocabulary_PrintsRedistributionNotice drives
// "hostus ingest" against a manifest whose eive trait vocabulary is pinned
// redistribution: unknown (testdata/dataset-restricted.yaml) and asserts
// the printed report includes the German "hinweis:" notice line — local
// ingest itself must still succeed (it is never gated), but the operator
// must SEE that this source cannot be shipped in an exported bundle without
// --force-include-restricted (see bundle_test.go's companion smoke test).
func TestIngestCommand_RestrictedVocabulary_PrintsRedistributionNotice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dataset=testdata/dataset-restricted.yaml", "--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "hinweis: eive (redistribution=unknown)") {
		t.Errorf("report %q, want a hinweis line naming eive's redistribution=unknown", got)
	}
	if !strings.Contains(got, "nicht redistribuierbar") {
		t.Errorf("report %q, want the notice to state it is not redistributable", got)
	}
}

// TestIngestCommand_MissingDatasetFlag_ReturnsError pins the "not
// implemented" stub's old exit-1 behavior (see main_test.go's TestRun,
// which invokes "hostus ingest" with no flags at all and expects exit 1):
// the real implementation must still fail, just for a different reason
// (--dataset is required), not silently succeed against an empty path.
func TestIngestCommand_MissingDatasetFlag_ReturnsError(t *testing.T) {
	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --dataset is missing, got nil")
	}
}

// TestIngestCommand_MissingDBFlag_ReturnsError confirms --db is likewise
// required: ingest must never silently pick an implicit database location.
func TestIngestCommand_MissingDBFlag_ReturnsError(t *testing.T) {
	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --db is missing, got nil")
	}
}

// TestIngestCommand_InvalidManifest_ReturnsError confirms a manifest that
// fails schema validation is rejected before any database write is
// attempted.
func TestIngestCommand_InvalidManifest_ReturnsError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset-invalid.yaml", "--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error for an invalid manifest, got nil")
	}
}

// TestIngestCommand_RegisteredOnRoot confirms "hostus ingest" is wired into
// the command tree, not just constructible in isolation.
func TestIngestCommand_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{ingestCmdName})
	if err != nil {
		t.Fatalf("Find(ingest): %v", err)
	}
	if cmd.Use != ingestCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, ingestCmdName)
	}
}
