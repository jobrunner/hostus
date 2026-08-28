package xref_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/xref"
)

func loadFixture(t *testing.T, name string) *xref.Dataset {
	t.Helper()
	path := "testdata/" + name
	ds, err := xref.Read(path)
	if err != nil {
		t.Fatalf("Read(%q): unexpected error: %v", path, err)
	}
	return ds
}

func TestRead_WikidataSample_RowCount(t *testing.T) {
	ds := loadFixture(t, "wikidata-sample.csv")
	if got, want := len(ds.Rows), 20; got != want {
		t.Errorf("len(Rows) = %d, want %d", got, want)
	}
	if len(ds.Errors) != 0 {
		t.Errorf("Errors = %v, want none for a clean fixture", ds.Errors)
	}
}

func TestRead_WikidataSample_RowFieldsByPosition(t *testing.T) {
	ds := loadFixture(t, "wikidata-sample.csv")
	var found *xref.Row
	for i := range ds.Rows {
		r := &ds.Rows[i]
		if r.JoinID == "396681-1" && r.Authority == "inat" && r.ExtID == "160927" {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatal("no row found for join_id=396681-1 authority=inat ext_id=160927")
	}
	if got, want := found.JoinAuthority, "powo"; got != want {
		t.Errorf("JoinAuthority = %q, want %q", got, want)
	}
	if got, want := found.WikidataQID, "Q159953"; got != want {
		t.Errorf("WikidataQID = %q, want %q", got, want)
	}
}

func TestRead_MissingFile(t *testing.T) {
	if _, err := xref.Read("testdata/does-not-exist.csv"); err == nil {
		t.Fatal("Read(missing file): expected error, got nil")
	}
}

const cleanHeader = "join_authority|join_id|authority|ext_id|wikidata_qid\n"

func TestRead_MalformedRowsAreCollectedNotPanicking(t *testing.T) {
	dir := t.TempDir()
	content := cleanHeader +
		// valid row (line 2)
		"powo|396681-1|inat|160927|Q159953\n" +
		// short row: missing trailing fields (line 3)
		"powo|226649-1|gbif\n" +
		// valid row (line 4)
		"powo|331174-2|wfo|wfo-4000009405|Q2697235\n"
	path := dir + "/malformed.csv"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ds, err := xref.Read(path)
	if err != nil {
		t.Fatalf("Read(%q): unexpected fatal error: %v", path, err)
	}
	if got, want := len(ds.Rows), 2; got != want {
		t.Fatalf("len(Rows) = %d, want %d (both clean rows should survive)", got, want)
	}
	if got, want := len(ds.Errors), 1; got != want {
		t.Fatalf("len(Errors) = %d, want %d (the short row)", got, want)
	}
	if !strings.Contains(ds.Errors[0].Error(), ":3:") {
		t.Errorf("Errors[0] = %q, want it to reference source line 3", ds.Errors[0])
	}
	if !strings.Contains(ds.Errors[0].Error(), "short row") {
		t.Errorf("Errors[0] = %q, want a short-row error", ds.Errors[0])
	}
}

// TestRead_ExtraLeadingColumnShortRowIsCollectedNotPanicking pins the
// short-row guard against a header carrying EXTRA columns: FieldsPerRecord
// = -1 tolerates a prepended column, which shifts every wanted column one
// position right, and a guard of len(wantHeader) would wave a row through
// only for Read to index past it and panic.
func TestRead_ExtraLeadingColumnShortRowIsCollectedNotPanicking(t *testing.T) {
	dir := t.TempDir()
	content := "extra|" + cleanHeader +
		// full row (6 fields): parses fine despite the extra column
		"x|powo|396681-1|inat|160927|Q159953\n" +
		// 5 fields: enough for a len(wantHeader)-only guard, one short of
		// wikidata_qid at index 5 — the panic case.
		"x|powo|226649-1|gbif|5388602\n"
	path := dir + "/extra-column.csv"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ds, err := xref.Read(path)
	if err != nil {
		t.Fatalf("Read(%q): unexpected fatal error: %v", path, err)
	}
	if got, want := len(ds.Rows), 1; got != want {
		t.Fatalf("len(Rows) = %d, want %d (only the full row survives)", got, want)
	}
	if got, want := ds.Rows[0].JoinID, "396681-1"; got != want {
		t.Errorf("Rows[0].JoinID = %q, want %q (columns must be read by header position, not by ordinal)", got, want)
	}
	if got, want := len(ds.Errors), 1; got != want {
		t.Fatalf("len(Errors) = %d, want %d (the short row must be collected)", got, want)
	}
	if !strings.Contains(ds.Errors[0].Error(), "short row") {
		t.Errorf("Errors[0] = %q, want a short-row error", ds.Errors[0])
	}
	if !strings.Contains(ds.Errors[0].Error(), "want at least 6") {
		t.Errorf("Errors[0] = %q, want the guard to demand 6 fields (wikidata_qid sits at index 5)", ds.Errors[0])
	}
}

func TestRead_MissingHeaderColumn(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad-header.csv"
	if err := os.WriteFile(path, []byte("join_authority|join_id|authority|ext_id\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := xref.Read(path); err == nil {
		t.Fatal("Read: expected error for missing expected header columns, got nil")
	}
}
