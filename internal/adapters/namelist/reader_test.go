package namelist_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/namelist"
)

// TestRead_RoundTripsEveryColumn pins that a FloraVeg row survives the decode
// with every column intact and in the source's own spelling — including the
// empty rank column FloraVeg's Life_form table simply does not have, which
// must stay empty rather than being invented.
func TestRead_RoundTripsEveryColumn(t *testing.T) {
	t.Parallel()

	ds, err := namelist.Read(filepath.Join("testdata", "floraveg-sample.csv"))
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	if len(ds.Errors) != 0 {
		t.Fatalf("Read: unexpected row errors: %v", ds.Errors)
	}
	if got, want := len(ds.Rows), 5; got != want {
		t.Fatalf("Read: got %d rows, want %d", got, want)
	}

	// The source document's own UC4 example, verbatim: three FloraVeg
	// spellings of Festuca ovina under three distinct SeqIDs.
	want := []namelist.Row{
		{Taxon: "Abies alba", Status: "accepted", SourceID: "2"},
		{Taxon: "Festuca ovina", Status: "accepted", SourceID: "5647"},
		{Taxon: "Festuca ovina aggr.", Status: "accepted", SourceID: "5648"},
		{Taxon: "Festuca ovina s. l.", Status: "accepted", SourceID: "5649"},
		{Taxon: "Acer opalus aggr.", Status: "accepted", SourceID: "81"},
	}
	for i, w := range want {
		if ds.Rows[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, ds.Rows[i], w)
		}
	}
}

// TestRead_BadRowsAreCollectedNotDropped pins the standing loss rule: a row
// the reader cannot use is skipped, but it lands in Errors so the count is
// visible — never silently discarded.
func TestRead_BadRowsAreCollectedNotDropped(t *testing.T) {
	t.Parallel()

	ds, err := namelist.Read(filepath.Join("testdata", "floraveg-broken.csv"))
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	if got, want := len(ds.Rows), 2; got != want {
		t.Fatalf("Read: got %d usable rows, want %d (%+v)", got, want, ds.Rows)
	}
	if ds.Rows[0].Taxon != "Abies alba" || ds.Rows[1].Taxon != "Quercus robur" {
		t.Errorf("Read: usable rows = %+v, want Abies alba + Quercus robur", ds.Rows)
	}
	if got, want := len(ds.Errors), 4; got != want {
		t.Fatalf("Read: got %d row errors, want %d (%v)", got, want, ds.Errors)
	}

	parts := make([]string, len(ds.Errors))
	for i, e := range ds.Errors {
		parts[i] = e.Error()
	}
	joined := strings.Join(parts, "\n")
	for _, want := range []string{"empty taxon", "empty source_id", "short row"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Read: errors %q do not mention %q", joined, want)
		}
	}
	// Every error names its line, so an operator can find the offending row.
	// Line 6 is a FOUR-field record: legal-looking, but one field short of
	// the contract's highest expected column (source_id at index 4). It must
	// be rejected as a short row rather than read past the end of the slice
	// — which is what pins minFieldsFor computing the MAX index, not a
	// smaller number.
	for _, want := range []string{"line 3", "line 4", "line 5", "line 6"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Read: errors %q do not mention %q", joined, want)
		}
	}
}

// TestRead_MissingColumnIsFatal pins that a header lacking a contract column
// fails the whole read: every row would produce the identical error, so this
// is a manifest/pipeline mistake, not per-row loss.
func TestRead_MissingColumnIsFatal(t *testing.T) {
	t.Parallel()

	_, err := namelist.Read(filepath.Join("testdata", "wrong-header.csv"))
	if err == nil {
		t.Fatal("Read: want error for a header missing accepted_taxon, got nil")
	}
	if !strings.Contains(err.Error(), "accepted_taxon") {
		t.Errorf("Read: error %q does not name the missing column", err)
	}
}

func TestRead_MissingFileIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := namelist.Read(filepath.Join("testdata", "does-not-exist.csv")); err == nil {
		t.Fatal("Read: want error for a missing file, got nil")
	}
}

