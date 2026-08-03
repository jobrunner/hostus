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

	backboneReport, traitReports, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(backboneReport.Backbones) != 1 {
		t.Fatalf("len(Backbones) = %d, want 1", len(backboneReport.Backbones))
	}
	if len(traitReports) != 1 {
		t.Fatalf("len(traitReports) = %d, want 1 (the manifest pins one trait vocabulary)", len(traitReports))
	}

	tr := traitReports[0]
	if tr.Vocab != string(domain.VocabEIVE) {
		t.Errorf("traitReports[0].Vocab = %q, want %q", tr.Vocab, domain.VocabEIVE)
	}
	if tr.Rows == 0 {
		t.Error("traitReports[0].Rows = 0, want the fixture's rows")
	}
	if tr.Matched == 0 {
		t.Error("traitReports[0].Matched = 0, want the WCVP-resolvable fixture rows to have been written")
	}
	if tr.Unmatched == 0 {
		t.Error("traitReports[0].Unmatched = 0, want the fixture's deliberately absent taxa to be reported as lost")
	}
	if got := tr.Matched + tr.Unmatched + tr.Ambiguous; got != tr.Rows {
		t.Errorf("Matched+Unmatched+Ambiguous = %d, want %d (= Rows)", got, tr.Rows)
	}
	if len(tr.UnmatchedSample) == 0 {
		t.Error("traitReports[0].UnmatchedSample is empty, want the lossy crosswalk to name the taxa it dropped")
	}
}

func TestIngest_ManifestParseErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, _, err := app.Ingest(context.Background(), "testdata/does-not-exist.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for a missing manifest, got nil")
	}
}

func TestIngest_OpenDatabaseErrorPropagates(t *testing.T) {
	// A directory is not a usable SQLite file path.
	if _, _, err := app.Ingest(context.Background(), "testdata/dataset.yaml", t.TempDir()); err == nil {
		t.Fatal("app.Ingest: expected an error for an unopenable database path, got nil")
	}
}

func TestIngest_BackboneIngestErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	_, traitReports, err := app.Ingest(context.Background(), "testdata/dataset-bad-backbone-path.yaml", dbPath)
	if err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable backbone path, got nil")
	}
	if traitReports != nil {
		t.Errorf("traitReports = %v, want nil (trait ingest must not run after the backbone failed)", traitReports)
	}
}

func TestIngest_TraitVocabularyReadErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, _, err := app.Ingest(context.Background(), "testdata/dataset-bad-trait-path.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable trait CSV path, got nil")
	}
}

func TestIngest_UnknownTraitVocabularyIDErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, _, err := app.Ingest(context.Background(), "testdata/dataset-unknown-trait-vocab.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for a manifest pinning an unknown trait vocabulary id, got nil")
	}
}
