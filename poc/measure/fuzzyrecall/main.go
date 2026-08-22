// Command fuzzyrecall measures, against a REAL hostus index, whether a fuzzy
// prefilter strategy actually returns the name it is supposed to find.
//
// THROWAWAY measurement tool (see poc/measure/latency, poc/measure/bridge for
// the same pattern). It answers one question and is not part of the served
// binary: the production prefilter (internal/adapters/sqlite.
// fuzzyCandidateRows) orders its candidates ALPHABETICALLY before LIMIT 20,
// so on a real index the 20 slots are consumed by alphabetically-early names
// and the intended target is never scored. Pool size is NOT the metric that
// decides a fix — "is the target in the returned set" is.
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// window mirrors the production fuzzyCandidateLengthWindow.
const window = 3

// prodLimit mirrors the production fuzzyCandidateLimit.
const prodLimit = 20

type cand struct {
	nameID string
	fold   string
}

type strategy struct {
	name string
	// run returns the candidate names this strategy's prefilter would hand
	// to the scorer for query fold `want`.
	run func(ctx context.Context, db *sql.DB, want, backbone string) ([]cand, error)
}

type testCase struct {
	class    string // synthetic mutation class, or "esy"
	query    string // verbatim query (pre-canonicalization)
	wantFold string // the target's canonical_fold; "" = unknown (real ESy case)
}

type result struct {
	found    int // target in returned set
	top      int // target is the best-scoring candidate AND >= threshold
	resolved int // ANY candidate >= threshold (for unknown-target cases)
	pool     int // total candidates returned, summed
	cases    int
	durs     []time.Duration
}

func main() {
	dbPath := flag.String("db", "", "path to a real hostus index (required)")
	esyPath := flag.String("esy", "", "path to situs species_roles.csv (real control group)")
	backbone := flag.String("backbone", "wcvp", "entry_backbone filter, '' for none")
	n := flag.Int("n", 200, "synthetic base names to sample")
	seed := flag.Int64("seed", 20260822, "sampling/mutation seed")
	esyMax := flag.Int("esy-max", 0, "cap real ESy cases (0 = all unresolved ones)")
	only := flag.String("only", "", "run only strategies whose name contains this substring")
	dump := flag.String("dump", "", "strategy name substring: print its top candidate per ESy case for hand-checking")
	flag.Parse()

	if *dbPath == "" {
		failf("-db is required")
	}

	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		failf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	cases, err := synthetic(ctx, db, *backbone, *n, *seed)
	if err != nil {
		failf("building synthetic cases: %v", err)
	}
	fmt.Printf("synthetic cases: %d (from %d base names)\n", len(cases), *n)

	if *esyPath != "" {
		real, err := esyCases(ctx, db, *esyPath, *backbone, *esyMax)
		if err != nil {
			failf("building ESy cases: %v", err)
		}
		fmt.Printf("real ESy cases (no exact match in index): %d\n", len(real))
		cases = append(cases, real...)
	}

	strategies := []strategy{
		{"baseline(1-rune,±3,alpha,LIMIT20)", stratBaseline},
		{"nolimit(1-rune,±3,ALL)", stratNoLimit},
		{"prefix2(2-rune,±3,ALL)", stratPrefixN(2)},
		{"prefix3(3-rune,±3,ALL)", stratPrefixN(3)},
		{"prefix4(4-rune,±3,ALL)", stratPrefixN(4)},
		{"prefix5(5-rune,±3,ALL)", stratPrefixN(5)},
		{"ftsEpithet(fts_name MATCH epithet)", stratFTSEpithet},
		{"prefix4+ftsEpithet", stratGenusOrEpithet},
	}

	if *only != "" {
		var kept []strategy
		for _, s := range strategies {
			if strings.Contains(s.name, *only) {
				kept = append(kept, s)
			}
		}
		strategies = kept
	}

	// classes keeps report order stable and independent of map iteration.
	var classes []string
	seenClass := map[string]bool{}
	for _, c := range cases {
		if !seenClass[c.class] {
			seenClass[c.class] = true
			classes = append(classes, c.class)
		}
	}

	// results[strategy][class]
	results := map[string]map[string]*result{}
	for _, s := range strategies {
		results[s.name] = map[string]*result{}
		for _, cl := range classes {
			results[s.name][cl] = &result{}
		}
	}

	for _, tc := range cases {
		want := Canonicalize(tc.query)
		if want == "" {
			continue
		}
		for _, s := range strategies {
			start := time.Now()
			cands, err := s.run(ctx, db, want, *backbone)
			dur := time.Since(start)
			if err != nil {
				failf("strategy %s on %q: %v", s.name, tc.query, err)
			}
			r := results[s.name][tc.class]
			r.cases++
			r.pool += len(cands)
			r.durs = append(r.durs, dur)

			bestSim, bestFold := 0.0, ""
			for _, c := range cands {
				sim := Similarity(want, c.fold)
				if sim > bestSim {
					bestSim, bestFold = sim, c.fold
				}
			}
			if bestSim >= FuzzyThreshold {
				r.resolved++
			}
			if *dump != "" && tc.class == "esy-unresolved" && strings.Contains(s.name, *dump) {
				fmt.Printf("  ESY %-40s -> %-40s %.3f%s\n", want, bestFold, bestSim,
					map[bool]string{true: "", false: "  (below threshold)"}[bestSim >= FuzzyThreshold])
			}
			if tc.wantFold != "" {
				for _, c := range cands {
					if c.fold == tc.wantFold {
						r.found++
						break
					}
				}
				if bestFold == tc.wantFold && bestSim >= FuzzyThreshold {
					r.top++
				}
			}
		}
	}

	report(strategies, classes, results)
}

