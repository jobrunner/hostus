package traits_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/traits"
)

func loadFixture(t *testing.T, name string) *traits.Dataset {
	t.Helper()
	path := "testdata/" + name
	ds, err := traits.Read(path)
	if err != nil {
		t.Fatalf("Read(%q): unexpected error: %v", path, err)
	}
	return ds
}

func findRow(t *testing.T, ds *traits.Dataset, taxon, dim string) *traits.Row {
	t.Helper()
	for i := range ds.Rows {
		if ds.Rows[i].Taxon == taxon && ds.Rows[i].Dim == dim {
			return &ds.Rows[i]
		}
	}
	t.Fatalf("no row found for taxon %q dim %q", taxon, dim)
	return nil
}

func TestRead_EIVESample_RowCounts(t *testing.T) {
	ds := loadFixture(t, "eive-sample.csv")
	if got, want := len(ds.Rows), 25; got != want {
		t.Errorf("len(Rows) = %d, want %d", got, want)
	}
	if len(ds.Errors) != 0 {
		t.Errorf("Errors = %v, want none for a clean fixture", ds.Errors)
	}
}

func TestRead_EIVERow_HasNicheWidthAndNSystems(t *testing.T) {
	ds := loadFixture(t, "eive-sample.csv")
	row := findRow(t, ds, "Corynephorus canescens", "M")

	if got, want := row.Vocab, "eive"; got != want {
		t.Errorf("Vocab = %q, want %q", got, want)
	}
	if got, want := row.VocabVersion, "1.0"; got != want {
		t.Errorf("VocabVersion = %q, want %q", got, want)
	}
	if row.NicheWidth == nil {
		t.Fatal("NicheWidth = nil, want non-nil for an EIVE row")
	}
	if got, want := *row.NicheWidth, 3.42710384303395; got != want {
		t.Errorf("*NicheWidth = %v, want %v", got, want)
	}
	if row.NSystems == nil {
		t.Fatal("NSystems = nil, want non-nil for an EIVE row")
	}
	if got, want := *row.NSystems, 20; got != want {
		t.Errorf("*NSystems = %d, want %d", got, want)
	}
}

func TestRead_TichySample_RowCounts(t *testing.T) {
	ds := loadFixture(t, "tichy-sample.csv")
	if got, want := len(ds.Rows), 25; got != want {
		t.Errorf("len(Rows) = %d, want %d", got, want)
	}
	if len(ds.Errors) != 0 {
		t.Errorf("Errors = %v, want none for a clean fixture", ds.Errors)
	}
}

func TestRead_TichyRow_HasNilNicheWidthButRealValue(t *testing.T) {
	ds := loadFixture(t, "tichy-sample.csv")
	row := findRow(t, ds, "Corynephorus canescens", "L")

	if got, want := row.Vocab, "tichy2023"; got != want {
		t.Errorf("Vocab = %q, want %q", got, want)
	}
	if got, want := row.Value, 8.4; got != want {
		t.Errorf("Value = %v, want %v", got, want)
	}
	if row.NicheWidth != nil {
		t.Errorf("NicheWidth = %v, want nil (Tichy does not provide niche width)", *row.NicheWidth)
	}
	if row.NSystems != nil {
		t.Errorf("NSystems = %v, want nil (Tichy does not provide n_systems)", *row.NSystems)
	}
}

func TestRead_MidoloSample_RowCounts(t *testing.T) {
	ds := loadFixture(t, "midolo-sample.csv")
	if got, want := len(ds.Rows), 25; got != want {
		t.Errorf("len(Rows) = %d, want %d", got, want)
	}
	if len(ds.Errors) != 0 {
		t.Errorf("Errors = %v, want none for a clean fixture", ds.Errors)
	}
}

func TestRead_MidoloRow_ParsesDisturbanceDim(t *testing.T) {
	ds := loadFixture(t, "midolo-sample.csv")
	row := findRow(t, ds, "Abies alba", "disturbance_severity")

	if got, want := row.Vocab, "midolo2023"; got != want {
		t.Errorf("Vocab = %q, want %q", got, want)
	}
	if got, want := row.Value, 0.6958; got != want {
		t.Errorf("Value = %v, want %v", got, want)
	}
	if row.NicheWidth != nil {
		t.Errorf("NicheWidth = %v, want nil (Midolo does not provide niche width)", *row.NicheWidth)
	}
	if row.NSystems != nil {
		t.Errorf("NSystems = %v, want nil (Midolo does not provide n_systems)", *row.NSystems)
	}
}