// TestRead_EmptyFileIsAnError pins that a truncated/zero-byte artifact fails
// at the header read rather than being reported as a name space with zero
// names.
func TestRead_EmptyFileIsAnError(t *testing.T) {
	t.Parallel()

	empty := filepath.Join(t.TempDir(), "empty.csv")
	if err := writeFile(empty, ""); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := namelist.Read(empty); err == nil {
		t.Fatal("Read: want error for an empty file, got nil")
	}
}

// TestRead_HeaderOnlyIsAnEmptyDataset pins the other side of the boundary: a
// well-formed file with no data rows is legal and yields zero rows and zero
// errors, which is a different outcome from a truncated file.
func TestRead_HeaderOnlyIsAnEmptyDataset(t *testing.T) {
	t.Parallel()

	headerOnly := filepath.Join(t.TempDir(), "header-only.csv")
	if err := writeFile(headerOnly, "taxon|rank|status|accepted_taxon|source_id\n"); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	ds, err := namelist.Read(headerOnly)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	if len(ds.Rows) != 0 || len(ds.Errors) != 0 {
		t.Errorf("Read: got %d rows / %d errors, want 0/0", len(ds.Rows), len(ds.Errors))
	}
}

// TestRead_TrimsSurroundingWhitespace pins that padding in the pipeline's
// output never becomes part of a stored name (which would defeat the exact
// crosswalk key) or of an ext_id.
func TestRead_TrimsSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	padded := filepath.Join(t.TempDir(), "padded.csv")
	if err := writeFile(padded, "taxon|rank|status|accepted_taxon|source_id\n  Abies alba  | SPE | accepted | | 2 \n"); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	ds, err := namelist.Read(padded)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	want := namelist.Row{Taxon: "Abies alba", Rank: "SPE", Status: "accepted", SourceID: "2"}
	if len(ds.Rows) != 1 || ds.Rows[0] != want {
		t.Errorf("Read: got %+v, want exactly [%+v]", ds.Rows, want)
	}
}

// TestRead_SynonymRowKeepsItsAcceptedName pins the contract's synonymy
// column, which FloraVeg never populates but GermanSL/EuroSL do — the reader
// is shared, so the column must survive.
func TestRead_SynonymRowKeepsItsAcceptedName(t *testing.T) {
	t.Parallel()

	syn := filepath.Join(t.TempDir(), "synonym.csv")
	if err := writeFile(syn, "taxon|rank|status|accepted_taxon|source_id\nFestuca duriuscula|SPE|synonym|Festuca ovina|99\n"); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	ds, err := namelist.Read(syn)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	want := namelist.Row{Taxon: "Festuca duriuscula", Rank: "SPE", Status: "synonym", AcceptedTaxon: "Festuca ovina", SourceID: "99"}
	if len(ds.Rows) != 1 || ds.Rows[0] != want {
		t.Errorf("Read: got %+v, want exactly [%+v]", ds.Rows, want)
	}
}

// TestRead_ColumnOrderIsTakenFromTheHeader pins that the decode is keyed on
// the header names, not on positions: a pipeline emitting the contract's
// columns in another order must still decode correctly rather than silently
// swapping name and id.
func TestRead_ColumnOrderIsTakenFromTheHeader(t *testing.T) {
	t.Parallel()

	reordered := filepath.Join(t.TempDir(), "reordered.csv")
	if err := writeFile(reordered, "source_id|status|taxon|accepted_taxon|rank\n2|accepted|Abies alba||SPE\n"); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	ds, err := namelist.Read(reordered)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	want := namelist.Row{Taxon: "Abies alba", Rank: "SPE", Status: "accepted", SourceID: "2"}
	if len(ds.Rows) != 1 || ds.Rows[0] != want {
		t.Errorf("Read: got %+v, want exactly [%+v]", ds.Rows, want)
	}
}

