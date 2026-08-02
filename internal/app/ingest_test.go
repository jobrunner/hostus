package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestIngest_ReportsTraitVocabularies drives the REAL composition root
// ("hostus ingest"'s entry point) against a manifest that pins a trait
// vocabulary, on a REAL on-disk SQLite file. That combination is what makes
// this more than a happy-path assertion: the sqlite adapter runs with
// SetMaxOpenConns(1), so any repository read issued while the trait ingest
// transaction is open would deadlock here rather than fail — see
// application.IngestTraits' two-phase contract.
func TestIngest_ReportsTraitVocabularies(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.Backbone.Backbones) != 1 {
		t.Fatalf("len(Backbones) = %d, want 1", len(reports.Backbone.Backbones))
	}
	if len(reports.Traits) != 1 {
		t.Fatalf("len(reports.Traits) = %d, want 1 (the manifest pins one trait vocabulary)", len(reports.Traits))
	}

	tr := reports.Traits[0]
	if tr.Vocab != string(domain.VocabEIVE) {
		t.Errorf("reports.Traits[0].Vocab = %q, want %q", tr.Vocab, domain.VocabEIVE)
	}
	if tr.Rows == 0 {
		t.Error("reports.Traits[0].Rows = 0, want the fixture's rows")
	}
	if tr.Matched == 0 {
		t.Error("reports.Traits[0].Matched = 0, want the WCVP-resolvable fixture rows to have been written")
	}
	if tr.Unmatched == 0 {
		t.Error("reports.Traits[0].Unmatched = 0, want the fixture's deliberately absent taxa to be reported as lost")
	}
	if got := tr.Matched + tr.Unmatched + tr.Ambiguous; got != tr.Rows {
		t.Errorf("Matched+Unmatched+Ambiguous = %d, want %d (= Rows)", got, tr.Rows)
	}
	if len(tr.UnmatchedSample) == 0 {
		t.Error("reports.Traits[0].UnmatchedSample is empty, want the lossy crosswalk to name the taxa it dropped")
	}
}

// TestIngest_ReportsXrefSources mirrors TestIngest_ReportsTraitVocabularies
// for the xref-ingest leg of the same manifest (testdata/dataset.yaml now
// also pins the wikidata xref-source fixture) — same REAL on-disk SQLite
// file, same two-phase-transaction concern (application.IngestXrefs must
// never read the repository while its ingest transaction is open).
func TestIngest_ReportsXrefSources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.Xrefs) != 1 {
		t.Fatalf("len(reports.Xrefs) = %d, want 1 (the manifest pins one xref source)", len(reports.Xrefs))
	}

	xr := reports.Xrefs[0]
	if xr.Source != "wikidata" {
		t.Errorf("reports.Xrefs[0].Source = %q, want %q", xr.Source, "wikidata")
	}
	if xr.Redistribution != string(domain.RedistributionAllowed) {
		t.Errorf("reports.Xrefs[0].Redistribution = %q, want %q", xr.Redistribution, domain.RedistributionAllowed)
	}
	if xr.Rows == 0 {
		t.Error("reports.Xrefs[0].Rows = 0, want the fixture's rows")
	}
	if xr.Matched == 0 {
		t.Error("reports.Xrefs[0].Matched = 0, want the powo-resolvable fixture rows to have been written")
	}
	if got := xr.Matched + xr.Unmatched + xr.Conflicting; got != xr.Rows {
		t.Errorf("Matched+Unmatched+Conflicting = %d, want %d (= Rows)", got, xr.Rows)
	}
	if xr.PerAuthority["inat"] == 0 {
		t.Error(`reports.Xrefs[0].PerAuthority["inat"] = 0, want the fixture's inat rows to have resolved`)
	}
}

func TestIngest_XrefSourceReadErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset-bad-xref-path.yaml", dbPath)
	if err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable xref CSV path, got nil")
	}
	if len(reports.Xrefs) != 0 {
		t.Errorf("reports.Xrefs = %v, want empty (the failing source must not appear in the report)", reports.Xrefs)
	}
}

func TestIngest_ManifestParseErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, err := app.Ingest(context.Background(), "testdata/does-not-exist.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for a missing manifest, got nil")
	}
}

func TestIngest_OpenDatabaseErrorPropagates(t *testing.T) {
	// A directory is not a usable SQLite file path.
	if _, err := app.Ingest(context.Background(), "testdata/dataset.yaml", t.TempDir()); err == nil {
		t.Fatal("app.Ingest: expected an error for an unopenable database path, got nil")
	}
}

func TestIngest_BackboneIngestErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset-bad-backbone-path.yaml", dbPath)
	if err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable backbone path, got nil")
	}
	if reports.Traits != nil {
		t.Errorf("reports.Traits = %v, want nil (trait ingest must not run after the backbone failed)", reports.Traits)
	}
}

func TestIngest_TraitVocabularyReadErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, err := app.Ingest(context.Background(), "testdata/dataset-bad-trait-path.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable trait CSV path, got nil")
	}
}

func TestIngest_UnknownTraitVocabularyIDErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, err := app.Ingest(context.Background(), "testdata/dataset-unknown-trait-vocab.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for a manifest pinning an unknown trait vocabulary id, got nil")
	}
}