func TestRead_MissingFile(t *testing.T) {
	if _, err := traits.Read("testdata/does-not-exist.csv"); err == nil {
		t.Fatal("Read(missing file): expected error, got nil")
	}
}

const cleanHeader = "taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"

func TestRead_MalformedRowsAreCollectedNotPanicking(t *testing.T) {
	dir := t.TempDir()
	content := cleanHeader +
		// valid row (line 2)
		"Corynephorus canescens|eive|1.0|M|2.48|3.42|20\n" +
		// short row: missing trailing fields (line 3)
		"Festuca ovina|eive|1.0\n" +
		// non-numeric value (line 4)
		"Jacobaea vulgaris|eive|1.0|N|not-a-number|3.5|18\n" +
		// bad niche_width (line 5)
		"Abies alba|eive|1.0|R|4.1|not-a-float|15\n" +
		// bad n_systems (line 6)
		"Quercus robur|eive|1.0|L|6.2|4.0|not-an-int\n"
	path := dir + "/malformed.csv"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ds, err := traits.Read(path)
	if err != nil {
		t.Fatalf("Read(%q): unexpected fatal error: %v", path, err)
	}
	if got, want := len(ds.Rows), 1; got != want {
		t.Fatalf("len(Rows) = %d, want %d (only the clean row should survive)", got, want)
	}
	if got, want := len(ds.Errors), 4; got != want {
		t.Fatalf("len(Errors) = %d, want %d (short row + bad value + bad niche_width + bad n_systems)", got, want)
	}
	wantLines := []string{":3:", ":4:", ":5:", ":6:"}
	for i, want := range wantLines {
		if !strings.Contains(ds.Errors[i].Error(), want) {
			t.Errorf("Errors[%d] = %q, want it to reference source line %q", i, ds.Errors[i], want)
		}
	}
}

// TestRead_ExtraLeadingColumnShortRowIsCollectedNotPanicking pins the
// short-row guard against a header carrying EXTRA columns. The reader sets
// FieldsPerRecord = -1 precisely to tolerate ragged input, so a pipeline
// that prepends a column is legal — but it shifts every wanted column one
// position right, and a guard of len(wantHeader) (7) would then wave a
// 7-field row through only for parseRow to index row[7] and panic. The
// documented posture is "collect the bad row in Errors, never panic".
func TestRead_ExtraLeadingColumnShortRowIsCollectedNotPanicking(t *testing.T) {
	dir := t.TempDir()
	content := "extra|" + cleanHeader +
		// full row (8 fields): parses fine despite the extra column
		"x|Corynephorus canescens|eive|1.0|M|2.48|3.42|20\n" +
		// 7 fields: enough for the OLD guard, one short of n_systems at
		// index 7 — the panic case.
		"x|Festuca ovina|eive|1.0|N|3.1|2.0\n"
	path := dir + "/extra-column.csv"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ds, err := traits.Read(path)
	if err != nil {
		t.Fatalf("Read(%q): unexpected fatal error: %v", path, err)
	}
	if got, want := len(ds.Rows), 1; got != want {
		t.Fatalf("len(Rows) = %d, want %d (only the full row survives)", got, want)
	}
	if got, want := ds.Rows[0].Taxon, "Corynephorus canescens"; got != want {
		t.Errorf("Rows[0].Taxon = %q, want %q (columns must be read by header position, not by ordinal)", got, want)
	}
	if got, want := len(ds.Errors), 1; got != want {
		t.Fatalf("len(Errors) = %d, want %d (the short row must be collected)", got, want)
	}
	if !strings.Contains(ds.Errors[0].Error(), "short row") {
		t.Errorf("Errors[0] = %q, want a short-row error", ds.Errors[0])
	}
	if !strings.Contains(ds.Errors[0].Error(), "want at least 8") {
		t.Errorf("Errors[0] = %q, want the guard to demand 8 fields (n_systems sits at index 7)", ds.Errors[0])
	}
}

func TestRead_MissingHeaderColumn(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad-header.csv"
	if err := os.WriteFile(path, []byte("taxon|vocab|dim|value\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := traits.Read(path); err == nil {
		t.Fatal("Read: expected error for missing expected header columns, got nil")
	}
}
