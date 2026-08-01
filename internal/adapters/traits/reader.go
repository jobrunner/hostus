// Package traits reads the canonical, pipe-delimited trait CSV emitted by
// the per-vocabulary pipelines under pipelines/{eive,tichy,midolo}/build.sh
// (see pipelines/README.md for the contract). It stays string-typed rather
// than parsing into internal/domain's TraitVocab/TraitDim: this adapter's
// job is a thin, defensive CSV decode, not vocabulary/dimension validation.
package traits

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// Row is one row of the canonical trait CSV: one (taxon, dim) observation
// from one vocabulary.
//
// NicheWidth and NSystems are pointers on purpose: Tichý and Midolo never
// populate the niche_width/n_systems columns (empty field in the CSV), while
// EIVE always does. A nil pointer means "this vocabulary did not provide
// this datum"; a non-nil pointer to 0.0/0 means it was provided and happens
// to be zero. Never collapse nil into a zero value.
type Row struct {
	Taxon        string
	Vocab        string
	VocabVersion string
	Dim          string
	Value        float64
	NicheWidth   *float64
	NSystems     *int
}

// Dataset is the parsed canonical trait CSV. Errors collects non-fatal,
// per-row problems (short row, non-numeric value/niche_width/n_systems):
// such rows are skipped rather than causing Read to fail outright, matching
// the WCVP reader's defensive posture for bulk vendor/pipeline data.
type Dataset struct {
	Rows   []Row
	Errors []error
}

var wantHeader = []string{
	"taxon", "vocab", "vocab_version", "dim", "value", "niche_width", "n_systems",
}

// Read parses the canonical trait CSV at path.
func Read(path string) (*Dataset, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("traits: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.Comma = '|'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("traits: read header of %s: %w", path, err)
	}
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[name] = i
	}
	for _, want := range wantHeader {
		if _, ok := idx[want]; !ok {
			return nil, fmt.Errorf("traits: %s: missing expected column %q in header %v", path, want, header)
		}
	}

	var ds Dataset
	minFields := len(wantHeader)
	line := 1 // header was line 1
	for {
		line++
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ds.Errors = append(ds.Errors, fmt.Errorf("traits: %s:%d: %w", path, line, err))
			continue
		}
		if len(row) < minFields {
			ds.Errors = append(ds.Errors, fmt.Errorf("traits: %s:%d: short row: got %d fields, want at least %d", path, line, len(row), minFields))
			continue
		}
		parsed, err := parseRow(idx, row)
		if err != nil {
			ds.Errors = append(ds.Errors, fmt.Errorf("traits: %s:%d: %w", path, line, err))
			continue
		}
		ds.Rows = append(ds.Rows, parsed)
	}
	return &ds, nil
}

// parseRow decodes one data row. value is mandatory; niche_width/n_systems
// are optional (empty string -> nil pointer, never a zero value).
func parseRow(idx map[string]int, row []string) (Row, error) {
	valueStr := row[idx["value"]]
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return Row{}, fmt.Errorf("invalid value %q: %w", valueStr, err)
	}

	var nicheWidth *float64
	if s := row[idx["niche_width"]]; s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return Row{}, fmt.Errorf("invalid niche_width %q: %w", s, err)
		}
		nicheWidth = &v
	}

	var nSystems *int
	if s := row[idx["n_systems"]]; s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			return Row{}, fmt.Errorf("invalid n_systems %q: %w", s, err)
		}
		nSystems = &v
	}

	return Row{
		Taxon:        row[idx["taxon"]],
		Vocab:        row[idx["vocab"]],
		VocabVersion: row[idx["vocab_version"]],
		Dim:          row[idx["dim"]],
		Value:        value,
		NicheWidth:   nicheWidth,
		NSystems:     nSystems,
	}, nil
}
