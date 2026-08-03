// Package cdm reads the two canonical, pipe-delimited CSVs emitted by
// pipelines/cdm/build.sh (see pipelines/README.md and pipelines/cdm/README.md
// for the contract): the taxonomic CONCEPTS of the CDM rl_standardliste —
// each scoped by the sec. reference space it belongs to — and the typed
// CONCEPT RELATIONS between them (UC6, SP5).
//
// Like internal/adapters/traits and internal/adapters/xref it stays entirely
// string-typed: no rank, status or relation vocabulary is interpreted here.
// That is deliberate. The relation vocabulary is exactly the thing SP1 got
// wrong by assuming it, so the raw spelling is carried through untouched and
// domain.ParseRelation — a strict parser that fails loudly — is the single
// place it is interpreted.
//
// Both files are pipe-delimited with RFC-4180 quoting (Python csv.writer,
// delimiter='|', QUOTE_MINIMAL): 237 concepts of the full crawl carry a '"'
// character, e.g. `Achillea millefolium "Sammelart"`. They must therefore be
// parsed with encoding/csv and Comma='|', never with strings.Split.
package cdm

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ConceptRow is one row of cdm-concepts-canonical.csv: one taxonomic concept
// in one sec. reference space.
//
// Every field is the VERBATIM source value. In particular Status is the raw
// CDM taxonStatus and is EMPTY where the classification tree walk has not
// reached the concept — that is honest absence, not a missing value to be
// defaulted, and callers must handle it explicitly. SecUUID/SecTitle are
// likewise empty for the ~1,1% of concepts the sec. crosswalk does not map.
type ConceptRow struct {
	ConceptUUID        string
	ScientificName     string
	Authorship         string
	Rank               string
	Status             string
	SecUUID            string
	SecTitle           string
	ClassificationUUID string
	ParentUUID         string
}

// RelationRow is one row of cdm-relations-canonical.csv: one directed,
// typed assertion between two concepts.
//
// IsConceptRelation is a TRI-STATE pointer, not a bool: CDM's
// conceptRelationship flag distinguishes a genuine Berendsohn concept
// relation (true) from a misapplied-name relation (false), and the column is
// EMPTY for an edge only ever seen from its to-end, where the flag is
// genuinely unknown. Folding empty into false would silently reclassify an
// unknown as a name-usage assertion.
type RelationRow struct {
	FromUUID string
	ToUUID   string
	// RelationType is the raw CDM label ("Congruent to", "Includes",
	// "Included in or Includes or Overlaps", ...). It is NOT parsed here —
	// see the package comment.
	RelationType      string
	RelationSymbol    string
	IsConceptRelation *bool
	RelationshipUUID  string
}

// ConceptDataset / RelationDataset carry the parsed rows plus the non-fatal,
// per-row problems encountered (short row, malformed quoting, an
// unparseable is_concept_relation flag). Such rows are SKIPPED and reported
// rather than aborting the read — the same defensive posture the traits/xref
// readers take for bulk pipeline data — so one corrupt line in 51.466 never
// costs a 16–20 h crawl. Damaged records never truncate the traversal
// either: every later row is still read, however many bad ones precede it.
// The single exception is a read that consumes no input at all (see
// maxStalledReads), which is a hard error rather than a collected one.
type ConceptDataset struct {
	Rows   []ConceptRow
	Errors []error
}

// RelationDataset is the relation-side counterpart of ConceptDataset.
type RelationDataset struct {
	Rows   []RelationRow
	Errors []error
}

// maxStalledReads bounds how many consecutive reads that consumed NO INPUT
// readCSV tolerates before it gives up with a hard error. It deliberately
// does not bound reads that merely FAILED — see the loop in readCSV for why
// the distinction is the whole point.
const maxStalledReads = 20

var conceptColumns = []string{
	"concept_uuid", "scientific_name", "authorship", "rank", "status",
	"sec_uuid", "sec_title", "classification_uuid", "parent_uuid",
}

