// Package namelist reads the canonical, pipe-delimited NAME-LIST CSV emitted
// by the checklist pipelines under pipelines/{floraveg,germansl,eurosl,
// euromed}/build.sh (see pipelines/README.md, "Canonical CSV contract (name
// lists)").
//
// The package is named for the CONTRACT, not for FloraVeg: all four
// pipelines emit the identical header, and a per-source reader would be four
// copies of one decode. Only floraveg is pinned in the manifest today.
//
// Like the traits/xref readers this stays string-typed — a thin, defensive
// CSV decode, not vocabulary or rank validation. In particular `rank` is
// carried through in the SOURCE's own vocabulary (FloraVeg's Life_form table
// provides none at all, so it is empty for every FloraVeg row) and is
// deliberately not mapped onto domain.Rank here.
package namelist

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Row is one row of the canonical name-list CSV: one name the source uses,
// with the source's own stable id.
//
// Rank/Status/AcceptedTaxon are kept verbatim. AcceptedTaxon is a NAME
// string, not an id — the contract has no id-based synonymy link — and is
// empty for accepted rows.
type Row struct {
	Taxon         string
	Rank          string
	Status        string
	AcceptedTaxon string
	SourceID      string
	// ParentID/ParentRank are the source's own parent-taxon link
	// (EuroSL/GermanSL: IsChildTaxonOfID / the parent row's TaxonRank).
	// OPTIONAL: not every pipeline emits them (euromed's 5-column CSV
	// never does), so they are deliberately excluded from wantHeader —
	// see headerIndex/rowFrom. Absent means both fields stay "".
	ParentID   string
	ParentRank string
	// VernacularDE is the German common name, verbatim from the source's
	// own vernacular column. OPTIONAL: only GermanSL emits it (its
	// VernacularName column); every other pipeline's rows leave this "".
	// Deliberately excluded from wantHeader for the same reason as
	// ParentID/ParentRank above — see headerIndex/rowFrom.
	VernacularDE string
}

// Dataset is the parsed canonical name-list CSV. Errors collects non-fatal,
// per-row problems (short row, empty taxon, empty source_id): such rows are
// SKIPPED but never silently — the count is surfaced on the ingest report,
// matching the traits/wcvp readers' defensive posture for bulk pipeline data.
type Dataset struct {
	Rows   []Row
	Errors []error
}

var wantHeader = []string{"taxon", "rank", "status", "accepted_taxon", "source_id"}

// Read parses the canonical name-list CSV at path.
func Read(path string) (*Dataset, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("namelist: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.Comma = '|'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("namelist: read header of %s: %w", path, err)
	}
	idx, err := headerIndex(path, header)
	if err != nil {
		return nil, err
	}

	var ds Dataset
	minFields := minFieldsFor(idx)
	line := 1 // header was line 1
	for {
		line++
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("namelist: %s line %d: %w", path, line, err)
		}
		row, rerr := rowFrom(rec, idx, minFields)
		if rerr != nil {
			ds.Errors = append(ds.Errors, fmt.Errorf("namelist: %s line %d: %w", path, line, rerr))
			continue
		}
		ds.Rows = append(ds.Rows, row)
	}
	return &ds, nil
}

// headerIndex maps each expected column name to its position, failing if any
// is absent. A missing column is fatal (unlike a bad row): every subsequent
// row would produce the same error, so reporting it once, up front, is both
// cheaper and clearer.
func headerIndex(path string, header []string) (map[string]int, error) {
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[name] = i
	}
	for _, want := range wantHeader {
		if _, ok := idx[want]; !ok {
			return nil, fmt.Errorf("namelist: %s: missing expected column %q in header %v", path, want, header)
		}
	}
	return idx, nil
}

// minFieldsFor returns the number of fields a record must have for every
// expected column's index to be in range. It is the MAX over the expected
// columns' positions, not len(wantHeader): a header may legally carry extra
// columns or list the contract's columns in another order, and a record
// short of the highest expected index would otherwise index out of range.
//
// i+1 >= maximum is a genuinely equivalent mutant at CONDITIONALS_BOUNDARY:
// headerIndex maps each expected column to a DISTINCT position, so the i+1
// values are pairwise distinct and i+1 can never equal the running maximum
// — the boundary the mutant moves is unreachable. Same provable-equivalence
// class as the documented boundaries in internal/application.
func minFieldsFor(idx map[string]int) int {
	maximum := 0
	for _, want := range wantHeader {
		if i := idx[want]; i+1 > maximum {
			maximum = i + 1
		}
	}
	return maximum
}

// rowFrom decodes one record. The two rejections are the two that would
// otherwise write unusable data: a row with no taxon has nothing to
// crosswalk, and a row with no source_id has no stable identity to key the
// resulting entry on (name_space_entry's primary key is (space, ext_id), so
// two id-less rows would silently overwrite one another).
func rowFrom(rec []string, idx map[string]int, minFields int) (Row, error) {
	if len(rec) < minFields {
		return Row{}, fmt.Errorf("short row: %d fields, want at least %d", len(rec), minFields)
	}
	row := Row{
		Taxon:         strings.TrimSpace(rec[idx["taxon"]]),
		Rank:          strings.TrimSpace(rec[idx["rank"]]),
		Status:        strings.TrimSpace(rec[idx["status"]]),
		AcceptedTaxon: strings.TrimSpace(rec[idx["accepted_taxon"]]),
		SourceID:      strings.TrimSpace(rec[idx["source_id"]]),
	}
	if row.Taxon == "" {
		return Row{}, errors.New("empty taxon")
	}
	if row.SourceID == "" {
		return Row{}, fmt.Errorf("taxon %q: empty source_id", row.Taxon)
	}
	if i, ok := idx["parent_id"]; ok && i < len(rec) {
		row.ParentID = strings.TrimSpace(rec[i])
	}
	if i, ok := idx["parent_rank"]; ok && i < len(rec) {
		row.ParentRank = strings.TrimSpace(rec[i])
	}
	if i, ok := idx["vernacular_de"]; ok && i < len(rec) {
		row.VernacularDE = strings.TrimSpace(rec[i])
	}
	return row, nil
}
