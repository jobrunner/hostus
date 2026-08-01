// Hardening Task 5 measurement: the marginal gain of each deterministic
// name-normalisation rule on the trait crosswalk.
//
// This is not a reimplementation of the crosswalk, it is the same decision
// procedure fed from the same data:
//
//   - the lookup keys come from NameCandidates, copied VERBATIM from
//     internal/domain/normalize.go by poc/measure/gen_normalize.sh (and
//     Canonicalize likewise by gen_canonicalize.sh);
//   - the index is the ingested database's canonical_fold -> distinct
//     concept-id count, which is exactly what sqlite.MatchExact resolves
//     (name JOIN concept_name, keyed on canonical_fold);
//   - the classification is application.resolveTraitName's: no key answers
//     -> unmatched; the first answering key resolves to one concept ->
//     matched; to several -> ambiguous, and the walk stops there.
//
// Its faithfulness is checked, not assumed: the "exact" row it prints must
// reproduce the recorded baseline from a real `hostus ingest` run
// (docs/research/reality-check.md, M2') exactly, otherwise the deltas below
// it mean nothing.
package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ruleSet is one measured configuration: which normalisation rules are
// allowed to contribute keys beyond the plain exact key.
type ruleSet struct {
	label string
	rules map[NormalizationRule]bool
}

// measuredRuleSets is the measurement plan: the baseline, each rule on its
// own (so its marginal gain over the baseline is visible), the hybrid
// sub-rules grouped, and finally everything together.
func measuredRuleSets() []ruleSet {
	only := func(label string, rs ...NormalizationRule) ruleSet {
		m := map[NormalizationRule]bool{RuleExact: true}
		for _, r := range rs {
			m[r] = true
		}
		return ruleSet{label: label, rules: m}
	}
	hybrid := []NormalizationRule{RuleHybridSpacing, RuleHybridMarkerDropped, RuleHybridMarkerAdded}
	aggregate := []NormalizationRule{RuleAggregate, RuleAggregateToNominate}
	all := append(append([]NormalizationRule{}, hybrid...), aggregate...)
	all = append(all, RuleAutonym, RuleOrthographyGenitive)
	return []ruleSet{
		only("exact (baseline)"),
		only("+ hybrid:spacing", RuleHybridSpacing),
		only("+ hybrid:marker_dropped", RuleHybridMarkerDropped),
		only("+ hybrid:marker_added", RuleHybridMarkerAdded),
		only("+ hybrid (all three)", hybrid...),
		only("+ aggregate (marked only)", RuleAggregate),
		only("+ aggregate (incl. nominate fallback)", aggregate...),
		only("+ autonym", RuleAutonym),
		only("+ orthography:genitive", RuleOrthographyGenitive),
		only("ALL RULES", all...),
	}
}

// outcome tallies one vocabulary's crosswalk under one ruleSet, at both the
// row level (what the ingest report prints) and the taxon level (what M2.2
// measures).
type outcome struct {
	rowsMatched, rowsUnmatched, rowsAmbiguous int
	taxaMatched, taxaUnmatched, taxaAmbiguous int
	byRuleTaxa                                map[NormalizationRule]int
	byRuleRows                                map[NormalizationRule]int
}

// conceptIndex is canonical_fold -> number of DISTINCT taxon concepts that
// key resolves to. Zero (absent) means MatchExact returns nothing.
type conceptIndex map[string]int