var relationColumns = []string{
	"from_uuid", "to_uuid", "relation_type", "relation_symbol",
	"is_concept_relation", "relationship_uuid",
}

// ReadConcepts parses the canonical CDM concepts CSV at path.
func ReadConcepts(path string) (*ConceptDataset, error) {
	var ds ConceptDataset
	err := readCSV(path, "concepts", conceptColumns, func(_ int, get func(string) string) {
		ds.Rows = append(ds.Rows, ConceptRow{
			ConceptUUID:        get("concept_uuid"),
			ScientificName:     get("scientific_name"),
			Authorship:         get("authorship"),
			Rank:               get("rank"),
			Status:             get("status"),
			SecUUID:            get("sec_uuid"),
			SecTitle:           get("sec_title"),
			ClassificationUUID: get("classification_uuid"),
			ParentUUID:         get("parent_uuid"),
		})
	}, func(err error) { ds.Errors = append(ds.Errors, err) })
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

// ReadRelations parses the canonical CDM relations CSV at path.
func ReadRelations(path string) (*RelationDataset, error) {
	var ds RelationDataset
	err := readCSV(path, "relations", relationColumns, func(line int, get func(string) string) {
		flag, ferr := parseConceptFlag(get("is_concept_relation"))
		if ferr != nil {
			ds.Errors = append(ds.Errors, fmt.Errorf("cdm: relations: line %d: %w", line, ferr))
			return
		}
		ds.Rows = append(ds.Rows, RelationRow{
			FromUUID:          get("from_uuid"),
			ToUUID:            get("to_uuid"),
			RelationType:      get("relation_type"),
			RelationSymbol:    get("relation_symbol"),
			IsConceptRelation: flag,
			RelationshipUUID:  get("relationship_uuid"),
		})
	}, func(err error) { ds.Errors = append(ds.Errors, err) })
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

// parseConceptFlag decodes the tri-state is_concept_relation column. An
// empty value is nil (unknown), "true"/"false" are the two known states, and
// anything else is an error naming the offending value rather than a guess —
// the same fail-loudly rule domain.ParseRelation applies to the relation
// type itself.
func parseConceptFlag(s string) (*bool, error) {
	switch s {
	case "":
		return nil, nil
	case "true":
		t := true
		return &t, nil
	case "false":
		f := false
		return &f, nil
	default:
		return nil, fmt.Errorf("unparseable is_concept_relation value %q", s)
	}
}

// readCSV is the shared pipe-CSV decode both readers use: it opens path,
// verifies every wanted column is present in the header (a missing column is
// a HARD error — the file is not the contract), then hands each data row to
// emit as a column-name lookup. Per-row problems go to collect with the
// 1-based line number, never a panic and never a silent skip.
func readCSV(path, what string, columns []string, emit func(line int, get func(string) string), collect func(error)) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("cdm: %s: open %s: %w", what, path, err)
	}
	defer func() { _ = f.Close() }()
	return decodeCSV(f, path, what, columns, emit, collect)
}

