package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestIngestCDMFixtureEndToEnd drives the whole SP5 path — manifest ->
// cdm reader -> application.IngestCDM -> sqlite — over the committed
// 18-concept/14-relation fixture. The real 51.466-concept crawl is a 16–20 h
// job and is deliberately never a test dependency; the numbers asserted here
// are the fixture's, and the full-scale run is Task 5's business.
func TestIngestCDMFixtureEndToEnd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")
	reports, err := app.Ingest(context.Background(), "testdata/dataset-cdm.yaml", dbPath)
	if err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}
	if len(reports.ConceptSources) != 1 {
		t.Fatalf("got %d concept-source reports, want 1", len(reports.ConceptSources))
	}
	r := reports.ConceptSources[0]

	if r.Concepts != 18 || r.ConceptsWritten != 18 {
		t.Errorf("concepts read/written = %d/%d, want 18/18", r.Concepts, r.ConceptsWritten)
	}
	// 17 of the 18 fixture concepts carry a sec.; one (Sisymbrium
	// pyrenaicum) has none, which is honest absence, not an error.
	if r.SecReferences != 10 {
		t.Errorf("SecReferences = %d, want 10", r.SecReferences)
	}
	if r.ConceptsWithoutSec != 1 {
		t.Errorf("ConceptsWithoutSec = %d, want 1", r.ConceptsWithoutSec)
	}
	if r.Relations != 14 {
		t.Errorf("Relations = %d, want 14", r.Relations)
	}
	// One of the 14 is a misapplied-name row (conceptRelationship=false):
	// dropped by the documented rule, so 13 land.
	if r.NonConcept != 1 {
		t.Errorf("NonConcept = %d, want 1 (the misapplied-name row)", r.NonConcept)
	}
	if r.RelationsWritten != 13 {
		t.Errorf("RelationsWritten = %d, want 13", r.RelationsWritten)
	}
	if r.ReaderErrors != 0 {
		t.Errorf("ReaderErrors = %d, want 0", r.ReaderErrors)
	}
	if r.Redistribution != string(domain.RedistributionUnknown) {
		t.Errorf("Redistribution = %q, want unknown", r.Redistribution)
	}
	assertCDMRelationTypes(t, r.PerRelationType)
	// The fixture's only parent_uuid points outside the 18 rows.
	if r.UnresolvedParents != 1 {
		t.Errorf("UnresolvedParents = %d, want 1", r.UnresolvedParents)
	}
	if r.UnresolvedEnds != 0 {
		t.Errorf("UnresolvedEnds = %d, want 0", r.UnresolvedEnds)
	}
}

// assertCDMRelationTypes pins the per-type breakdown of the fixture's 13
// written relations, and that the misapplied-name row is not among them.
func assertCDMRelationTypes(t *testing.T, perType map[string]int) {
	t.Helper()
	for rel, want := range map[string]int{
		string(domain.RelationCongruent): 9,
		string(domain.RelationIncludes):  1,
		string(domain.RelationOverlaps):  1,
		string(domain.RelationUncertain): 1,
		string(domain.RelationProParte):  1,
	} {
		if perType[rel] != want {
			t.Errorf("PerRelationType[%s] = %d, want %d", rel, perType[rel], want)
		}
	}
	if _, ok := perType[string(domain.RelationMisapplied)]; ok {
		t.Error("a misapplied-name row must never reach concept_relation")
	}
}

func TestIngestCDMFailsOnAnUnreadableConceptCSV(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")
	if _, err := app.Ingest(context.Background(), "testdata/dataset-cdm-bad-path.yaml", dbPath); err == nil {
		t.Fatal("want an error for a concept source whose CSV does not exist")
	}
}

func TestIngestCDMSurfacesReaderErrorsWithoutAborting(t *testing.T) {
	// A damaged artifact must be VISIBLE, not fatal: the CSVs are the output
	// of a 16–20 h crawl, so one bad line costs that line and nothing else.
	// The count sums BOTH readers — a concepts-side error and a
	// relations-side one.
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")
	reports, err := app.Ingest(context.Background(), "testdata/dataset-cdm-broken.yaml", dbPath)
	if err != nil {
		t.Fatalf("Ingest must not fail on malformed rows: %v", err)
	}
	r := reports.ConceptSources[0]
	if r.ReaderErrors != 2 {
		t.Errorf("ReaderErrors = %d, want 2 (one per CSV)", r.ReaderErrors)
	}
	if r.ConceptsWritten != 1 {
		t.Errorf("ConceptsWritten = %d, want 1 (the good row)", r.ConceptsWritten)
	}
	if r.RelationsWritten != 0 {
		t.Errorf("RelationsWritten = %d, want 0 (the only relation row was unreadable)", r.RelationsWritten)
	}
}
