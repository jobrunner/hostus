package app_test

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
)

// readCSV reads path as a parsed CSV, failing the test on any error.
func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %q: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("parsing %q: %v", path, err)
	}
	return rows
}

// TestExportCrosswalk_WritesBothCSVsAndReportsCollisions drives
// app.ExportCrosswalk against testdata/dataset-agreement.yaml — the SAME
// fixture internal/application's Critical-1 regression test uses for the
// Fall-B-/agreement-ingest path (see that file's own comment). It doubles
// as this feature's collision fixture: after ingest, "Festuca" and
// "Festuca ovina agg." each carry BOTH a Fall-A name_space_entry row
// (auto-resolved against WCVP by canonical name) AND their own native
// Fall-B eurosl concept — a genuine collision is the EXPECTED outcome
// here, not a contrived edge case. Concept ids below were read directly
// off a real ingest of this fixture, not guessed.
func TestExportCrosswalk_WritesBothCSVsAndReportsCollisions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outDir := filepath.Join(dir, "out")

	if _, err := app.Ingest(ctx, "testdata/dataset-agreement.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	report, err := app.ExportCrosswalk(ctx, dbPath, outDir)
	if err != nil {
		t.Fatalf("app.ExportCrosswalk: unexpected error: %v", err)
	}

	if report.CrosswalkRows != 5 {
		t.Errorf("report.CrosswalkRows = %d, want 5 (3 Fall A + 2 Fall B)", report.CrosswalkRows)
	}
	if report.MemberRows != 2 {
		t.Errorf("report.MemberRows = %d, want 2 (eurosl + germansl aggregate edges)", report.MemberRows)
	}
	if len(report.NameCollisions) != 2 {
		t.Fatalf("len(report.NameCollisions) = %d, want 2, got %+v", len(report.NameCollisions), report.NameCollisions)
	}
	wantCollisions := map[string][2]string{
		"Festuca":            {"wcvp:concept:451511", "eurosl:concept:e-gen1"},
		"Festuca ovina agg.": {"wcvp:concept:415853", "eurosl:concept:e-agg1"},
	}
	for _, c := range report.NameCollisions {
		want, ok := wantCollisions[c.Name]
		if !ok {
			t.Errorf("unexpected collision name %q", c.Name)
			continue
		}
		if c.FallAConceptID != want[0] || c.FallBConceptID != want[1] {
			t.Errorf("collision %q = (%q, %q), want (%q, %q)", c.Name, c.FallAConceptID, c.FallBConceptID, want[0], want[1])
		}
	}

	rows := readCSV(t, filepath.Join(outDir, "eurosl_crosswalk.csv"))
	if len(rows) != 6 { // header + 5 data rows
		t.Fatalf("eurosl_crosswalk.csv has %d rows (incl. header), want 6: %+v", len(rows), rows)
	}
	if rows[0][0] != "name" || rows[0][1] != "concept_id" {
		t.Errorf("eurosl_crosswalk.csv header = %v, want [name concept_id]", rows[0])
	}

	memberRows := readCSV(t, filepath.Join(outDir, "aggregate_members.csv"))
	if len(memberRows) != 3 { // header + 2 data rows
		t.Fatalf("aggregate_members.csv has %d rows (incl. header), want 3: %+v", len(memberRows), memberRows)
	}
	if memberRows[0][0] != "aggregate_concept_id" || memberRows[0][1] != "member_concept_id" || memberRows[0][2] != "member_name" {
		t.Errorf("aggregate_members.csv header = %v, want [aggregate_concept_id member_concept_id member_name]", memberRows[0])
	}
	foundEurosl := false
	for _, r := range memberRows[1:] {
		if r[0] == "eurosl:concept:e-agg1" {
			foundEurosl = true
			if r[1] != "wcvp:concept:415853" || r[2] != "Festuca ovina" {
				t.Errorf("eurosl aggregate row = %v, want member wcvp:concept:415853/Festuca ovina", r)
			}
		}
	}
	if !foundEurosl {
		t.Errorf("aggregate_members.csv = %+v, want a row for eurosl:concept:e-agg1", memberRows)
	}
}

// TestExportCrosswalk_CreatesMissingOutDir confirms --out-dir need not
// already exist.
func TestExportCrosswalk_CreatesMissingOutDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outDir := filepath.Join(dir, "does", "not", "exist", "yet")

	if _, err := app.Ingest(ctx, "testdata/dataset-no-namespace.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if _, err := app.ExportCrosswalk(ctx, dbPath, outDir); err != nil {
		t.Fatalf("app.ExportCrosswalk: unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "eurosl_crosswalk.csv")); err != nil {
		t.Errorf("eurosl_crosswalk.csv not created in new out-dir: %v", err)
	}
}

// TestExportCrosswalk_EmptyAggregateData_WritesHeaderOnly pins the spec's
// error table: dataset-no-namespace.yaml declares no name_spaces at all,
// so concept_aggregate stays empty -> aggregate_members.csv gets only its
// header row, never an error.
func TestExportCrosswalk_EmptyAggregateData_WritesHeaderOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	outDir := filepath.Join(dir, "out")

	if _, err := app.Ingest(ctx, "testdata/dataset-no-namespace.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	report, err := app.ExportCrosswalk(ctx, dbPath, outDir)
	if err != nil {
		t.Fatalf("app.ExportCrosswalk: unexpected error: %v", err)
	}
	if report.MemberRows != 0 {
		t.Errorf("report.MemberRows = %d, want 0", report.MemberRows)
	}
	if len(report.NameCollisions) != 0 {
		t.Errorf("report.NameCollisions = %+v, want none", report.NameCollisions)
	}
	rows := readCSV(t, filepath.Join(outDir, "aggregate_members.csv"))
	if len(rows) != 1 {
		t.Errorf("aggregate_members.csv has %d rows, want 1 (header only): %+v", len(rows), rows)
	}
}

// TestExportCrosswalk_UnopenableDatabase_ReportsNamedError mirrors
// app.Bundle's TestBundle_UnopenableDatabase_ReportsNamedError.
func TestExportCrosswalk_UnopenableDatabase_ReportsNamedError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir", "hostus.sqlite")
	report, err := app.ExportCrosswalk(context.Background(), missing, t.TempDir())
	if err == nil {
		t.Fatalf("app.ExportCrosswalk(%q): want an error naming the unopenable database", missing)
	}
	if report.CrosswalkRows != 0 {
		t.Errorf("report.CrosswalkRows = %d, want 0 on the error path", report.CrosswalkRows)
	}
}
