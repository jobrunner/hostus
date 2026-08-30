package app_test

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
)

// readCSV reads path as a parsed CSV, failing the test on any error.
func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(filepath.Clean(path))
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
	assertWantedCollisions(t, report.NameCollisions)

	assertCrosswalkCSV(t, filepath.Join(outDir, "eurosl_crosswalk.csv"))
	assertAggregateMembersCSV(t, filepath.Join(outDir, "aggregate_members.csv"))
}

// assertWantedCollisions checks got against the two collisions this
// fixture is known to produce (see the test's doc comment).
func assertWantedCollisions(t *testing.T, got []app.CrosswalkCollision) {
	t.Helper()
	wantCollisions := map[string][2]string{
		"Festuca":            {"wcvp:concept:451511", "eurosl:concept:e-gen1"},
		"Festuca ovina agg.": {"wcvp:concept:415853", "eurosl:concept:e-agg1"},
	}
	for _, c := range got {
		want, ok := wantCollisions[c.Name]
		if !ok {
			t.Errorf("unexpected collision name %q", c.Name)
			continue
		}
		if c.FallAConceptID != want[0] || c.FallBConceptID != want[1] {
			t.Errorf("collision %q = (%q, %q), want (%q, %q)", c.Name, c.FallAConceptID, c.FallBConceptID, want[0], want[1])
		}
	}
}

// assertCrosswalkCSV checks eurosl_crosswalk.csv's header and its full,
// exact row order: all 3 Fall A rows (by name), then all 2 Fall B rows (by
// name) — a plain concatenation, never a merge/re-sort across the two
// sources. This locks in writeCrosswalkCSV's determinism promise (spec's
// UNION, not a merge that could hide a collision). Row values were read
// directly off a real ingest of dataset-agreement.yaml (see this test
// file's TestExportCrosswalk_WritesBothCSVsAndReportsCollisions doc
// comment), not guessed.
func assertCrosswalkCSV(t *testing.T, path string) {
	t.Helper()
	rows := readCSV(t, path)
	want := [][]string{
		{"name", "concept_id"},
		{"Festuca", "wcvp:concept:451511"},
		{"Festuca ovina", "wcvp:concept:415853"},
		{"Festuca ovina agg.", "wcvp:concept:415853"},
		{"Festuca", "eurosl:concept:e-gen1"},
		{"Festuca ovina agg.", "eurosl:concept:e-agg1"},
	}
	if len(rows) != len(want) {
		t.Fatalf("eurosl_crosswalk.csv has %d rows (incl. header), want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i][0] != want[i][0] || rows[i][1] != want[i][1] {
			t.Errorf("eurosl_crosswalk.csv row %d = %v, want %v", i, rows[i], want[i])
		}
	}
}

// assertAggregateMembersCSV checks aggregate_members.csv's header, row
// count, and the eurosl aggregate's member row.
func assertAggregateMembersCSV(t *testing.T, path string) {
	t.Helper()
	memberRows := readCSV(t, path)
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

// TestExportCrosswalk_NonexistentDatabaseFile_ReportsNamedError pins the gap
// TestExportCrosswalk_UnopenableDatabase_ReportsNamedError misses: a --db
// path in an EXISTING directory that simply has no file at that name yet.
// sqlite.Open CREATES the file and applies schema.sql for a path that does
// not exist (see internal/adapters/sqlite/db.go), so this is never
// "unopenable" from Open's point of view — without an explicit existence
// check, a typo'd --db silently produces two empty, header-only CSVs and
// exits 0 (spec's error table: "DB nicht lesbar -> Fehler, Befehl bricht
// ab").
func TestExportCrosswalk_NonexistentDatabaseFile_ReportsNamedError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "typo.sqlite") // dir exists, file does not
	outDir := filepath.Join(t.TempDir(), "out")

	report, err := app.ExportCrosswalk(context.Background(), dbPath, outDir)
	if err == nil {
		t.Fatalf("app.ExportCrosswalk(%q): want an error, got nil (report=%+v)", dbPath, report)
	}
	if !strings.Contains(err.Error(), dbPath) {
		t.Errorf("app.ExportCrosswalk error = %q, want it to mention %q", err.Error(), dbPath)
	}
	if report.CrosswalkRows != 0 || report.MemberRows != 0 || len(report.NameCollisions) != 0 {
		t.Errorf("app.ExportCrosswalk report = %+v, want the zero value on the error path", report)
	}
	if _, statErr := os.Stat(dbPath); statErr == nil {
		t.Errorf("app.ExportCrosswalk must not create %q on the error path", dbPath)
	}
	for _, name := range []string{"eurosl_crosswalk.csv", "aggregate_members.csv"} {
		if _, statErr := os.Stat(filepath.Join(outDir, name)); statErr == nil {
			t.Errorf("app.ExportCrosswalk must not create %q on the error path", name)
		}
	}
}
