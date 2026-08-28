// Package xref reads the canonical, pipe-delimited cross-reference CSV
// emitted by pipelines/wikidata/build.sh (see pipelines/README.md for the
// contract): one row per (bridge item x external authority), carrying the
// join key (join_authority/join_id) the ingest resolves against hostus'
// existing xref table. It stays string-typed: a thin, defensive CSV decode,
// not validation of the authorities/ids themselves.
package xref

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Row is one row of the canonical xref CSV: one external authority's id for
// the bridge item identified by (JoinAuthority, JoinID) — e.g.
// JoinAuthority="powo", JoinID="396681-1", Authority="inat",
// ExtID="160927".
type Row struct {
	JoinAuthority string
	JoinID        string
	Authority     string
	ExtID         string
	WikidataQID   string
}

// Dataset is the parsed canonical xref CSV. Errors collects non-fatal,
// per-row problems (short row): such rows are skipped rather than causing
// Read to fail outright, matching the traits/WCVP readers' defensive
// posture for bulk pipeline data.
type Dataset struct {
	Rows   []Row
	Errors []error
}

var wantHeader = []string{
	"join_authority", "join_id", "authority", "ext_id", "wikidata_qid",
}

// Read parses the canonical xref CSV at path.
func Read(path string) (*Dataset, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("xref: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.Comma = '|'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("xref: read header of %s: %w", path, err)
	}
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[name] = i
	}
	for _, want := range wantHeader {
		if _, ok := idx[want]; !ok {
			return nil, fmt.Errorf("xref: %s: missing expected column %q in header %v", path, want, header)
		}
	}

	var ds Dataset
	minFields := minFieldsFor(idx)
	line := 1 // header was line 1
	for {
		line++
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ds.Errors = append(ds.Errors, fmt.Errorf("xref: %s:%d: %w", path, line, err))
			continue
		}
		if len(row) < minFields {
			ds.Errors = append(ds.Errors, fmt.Errorf("xref: %s:%d: short row: got %d fields, want at least %d", path, line, len(row), minFields))
			continue
		}
		ds.Rows = append(ds.Rows, Row{
			JoinAuthority: row[idx["join_authority"]],
			JoinID:        row[idx["join_id"]],
			Authority:     row[idx["authority"]],
			ExtID:         row[idx["ext_id"]],
			WikidataQID:   row[idx["wikidata_qid"]],
		})
	}
	return &ds, nil
}

// minFieldsFor returns how many fields a data row must have for Read to
// index every wanted column safely: one past the RIGHTMOST position any
// wanted column occupies in the ACTUAL header. See the identical helper in
// internal/adapters/namelist/reader.go for why this is not simply
// len(wantHeader).
func minFieldsFor(idx map[string]int) int {
	minFields := 0
	for _, want := range wantHeader {
		// i+1 > minFields is a genuinely equivalent mutant at
		// CONDITIONALS_BOUNDARY (>=): when i+1 == minFields exactly,
		// reassigning minFields to it is a no-op (same value), so no test
		// can observe the difference — see the identical, more fully
		// documented equivalence in traits.minFieldsFor.
		if i := idx[want]; i+1 > minFields {
			minFields = i + 1
		}
	}
	return minFields
}
