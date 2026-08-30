package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportCrosswalkCommand_WritesBothFilesAndPrintsReport drives
// "hostus export-crosswalk --db <fixture> --out-dir <dir>" end to end
// through the real cobra wiring. dataset-no-namespace.yaml has no
// name_spaces at all, so this is a minimal but fully deterministic happy
// path (crosswalk_rows=0 member_rows=0 collisions=0) — the richer
// collision-bearing case is already covered at the app layer
// (internal/app/export_crosswalk_test.go), so this test only proves the
// CLI wiring, not the business logic again.
func TestExportCrosswalkCommand_WritesBothFilesAndPrintsReport(t *testing.T) {
	dbPath := ingestFixtureDB(t)
	outDir := filepath.Join(t.TempDir(), "out")

	cmd := newExportCrosswalkCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db=" + dbPath, "--out-dir=" + outDir})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	for _, name := range []string{"eurosl_crosswalk.csv", "aggregate_members.csv"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("stat %q: %v", name, err)
		}
	}

	got := out.String()
	if !strings.Contains(got, outDir) {
		t.Errorf("report %q, want it to mention the output directory %q", got, outDir)
	}
	if !strings.Contains(got, "crosswalk_rows=0") {
		t.Errorf("report %q, want it to mention crosswalk_rows=0", got)
	}
}

// TestExportCrosswalkCommand_MissingDBFlag_ReturnsError confirms --db is
// required.
func TestExportCrosswalkCommand_MissingDBFlag_ReturnsError(t *testing.T) {
	cmd := newExportCrosswalkCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--out-dir=" + filepath.Join(t.TempDir(), "out")})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --db is missing, got nil")
	}
}

// TestExportCrosswalkCommand_MissingOutDirFlag_ReturnsError confirms
// --out-dir is likewise required.
func TestExportCrosswalkCommand_MissingOutDirFlag_ReturnsError(t *testing.T) {
	dbPath := ingestFixtureDB(t)

	cmd := newExportCrosswalkCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --out-dir is missing, got nil")
	}
}

// TestExportCrosswalkCommand_RegisteredOnRoot confirms "hostus
// export-crosswalk" is wired into the command tree, not just constructible
// in isolation.
func TestExportCrosswalkCommand_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{exportCrosswalkCmdName})
	if err != nil {
		t.Fatalf("Find(export-crosswalk): %v", err)
	}
	if cmd.Use != exportCrosswalkCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, exportCrosswalkCmdName)
	}
}
