// Command bridge answers M3 of the Reality-Check: of the trait-vocabulary
// taxon names that the full WCVP index does NOT resolve, how many appear in
// each alternative name list (Euro+Med, EuroSL, GermanSL, FloraVeg)?
//
// It is a pure name-resolution probe: it uses the SAME comparison key the
// service uses (domain.Canonicalize, verbatim-copied by
// gen_canonicalize.sh) and the SAME resolution predicate as
// sqlite.MatchExact — a name counts as resolved only if its
// canonical_fold exists in `name` AND that name is linked to a
// taxon_concept through concept_name.
package main

import (
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type namedPath struct {
	name string
	path string
}

type pathList []namedPath

func (p *pathList) String() string { return fmt.Sprint(*p) }

func (p *pathList) Set(v string) error {
	name, path, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("want name=path, got %q", v)
	}
	*p = append(*p, namedPath{name: name, path: path})
	return nil
}

func main() {
	var vocabs, lists pathList
	db := flag.String("db", "", "path to the ingested hostus SQLite database")
	norm := flag.Bool("norm", false, "Hardening Task 5 mode: measure the marginal gain of each name-normalisation rule (see norm.go) instead of the M3 bridge-list probe")
	a1diff := flag.Bool("a1diff", false, "Hardening Task 6 (A1) mode: measure how many (concept,dim) slots' winner changes under the explicit exact>unflagged>flagged precedence (see a1diff.go) vs the pre-fix first-row-wins rule")
	normbridge := flag.Bool("normbridge", false, "Hardening Task 6 mode: re-measure M6's licensing-bridge gain against the POST-normalisation unresolved set (see normbridge.go)")
	flag.Var(&vocabs, "vocab", "trait vocabulary as name=path/to/canonical.csv (repeatable)")
	flag.Var(&lists, "list", "alternative name list as name=path/to/canonical.csv (repeatable)")
	flag.Parse()

	if *norm {
		if err := runNorm(*db, vocabs); err != nil {
			fmt.Fprintln(os.Stderr, "bridge:", err)
			os.Exit(1)
		}
		return
	}

	if *a1diff {
		if err := runA1Diff(*db, vocabs); err != nil {
			fmt.Fprintln(os.Stderr, "bridge:", err)
			os.Exit(1)
		}
		return
	}

	if *normbridge {
		if err := runNormBridge(*db, vocabs, lists); err != nil {
			fmt.Fprintln(os.Stderr, "bridge:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(*db, vocabs, lists); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

func run(dbPath string, vocabs, lists pathList) error {
	wcvp, err := loadWCVPNames(dbPath)
	if err != nil {
		return err
	}
	fmt.Printf("WCVP resolvable canonical_fold keys (name JOIN concept_name): %d\n\n", len(wcvp))

	listSets := make(map[string]map[string]bool, len(lists))
	listAcc := make(map[string]map[string]string, len(lists))
	for _, l := range lists {
		s, err := loadColumn(l.path, "taxon")
		if err != nil {
			return err
		}
		a, err := loadAcceptedLinks(l.path)
		if err != nil {
			return err
		}
		listSets[l.name] = s
		listAcc[l.name] = a
		fmt.Printf("name list %-10s distinct canonicalized taxa: %d, davon mit aufgeloestem accepted_taxon: %d\n", l.name, len(s), len(a))
	}
	fmt.Println()

	unionAll := map[string]bool{}
	for _, v := range vocabs {
		taxa, err := loadColumn(v.path, "taxon")
		if err != nil {
			return err
		}
		unresolved := map[string]bool{}
		for t := range taxa {
			if !wcvp[t] {
				unresolved[t] = true
			}
		}
		fmt.Printf("## %s\n", v.name)
		fmt.Printf("distinct taxa: %d\n", len(taxa))
		fmt.Printf("in WCVP:       %d (%.2f%%)\n", len(taxa)-len(unresolved), pct(len(taxa)-len(unresolved), len(taxa)))
		fmt.Printf("NOT in WCVP:   %d (%.2f%%)\n", len(unresolved), pct(len(unresolved), len(taxa)))

		union := map[string]bool{}
		for _, l := range lists {
			gain := intersect(unresolved, listSets[l.name])
			for k := range gain {
				union[k] = true
				unionAll[k] = true
			}
			// bridged: the recovered name is a SYNONYM in the list and
			// the accepted name it points at DOES resolve in WCVP — i.e.
			// the bridge would actually land the trait value on a WCVP
			// concept, not merely confirm the name exists somewhere.
			bridged := 0
			for k := range gain {
				if acc, ok := listAcc[l.name][k]; ok && wcvp[acc] {
					bridged++
				}
			}
			fmt.Printf("  + %-10s recovers %6d of the %d unresolved (%.2f%% of all %s taxa); davon via accepted_taxon nach WCVP brueckbar: %d\n",
				l.name, len(gain), len(unresolved), pct(len(gain), len(taxa)), v.name, bridged)
		}
		// Exclusive contribution: names only this one source recovers.
		for _, l := range lists {
			excl := 0
			for k := range intersect(unresolved, listSets[l.name]) {
				only := true
				for _, other := range lists {
					if other.name != l.name && listSets[other.name][k] {
						only = false
						break
					}
				}
				if only {
					excl++
				}
			}
			fmt.Printf("  ! %-10s exclusive (no other list has it): %d\n", l.name, excl)
		}
		fmt.Printf("  = union of all lists recovers %d of %d unresolved (%.2f%% of all %s taxa); still unrecovered: %d\n\n",
			len(union), len(unresolved), pct(len(union), len(taxa)), v.name, len(unresolved)-len(union))

		if err := dumpSample(v.name, unresolved); err != nil {
			return err
		}
	}
	return nil
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func intersect(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

// dumpSample writes every unresolved name of one vocabulary to
// out/unmatched-<vocab>.txt (sorted) so the manual categorisation of M2 can
// draw a reproducible sample from it.
func dumpSample(vocab string, unresolved map[string]bool) error {
	all := make([]string, 0, len(unresolved))
	for k := range unresolved {
		all = append(all, k)
	}
	sort.Strings(all)
	return os.WriteFile("poc/measure/out/unmatched-"+vocab+".txt", []byte(strings.Join(all, "\n")+"\n"), 0o600)
}

// loadWCVPNames builds the set of canonical_fold keys that MatchExact can
// actually resolve: a name row that is linked to a taxon_concept.
func loadWCVPNames(path string) (map[string]bool, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`SELECT DISTINCT n.canonical_fold FROM name n JOIN concept_name cn ON cn.name_id = n.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = true
	}
	return out, rows.Err()
}

// loadAcceptedLinks reads a canonical name-list CSV and returns
// canon(taxon) -> canon(accepted_taxon) for every row that HAS a non-empty
// accepted_taxon, i.e. every synonym whose accepted concept the list
// actually names. Lists without that column (or with it always empty —
// FloraVeg, Euro+Med) yield an empty map, which is itself the M6 finding.
func loadAcceptedLinks(path string) (map[string]string, error) {
	rows, err := loadPairs(path, "taxon", "accepted_taxon")
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// loadColumn reads a pipe-delimited canonical CSV and returns the set of
// Canonicalize'd values of the named column.
func loadColumn(path, col string) (map[string]bool, error) {
	pairs, err := loadPairsRaw(path, col, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(pairs))
	for k := range pairs {
		out[k] = true
	}
	return out, nil
}

// loadPairs is loadPairsRaw restricted to rows whose second column is
// non-empty.
func loadPairs(path, keyCol, valCol string) (map[string]string, error) {
	all, err := loadPairsRaw(path, keyCol, valCol)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(all))
	for k, v := range all {
		if v != "" {
			out[k] = v
		}
	}
	return out, nil
}

// loadPairsRaw reads a pipe-delimited canonical CSV and returns
// Canonicalize(keyCol) -> Canonicalize(valCol) for every data row (valCol
// may be "" to read only the key column).
func loadPairsRaw(path, col, valCol string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // measurement probe, paths come from flags
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
	idx, vidx := -1, -1
	for i, h := range header {
		if h == col {
			idx = i
		}
		if valCol != "" && h == valCol {
			vidx = i
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%s: no %q column in %v", path, col, header)
	}
	if valCol != "" && vidx < 0 {
		return nil, fmt.Errorf("%s: no %q column in %v", path, valCol, header)
	}

	out := map[string]string{}
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
		v := ""
		if vidx >= 0 && vidx < len(row) {
			v = Canonicalize(row[vidx])
		}
		if _, seen := out[k]; !seen || v != "" {
			out[k] = v
		}
	}
	return out, nil
}
