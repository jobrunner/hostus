// Hardening Task 6 measurement (A1): does making selectTraitWinners'
// normalised-vs-normalised precedence explicit (exact > unflagged >
// flagged, instead of "first CSV row wins") change any STORED trait_value
// row against the real WCVP database?
//
// This reuses the same verbatim-copied NameCandidates ladder and the same
// canonical_fold index as norm.go, but resolves each row to an actual
// concept id (not just a match count) so it can group rows by the same
// (concept, dim) SLOT selectTraitWinners contends over — norm.go measures
// per-DISTINCT-TAXON outcomes and cannot see two different taxon names
// colliding on the same concept, which is exactly the case A1 is about.
package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

// slotKey is one (concept, dim) contention point within a single
// vocabulary — mirrors application.traitSlot.
type slotKey struct {
	concept string
	dim     string
}

// a1Row is one trait CSV row resolved to a concept, kept only far enough to
// replay both the OLD ("first row wins among normalised ties") and NEW
// (ruleRank) winner-selection rules.
type a1Row struct {
	rowIdx  int
	concept string
	dim     string
	rule    NormalizationRule
}

// ruleRank mirrors internal/application/traits_ingest.go's ruleRank table
// exactly (see that file's doc comment) — kept in sync by hand, the same
// way this whole directory keeps NameCandidates in sync via
// gen_normalize.sh.
var a1RuleRank = map[NormalizationRule]int{
	RuleExact:               0,
	RuleHybridSpacing:       1,
	RuleHybridMarkerDropped: 2,
	RuleHybridMarkerAdded:   3,
	RuleOrthographyGenitive: 4,
	RuleAggregate:           5,
	RuleAggregateToNominate: 6,
	RuleAutonym:             7,
}

// runA1Diff is the --a1diff entry point.
func runA1Diff(dbPath string, vocabs pathList) error {
	idxCount, idxConcept, err := loadConceptIDIndex(dbPath)
	if err != nil {
		return err
	}
	fmt.Printf("index: %d distinct canonical_fold keys (%d unambiguous, single-concept)\n\n", len(idxCount), len(idxConcept))

	for _, v := range vocabs {
		rows, err := loadTraitRows(v.path)
		if err != nil {
			return err
		}
		resolved := make([]a1Row, 0, len(rows))
		for i, r := range rows {
			rule, concept, ok := resolveToConcept(idxCount, idxConcept, r.taxon)
			if !ok {
				continue // unmatched or ambiguous: never reaches selectTraitWinners
			}
			resolved = append(resolved, a1Row{rowIdx: i, concept: concept, dim: r.dim, rule: rule})
		}

		bySlot := map[slotKey][]a1Row{}
		for _, r := range resolved {
			k := slotKey{concept: r.concept, dim: r.dim}
			bySlot[k] = append(bySlot[k], r)
		}

		changed := 0
		changedFlaggedToUnflagged := 0
		contested := 0
		for _, contenders := range bySlot {
			if len(contenders) < 2 {
				continue
			}
			contested++
			oldWinner := oldSelect(contenders)
			newWinner := newSelect(contenders)
			if oldWinner.rule != newWinner.rule || oldWinner.rowIdx != newWinner.rowIdx {
				changed++
				if oldWinner.rule.Flagged() && !newWinner.rule.Flagged() {
					changedFlaggedToUnflagged++
				}
			}
		}
		fmt.Printf("## %s\n", v.name)
		fmt.Printf("resolved rows: %d, contested (concept,dim) slots (>=2 contenders): %d\n", len(resolved), contested)
		fmt.Printf("slots whose winner CHANGES under the A1 fix: %d\n", changed)
		fmt.Printf("  of which flagged->unflagged (the defect A1 fixes): %d\n", changedFlaggedToUnflagged)
		fmt.Println()
	}
	return nil
}

// oldSelect replays the PRE-A1-fix rule: an exact match beats any
// normalised one; among ties (or among normalised rows when no exact row
// contends), the first row in CSV order wins.
func oldSelect(rows []a1Row) a1Row {
	best := rows[0]
	for _, r := range rows[1:] {
		if r.rule == RuleExact && best.rule != RuleExact {
			best = r
		}
	}
	return best
}

// newSelect replays the POST-A1-fix rule: lowest a1RuleRank wins; a tie
// (identical rule) keeps the first row in CSV order.
func newSelect(rows []a1Row) a1Row {
	best := rows[0]
	for _, r := range rows[1:] {
		if a1RuleRank[r.rule] < a1RuleRank[best.rule] {
			best = r
		}
	}
	return best
}

// resolveToConcept walks NameCandidates exactly as resolveTraitName does,
// stopping at the first key idxCount answers. It reports ok=false for
// unmatched (no key answers) OR ambiguous (the answering key resolves to
// more than one concept) — both never reach selectTraitWinners.
func resolveToConcept(idxCount conceptIndex, idxConcept map[string]string, verbatim string) (NormalizationRule, string, bool) {
	for _, cand := range NameCandidates(verbatim) {
		n := idxCount[cand.Key]
		if n == 0 {
			continue
		}
		if n > 1 {
			return "", "", false // ambiguous — the walk stops here, same as resolveTraitName
		}
		return cand.Rule, idxConcept[cand.Key], true
	}
	return "", "", false
}

// loadConceptIDIndex builds both the canonical_fold -> distinct-concept
// COUNT map (loadConceptIndex's job) and a canonical_fold -> single
// concept_id map for the keys that resolve unambiguously — the latter is
// what lets this probe group different taxon names onto the SAME concept,
// which loadConceptIndex's count-only shape cannot do.
func loadConceptIDIndex(path string) (conceptIndex, map[string]string, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`SELECT n.canonical_fold, cn.concept_id
		FROM name n JOIN concept_name cn ON cn.name_id = n.id`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := conceptIndex{}
	concepts := map[string]map[string]bool{}
	for rows.Next() {
		var k, c string
		if err := rows.Scan(&k, &c); err != nil {
			return nil, nil, err
		}
		if concepts[k] == nil {
			concepts[k] = map[string]bool{}
		}
		concepts[k][c] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	single := map[string]string{}
	for k, cs := range concepts {
		counts[k] = len(cs)
		if len(cs) == 1 {
			for c := range cs {
				single[k] = c
			}
		}
	}
	return counts, single, nil
}

// traitCSVRow is one (taxon, dim) data row, in file order — order matters
// here because both oldSelect and newSelect break ties by it.
type traitCSVRow struct {
	taxon string
	dim   string
}

// loadTraitRows reads a canonical trait CSV's taxon+dim columns, preserving
// row order (unlike loadTaxonRowCounts in norm.go, which collapses to
// per-taxon counts and cannot see per-row (concept,dim) contention).
func loadTraitRows(path string) ([]traitCSVRow, error) {
	f, err := os.Open(path) //nolint:gosec // measurement probe, path comes from a flag
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.Comma = '|'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	taxonIdx, dimIdx := -1, -1
	for i, h := range header {
		switch strings.TrimSpace(h) {
		case "taxon":
			taxonIdx = i
		case "dim":
			dimIdx = i
		}
	}
	if taxonIdx < 0 || dimIdx < 0 {
		return nil, fmt.Errorf("%s: missing \"taxon\" or \"dim\" column in %v", path, header)
	}

	var out []traitCSVRow
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if taxonIdx >= len(row) || dimIdx >= len(row) {
			continue
		}
		out = append(out, traitCSVRow{taxon: row[taxonIdx], dim: row[dimIdx]})
	}
	return out, nil
}