// TestRead_ParentColumnsAreOptional pins the EuroSL/GermanSL extension: a
// CSV that carries parent_id/parent_rank columns populates Row.ParentID/
// Row.ParentRank, while a CSV without them (the euromed/floraveg contract's
// 5-column shape) keeps reading successfully with both fields empty. The
// columns are deliberately NOT in wantHeader — adding them there would make
// headerIndex fail every euromed ingest, which never gained these columns.
func TestRead_ParentColumnsAreOptional(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		withParents := filepath.Join(t.TempDir(), "with-parents.csv")
		content := "taxon|rank|status|accepted_taxon|source_id|parent_id|parent_rank\n" +
			"Salsola kali|Species|accepted||id1|id0|Species Aggregate\n"
		if err := writeFile(withParents, content); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		ds, err := namelist.Read(withParents)
		if err != nil {
			t.Fatalf("Read: unexpected error: %v", err)
		}
		want := namelist.Row{
			Taxon: "Salsola kali", Rank: "Species", Status: "accepted",
			SourceID: "id1", ParentID: "id0", ParentRank: "Species Aggregate",
		}
		if len(ds.Rows) != 1 || ds.Rows[0] != want {
			t.Errorf("Read: got %+v, want exactly [%+v]", ds.Rows, want)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		noParents := filepath.Join(t.TempDir(), "no-parents.csv")
		content := "taxon|rank|status|accepted_taxon|source_id\n" +
			"Abies alba|SPE|accepted||2\n"
		if err := writeFile(noParents, content); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		ds, err := namelist.Read(noParents)
		if err != nil {
			t.Fatalf("Read: unexpected error: %v", err)
		}
		if len(ds.Rows) != 1 {
			t.Fatalf("Read: got %d rows, want 1 (%+v)", len(ds.Rows), ds.Rows)
		}
		if ds.Rows[0].ParentID != "" || ds.Rows[0].ParentRank != "" {
			t.Errorf("Read: got ParentID=%q ParentRank=%q, want both empty when the source has no parent columns", ds.Rows[0].ParentID, ds.Rows[0].ParentRank)
		}
	})
}

// TestRead_VernacularDEColumnIsOptional pins the GermanSL extension
// (analogous to TestRead_ParentColumnsAreOptional above): a CSV carrying a
// vernacular_de column populates Row.VernacularDE, while a CSV without it
// (every non-GermanSL pipeline) keeps reading successfully with the field
// empty. Deliberately not in wantHeader, for the same reason as parent_id/
// parent_rank — see headerIndex/rowFrom.
func TestRead_VernacularDEColumnIsOptional(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		withVernacular := filepath.Join(t.TempDir(), "with-vernacular.csv")
		content := "taxon|rank|status|accepted_taxon|source_id|parent_id|parent_rank|vernacular_de\n" +
			"Salsola kali|Species|accepted||id1|id0|Species Aggregate|Kali-Salzkraut\n"
		if err := writeFile(withVernacular, content); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		ds, err := namelist.Read(withVernacular)
		if err != nil {
			t.Fatalf("Read: unexpected error: %v", err)
		}
		want := namelist.Row{
			Taxon: "Salsola kali", Rank: "Species", Status: "accepted",
			SourceID: "id1", ParentID: "id0", ParentRank: "Species Aggregate",
			VernacularDE: "Kali-Salzkraut",
		}
		if len(ds.Rows) != 1 || ds.Rows[0] != want {
			t.Errorf("Read: got %+v, want exactly [%+v]", ds.Rows, want)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		noVernacular := filepath.Join(t.TempDir(), "no-vernacular.csv")
		content := "taxon|rank|status|accepted_taxon|source_id\n" +
			"Abies alba|SPE|accepted||2\n"
		if err := writeFile(noVernacular, content); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		ds, err := namelist.Read(noVernacular)
		if err != nil {
			t.Fatalf("Read: unexpected error: %v", err)
		}
		if len(ds.Rows) != 1 {
			t.Fatalf("Read: got %d rows, want 1 (%+v)", len(ds.Rows), ds.Rows)
		}
		if ds.Rows[0].VernacularDE != "" {
			t.Errorf("Read: got VernacularDE=%q, want empty when the source has no vernacular_de column", ds.Rows[0].VernacularDE)
		}
	})
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