// runNorm is the --norm entry point.
func runNorm(dbPath string, vocabs pathList) error {
	idx, err := loadConceptIndex(dbPath)
	if err != nil {
		return err
	}
	fmt.Printf("index: %d distinct canonical_fold keys resolvable via name JOIN concept_name\n\n", len(idx))

	for _, v := range vocabs {
		counts, err := loadTaxonRowCounts(v.path)
		if err != nil {
			return err
		}
		totalRows := 0
		for _, n := range counts {
			totalRows += n
		}
		fmt.Printf("## %s — %d distinct taxa, %d rows\n\n", v.name, len(counts), totalRows)
		fmt.Printf("| rule set | rows matched | rows unmatched | rows ambiguous | taxa matched | taxa unmatched | taxa ambiguous | taxa hit-rate |\n")
		fmt.Printf("|---|---:|---:|---:|---:|---:|---:|---:|\n")
		var base outcome
		for i, rs := range measuredRuleSets() {
			o := measure(idx, counts, rs)
			if i == 0 {
				base = o
			}
			fmt.Printf("| %s | %d | %d | %d | %d | %d | %d | %.2f %% |\n",
				rs.label, o.rowsMatched, o.rowsUnmatched, o.rowsAmbiguous,
				o.taxaMatched, o.taxaUnmatched, o.taxaAmbiguous,
				pct(o.taxaMatched, len(counts)))
			if i > 0 {
				fmt.Printf("| … delta vs baseline | %+d | %+d | %+d | %+d | %+d | %+d | %+.2f pp |\n",
					o.rowsMatched-base.rowsMatched, o.rowsUnmatched-base.rowsUnmatched, o.rowsAmbiguous-base.rowsAmbiguous,
					o.taxaMatched-base.taxaMatched, o.taxaUnmatched-base.taxaUnmatched, o.taxaAmbiguous-base.taxaAmbiguous,
					pct(o.taxaMatched, len(counts))-pct(base.taxaMatched, len(counts)))
			}
		}
		fmt.Println()
		printPerRule(idx, counts)
		fmt.Println()
	}
	return nil
}

// printPerRule breaks the ALL-RULES run down by the rule that actually
// produced each winning key — the same breakdown
// application.TraitIngestReport.Normalized carries.
func printPerRule(idx conceptIndex, counts map[string]int) {
	all := measuredRuleSets()
	o := measure(idx, counts, all[len(all)-1])
	rules := make([]NormalizationRule, 0, len(o.byRuleTaxa))
	for r := range o.byRuleTaxa {
		rules = append(rules, r)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i] < rules[j] })
	fmt.Printf("winning rule under ALL RULES (matched only):\n")
	for _, r := range rules {
		flag := ""
		if r.Flagged() {
			flag = "  [FLAGGED: circumscriptions equated, not identical]"
		}
		fmt.Printf("  %-24s taxa=%6d rows=%7d%s\n", r, o.byRuleTaxa[r], o.byRuleRows[r], flag)
	}
}

// measure walks every distinct taxon's candidate ladder — restricted to
// rs's rules — exactly as application.resolveTraitName does.
func measure(idx conceptIndex, counts map[string]int, rs ruleSet) outcome {
	o := outcome{byRuleTaxa: map[NormalizationRule]int{}, byRuleRows: map[NormalizationRule]int{}}
	for canon, rows := range counts {
		rule, concepts := resolve(idx, canon, rs)
		switch {
		case concepts == 0:
			o.taxaUnmatched++
			o.rowsUnmatched += rows
		case concepts > 1:
			o.taxaAmbiguous++
			o.rowsAmbiguous += rows
		default:
			o.taxaMatched++
			o.rowsMatched += rows
			if rule != RuleExact {
				o.byRuleTaxa[rule]++
				o.byRuleRows[rule] += rows
			}
		}
	}
	return o
}

// resolve returns the rule of the first candidate key the index answers and
// how many distinct concepts it answered with (0 = nothing answered).
func resolve(idx conceptIndex, canon string, rs ruleSet) (NormalizationRule, int) {
	for _, cand := range NameCandidates(canon) {
		if !rs.rules[cand.Rule] {
			continue
		}
		if n := idx[cand.Key]; n > 0 {
			return cand.Rule, n
		}
	}
	return RuleExact, 0
}

// loadConceptIndex builds the canonical_fold -> distinct-concept-count map
// MatchExact effectively resolves against.
func loadConceptIndex(path string) (conceptIndex, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`SELECT n.canonical_fold, COUNT(DISTINCT cn.concept_id)
		FROM name n JOIN concept_name cn ON cn.name_id = n.id
		GROUP BY n.canonical_fold`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := conceptIndex{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// loadTaxonRowCounts reads a canonical trait CSV and returns
// Canonicalize(taxon) -> number of data rows carrying that taxon, which is
// what turns a taxon-level outcome into the row-level counts the ingest
// report prints.
func loadTaxonRowCounts(path string) (map[string]int, error) {
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
	idx := -1
	for i, h := range header {
		if strings.TrimSpace(h) == "taxon" {
			idx = i
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%s: no \"taxon\" column in %v", path, header)
	}

	out := map[string]int{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if idx >= len(row) {
			continue
		}
		k := Canonicalize(row[idx])
		if k == "" {
			continue
		}
		out[k]++
	}
	return out, nil
}