// decodeCSV is readCSV without the file handling — the seam that lets a test
// inject a reader which FAILS rather than merely containing bad bytes. That
// distinction matters: a failing Read is the one error mode that does not
// consume input, and therefore the only one maxStalledReads bounds. path is
// carried through for the error messages only.
func decodeCSV(src io.Reader, path, what string, columns []string, emit func(line int, get func(string) string), collect func(error)) error {
	r := csv.NewReader(src)
	r.Comma = '|'
	// LazyQuotes stays OFF (unlike the xref reader): the CDM CSVs are
	// written by Python's csv.writer with QUOTE_MINIMAL, so their quoting is
	// RFC-4180 correct by construction. A quoting error therefore means the
	// file is damaged, and it is far better to report that row than to
	// silently reinterpret a '"' inside a name.
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("cdm: %s: read header of %s: %w", what, path, err)
	}
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[name] = i
	}
	minFields := 0
	for _, want := range columns {
		i, ok := idx[want]
		if !ok {
			return fmt.Errorf("cdm: %s: %s: missing expected column %q in header %v", what, path, want, header)
		}
		// One past the RIGHTMOST position any wanted column occupies in the
		// ACTUAL header — not simply len(columns), since a header may carry
		// extra columns before the ones we want.
		//
		// Two mutants survive here, both provably equivalent for the headers
		// this reader accepts (the wanted columns occupy positions 0..n-1, a
		// contiguous run, because a header missing any of them is rejected
		// above):
		//   - `>` -> `>=` (CONDITIONALS_BOUNDARY): at i+1 == minFields the
		//     reassignment writes the value already there, a no-op — the same
		//     documented equivalence as traits/xref's minFieldsFor.
		//   - `i+1` -> `i-1` in the GUARD (ARITHMETIC_BASE): the assignment
		//     still writes i+1, so the guard only decides WHEN it runs. Over a
		//     contiguous 0..n-1 run it fires on every other column and the
		//     final column always satisfies it, so minFields still ends at n.
		//     (Traced for both CSVs: 9 and 6.) Mutating the ASSIGNMENT on the
		//     next line is not equivalent and is killed by the two
		//     one-field-short tests.
		if i+1 > minFields {
			minFields = i + 1
		}
	}

	// record counts DATA RECORDS, and is only the fallback line number: a
	// record's real 1-based line comes from csv.Reader.FieldPos, which stays
	// correct when a quoted field spans newlines (a counter incremented once
	// per record would drift there, and the drift would end up in the
	// short-row message pointing an operator at the wrong line).
	record := 1
	stalled := 0
	for {
		record++
		offsetBefore := r.InputOffset()
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			collect(fmt.Errorf("cdm: %s: %s:%d: %w", what, path, errorLine(err, record), err))
			// Distinguish the two error modes by whether the reader CONSUMED
			// anything, which is the property that actually matters:
			//
			//   - A csv.ParseError (bare quote, unterminated quote) advances
			//     past the offending record. The loop makes progress, reaches
			//     EOF, and reports one error per bad record — bounded by the
			//     size of the file. Every LATER record is still read. This is
			//     the "one corrupt line in 51.466" case the reader exists to
			//     tolerate, and a wall of them must NOT stop the traversal:
			//     silently discarding the rest of a 51.466-row artifact and
			//     still returning success would produce a partial backbone
			//     indistinguishable from a complete one. That is strictly
			//     worse than a failed ingest.
			//
			//   - A sticky I/O error (a bad block, a disconnected volume)
			//     returns the same error forever WITHOUT advancing. EOF never
			//     comes, so the loop must end itself, and ds.Errors would
			//     otherwise grow until the process runs out of memory.
			//
			// Only the second case is bounded, and it is a HARD error rather
			// than a collected one: the file could not be read, so no caller
			// should be able to mistake the result for a complete parse.
			if r.InputOffset() != offsetBefore {
				stalled = 0
				continue
			}
			stalled++
			if stalled >= maxStalledReads {
				return fmt.Errorf("cdm: %s: %s: aborting after %d reads that consumed no input (last error: %w)", what, path, stalled, err)
			}
			continue
		}
		stalled = 0
		line, _ := r.FieldPos(0)
		if len(row) < minFields {
			collect(fmt.Errorf("cdm: %s: %s:%d: short row: got %d fields, want at least %d", what, path, line, len(row), minFields))
			continue
		}
		emit(line, func(col string) string { return row[idx[col]] })
	}
}

// errorLine extracts the real 1-based file line from a csv parse error,
// falling back to the record ordinal when the error is not a parse error
// (an I/O error, which has no line of its own).
func errorLine(err error, fallback int) int {
	var pe *csv.ParseError
	// pe.Line > 0 vs >= 0 is a genuinely equivalent mutant: encoding/csv
	// numbers lines from 1, so a *csv.ParseError never carries Line == 0 and
	// the two comparisons cannot disagree on any value that reaches here. The
	// guard is written as > 0 anyway so a zero value from some future path
	// falls back rather than reporting "line 0".
	if errors.As(err, &pe) && pe.Line > 0 {
		return pe.Line
	}
	return fallback
}