func report(strategies []strategy, classes []string, results map[string]map[string]*result) {
	for _, cl := range classes {
		fmt.Printf("\n=== class: %s\n", cl)
		fmt.Printf("%-42s %7s %7s %7s %9s %9s %9s\n",
			"strategy", "cases", "found", "top", "resolved", "avgPool", "p95")
		for _, s := range strategies {
			r := results[s.name][cl]
			if r.cases == 0 {
				continue
			}
			fmt.Printf("%-42s %7d %6.1f%% %6.1f%% %8.1f%% %9.0f %9s\n",
				s.name, r.cases,
				100*float64(r.found)/float64(r.cases),
				100*float64(r.top)/float64(r.cases),
				100*float64(r.resolved)/float64(r.cases),
				float64(r.pool)/float64(r.cases),
				p95(r.durs))
		}
	}
}

func p95(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(float64(len(s)) * 0.95)
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i].Round(time.Microsecond)
}

// ---- strategies -----------------------------------------------------------

// prefilterSQL is the production query shape, parameterized on the GLOB
// prefix and the LIMIT clause, so every variant below differs from the
// shipped one in exactly the dimension being measured.
func prefilter(ctx context.Context, db *sql.DB, want, prefix, backbone string, limit int) ([]cand, error) {
	q := `
		SELECT DISTINCT n.id, n.canonical_fold FROM name n
		JOIN concept_name cn ON cn.name_id = n.id
		JOIN taxon_concept tc ON tc.id = cn.concept_id
		WHERE n.canonical_fold GLOB ?
		  AND ABS(length(n.canonical_fold) - length(?)) <= ?
		  AND (? = '' OR tc.backbone_id = ?)
		ORDER BY ABS(length(n.canonical_fold) - length(?)), n.canonical_fold`
	args := []any{prefix, want, window, backbone, backbone, want}
	if limit > 0 {
		q += "\n\t\tLIMIT ?"
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanCands(rows)
}

func stratBaseline(ctx context.Context, db *sql.DB, want, backbone string) ([]cand, error) {
	return prefilter(ctx, db, want, globPrefix(want, 1), backbone, prodLimit)
}

func stratNoLimit(ctx context.Context, db *sql.DB, want, backbone string) ([]cand, error) {
	return prefilter(ctx, db, want, globPrefix(want, 1), backbone, 0)
}

// stratPrefixN narrows the GLOB prefix to n runes and drops the LIMIT
// entirely: the whole remaining pool is scored in Go. n trades recall on a
// typo INSIDE the prefix against pool size (and thus latency) — which is
// exactly the trade the typo-genus-* classes measure.
func stratPrefixN(n int) func(context.Context, *sql.DB, string, string) ([]cand, error) {
	return func(ctx context.Context, db *sql.DB, want, backbone string) ([]cand, error) {
		return prefilter(ctx, db, want, globPrefix(want, n), backbone, 0)
	}
}

// stratFTSEpithet routes through fts_name, whose unicode61 tokenizer makes
// every WORD of a canonical name an indexed token — so the epithet is
// reachable without a suffix scan. This is the only candidate route that can
// survive a genus rename, where the first rune itself changes.
func stratFTSEpithet(ctx context.Context, db *sql.DB, want, backbone string) ([]cand, error) {
	ep := epithet(want)
	if ep == "" {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT n.id, n.canonical_fold
		FROM fts_name f
		JOIN fts_name_map m ON m.rowid = f.rowid
		JOIN concept_name cn ON cn.concept_id = m.concept_id
		JOIN name n ON n.id = cn.name_id
		JOIN taxon_concept tc ON tc.id = m.concept_id
		WHERE fts_name MATCH ?
		  AND ABS(length(n.canonical_fold) - length(?)) <= ?
		  AND (? = '' OR tc.backbone_id = ?)`,
		ftsQuery(ep), want, window, backbone, backbone)
	if err != nil {
		return nil, err
	}
	return scanCands(rows)
}

func stratGenusOrEpithet(ctx context.Context, db *sql.DB, want, backbone string) ([]cand, error) {
	a, err := stratPrefixN(4)(ctx, db, want, backbone)
	if err != nil {
		return nil, err
	}
	b, err := stratFTSEpithet(ctx, db, want, backbone)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]cand, 0, len(a)+len(b))
	for _, c := range append(a, b...) {
		if !seen[c.nameID] {
			seen[c.nameID] = true
			out = append(out, c)
		}
	}
	return out, nil
}

func scanCands(rows *sql.Rows) ([]cand, error) {
	defer func() { _ = rows.Close() }()
	var out []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.nameID, &c.fold); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// globPrefix mirrors production's globEscape + "*", but takes the first n
// runes instead of always the first one.
func globPrefix(want string, n int) string {
	r := []rune(want)
	if n > len(r) {
		n = len(r)
	}
	var b strings.Builder
	for _, c := range r[:n] {
		switch c {
		case '*', '?', '[':
			b.WriteByte('[')
			b.WriteRune(c)
			b.WriteByte(']')
		default:
			b.WriteRune(c)
		}
	}
	b.WriteString("*")
	return b.String()
}

// ftsQuery wraps a bare token in double quotes so FTS5 treats it as a
// literal string, never as a query operator (AND/OR/NOT/NEAR/*/:/^).
func ftsQuery(tok string) string {
	return `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
}

func epithet(fold string) string {
	parts := strings.Fields(fold)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// ---- query sets -----------------------------------------------------------

// synthetic samples real binomial species names from the index and mutates
// them deterministically. The target is known BY CONSTRUCTION, which is what
// makes recall exactly measurable.
func synthetic(ctx context.Context, db *sql.DB, backbone string, n int, seed int64) ([]testCase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT n.canonical_fold FROM name n
		JOIN concept_name cn ON cn.name_id = n.id
		JOIN taxon_concept tc ON tc.id = cn.concept_id
		WHERE n.rank = 'SPECIES'
		  AND (? = '' OR tc.backbone_id = ?)
		  AND n.canonical_fold GLOB '* *'
		  AND n.canonical_fold NOT GLOB '* * *'
		ORDER BY n.canonical_fold
		LIMIT 400000`, backbone, backbone)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var all []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		all = append(all, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no binomial SPECIES names found (backbone %q)", backbone)
	}

	rnd := rand.New(rand.NewSource(seed))
	// Genus pool for the two rename classes, keyed by first rune so
	// "same-initial" (Astracantha -> Astragalus) and "different-initial"
	// (Coronilla -> Securigera) can be produced separately.
	genera := map[string][]string{}
	var allGenera []string
	for _, f := range all {
		g := strings.Fields(f)[0]
		r := string([]rune(g)[:1])
		if len(genera[r]) < 50 {
			genera[r] = append(genera[r], g)
		}
		if len(allGenera) < 5000 {
			allGenera = append(allGenera, g)
		}
	}

	var cases []testCase
	for i := 0; i < n; i++ {
		base := all[rnd.Intn(len(all))]
		g, ep := strings.Fields(base)[0], epithet(base)
		if len(ep) < 5 {
			continue
		}
		add := func(class, q string) {
			cases = append(cases, testCase{class: class, query: q, wantFold: base})
		}
		add("typo-transpose", g+" "+transpose(ep, len(ep)/2))
		add("typo-drop", g+" "+ep[:len(ep)/2]+ep[len(ep)/2+1:])
		add("typo-vowel", g+" "+swapVowel(ep))
		// A typo inside the genus is what a GLOB prefix cannot see. Split
		// by position: chars 1-2 defeat every prefix >= 2, chars 4-5 only
		// defeat the longer ones.
		if len([]rune(g)) >= 7 {
			add("typo-genus-pos2", transpose(g, 1)+" "+ep)
			add("typo-genus-pos5", transpose(g, 4)+" "+ep)
		}
		if g2 := pick(rnd, genera[string([]rune(g)[:1])], g); g2 != "" {
			add("genus-same-initial", g2+" "+ep)
		}
		if g3 := pickDifferentInitial(rnd, allGenera, g); g3 != "" {
			add("genus-diff-initial", g3+" "+ep)
		}
	}
	return cases, nil
}

func transpose(s string, i int) string {
	r := []rune(s)
	if i+1 >= len(r) {
		return s
	}
	r[i], r[i+1] = r[i+1], r[i]
	return string(r)
}

// swapVowel performs the single most common scientific-name misspelling:
// a<->e in the epithet (canescens -> canascens, the measured live failure).
func swapVowel(s string) string {
	r := []rune(s)
	for i := len(r) - 1; i >= 0; i-- {
		switch r[i] {
		case 'e':
			r[i] = 'a'
			return string(r)
		case 'a':
			r[i] = 'e'
			return string(r)
		}
	}
	return s
}

func pick(rnd *rand.Rand, pool []string, exclude string) string {
	for i := 0; i < 10 && len(pool) > 0; i++ {
		c := pool[rnd.Intn(len(pool))]
		if c != exclude && len([]rune(c)) >= len([]rune(exclude))-window && len([]rune(c)) <= len([]rune(exclude))+window {
			return c
		}
	}
	return ""
}

func pickDifferentInitial(rnd *rand.Rand, pool []string, exclude string) string {
	e := []rune(exclude)
	for i := 0; i < 20 && len(pool) > 0; i++ {
		c := pool[rnd.Intn(len(pool))]
		r := []rune(c)
		if r[0] != e[0] && len(r) >= len(e)-window && len(r) <= len(e)+window {
			return c
		}
	}
	return ""
}

// esyCases loads the real ESy species-role names and keeps only those with NO
// exact match in the index — i.e. exactly the rows that reach (or should
// reach) the fuzzy path in production. Their correct target is NOT known, so
// they are scored on "would this strategy resolve anything at all", and the
// top candidate is printed for hand-checking.
func esyCases(ctx context.Context, db *sql.DB, path, backbone string, max int) ([]testCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s: no data rows", path)
	}
	col := -1
	for i, h := range recs[0] {
		if h == "verbatim_name" {
			col = i
		}
	}
	if col < 0 {
		return nil, fmt.Errorf("%s: no verbatim_name column", path)
	}

	seen := map[string]bool{}
	var out []testCase
	for _, rec := range recs[1:] {
		v := rec[col]
		canon := Canonicalize(v)
		if canon == "" || seen[canon] {
			continue
		}
		seen[canon] = true
		var cnt int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM name n
			JOIN concept_name cn ON cn.name_id = n.id
			JOIN taxon_concept tc ON tc.id = cn.concept_id
			WHERE n.canonical_fold = ? AND (? = '' OR tc.backbone_id = ?)`,
			canon, backbone, backbone).Scan(&cnt); err != nil {
			return nil, err
		}
		if cnt > 0 {
			continue
		}
		out = append(out, testCase{class: "esy-unresolved", query: v})
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, nil
}

func failf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "fuzzyrecall: "+format+"\n", a...)
	os.Exit(1)
}
