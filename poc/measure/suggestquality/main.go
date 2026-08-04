// Command suggestquality answers SP7 Task 1: it measures the three ruled-in
// levers for GET /v1/suggest (baseline, area restriction, candidate cap)
// directly against the SQL the production Suggest path runs
// (internal/adapters/sqlite/suggest.go), plus the composition of the result
// list that the flooding complaint is about.
//
// It goes to SQL rather than over HTTP because neither of the two levers
// under test is expressible through the serving API today: `area` resolves
// to at most one WGSRPD-L3 code, and there is no cap parameter at all.
// The HTTP-level band for the baseline is measured by the sibling
// poc/measure/latency harness.
//
// The database is opened strictly read-only (mode=ro) — the production
// sqlite.Open would apply migrations to it.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// prefixes mirrors poc/measure/latency's set: the known-bad 2-char cases
// plus realistic 3-5-char ones.
var prefixes = []string{
	"ac", "ag", "al", "be", "ca", "ce", "fe", "ga", "po", "qu", "ra", "sa", "th", "tr", "ve",
	"ace", "ach", "arte", "betu", "cala", "care", "cent", "fest", "gali", "hier",
	"potent", "querc", "ranu", "salix", "thymu", "trifo", "veron", "viola",
	"abies", "acer", "picea", "pinus", "rubus",
}

// centralEurope is the WGSRPD-L3 code set poc/measure/run.sh's m5 step
// already uses for "DE/AT/CH and neighbours" (GER/AUT/SWI plus the
// bordering countries). Reused verbatim, not re-derived.
const centralEurope = "GER,AUT,SWI,CZE,POL,HUN,FRA,ITA,NET,BGM,DEN"

// rankOrder mirrors internal/domain.rankOrder. poc is a separate Go module
// and cannot import internal/, so the comparator is duplicated here.
var rankOrder = map[string]int{
	"FAMILY": 0, "GENUS": 1, "SPECIES": 2, "SUBSPECIES": 3,
	"NOTHOSUBSPECIES": 4, "VARIETY": 5, "SUBVARIETY": 6,
	"NOTHOVARIETY": 7, "FORM": 8, "SUBFORM": 9, "NOTHOFORM": 10,
}

const unknownRankOrder = 11

type item struct {
	id        string
	canonical string
	rank      string
	status    string
	score     float64
	inArea    bool
}

func main() {
	db := flag.String("db", "/tmp/full-real.sqlite", "path to the index; opened read-only")
	runs := flag.Int("runs", 5, "number of repeated runs; results are reported as a band over runs")
	reps := flag.Int("reps", 3, "measured repetitions per prefix within one run")
	warmup := flag.Int("warmup", 1, "unmeasured warmup repetitions per prefix per run")
	limit := flag.Int("limit", 10, "caller-visible limit; the SQL fetch budget is max(4*limit, 20)")
	capN := flag.Int("cap", 2000, "candidate cap for the cap scenario")
	areas := flag.String("areas", centralEurope, "comma-separated WGSRPD-L3 codes for the area scenario")
	only := flag.String("only", "all", "all|latency|composition")
	flag.Parse()

	if err := run(*db, *runs, *reps, *warmup, *limit, *capN, *areas, *only); err != nil {
		fmt.Fprintln(os.Stderr, "suggestquality:", err)
		os.Exit(1)
	}
}

func run(dbPath string, runs, reps, warmup, limit, capN int, areas, only string) error {
	dsn := "file:" + dbPath + "?mode=ro&_pragma=query_only(1)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	conn.SetMaxOpenConns(1)

	codes := splitCodes(areas)
	ctx := context.Background()

	fmt.Printf("db=%s runs=%d reps/prefix=%d warmup=%d limit=%d fetch-budget=%d cap=%d\n",
		dbPath, runs, reps, warmup, limit, fetchBudget(limit), capN)
	fmt.Printf("areas=%s (%d codes)\n", strings.Join(codes, ","), len(codes))
	fmt.Printf("prefixes=%d\n\n", len(prefixes))

	if only == "all" || only == "latency" {
		scenarios := []struct {
			name  string
			query func(string) (string, []any)
		}{
			{"S1 baseline (no area, no cap)", func(p string) (string, []any) {
				return suggestSQL(false, 0, limit), []any{ftsToken(p), fetchBudget(limit)}
			}},
			{"S2 area-scoped (Mitteleuropa, no cap)", func(p string) (string, []any) {
				j, _ := json.Marshal(codes)
				return suggestSQL(true, 0, limit), []any{ftsToken(p), string(j), string(j), fetchBudget(limit)}
			}},
			{fmt.Sprintf("S3 cap %d (no area)", capN), func(p string) (string, []any) {
				return suggestSQL(false, capN, limit), []any{ftsToken(p), capN, fetchBudget(limit)}
			}},
			{fmt.Sprintf("S4 area-scoped + cap %d", capN), func(p string) (string, []any) {
				j, _ := json.Marshal(codes)
				return suggestSQL(true, capN, limit), []any{ftsToken(p), capN, string(j), string(j), fetchBudget(limit)}
			}},
		}
		for _, s := range scenarios {
			if err := measure(ctx, conn, s.name, s.query, runs, reps, warmup); err != nil {
				return err
			}
		}
	}

	if only == "all" || only == "composition" {
		if err := composition(ctx, conn, codes, capN, limit); err != nil {
			return err
		}
	}
	return nil
}

func splitCodes(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.ToUpper(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func fetchBudget(limit int) int {
	if limit <= 0 {
		limit = 20
	}
	if n := limit * 4; n > 20 {
		return n
	}
	return 20
}

func ftsToken(q string) string {
	return `"` + strings.ReplaceAll(q, `"`, `""`) + `"*`
}

// suggestSQL reproduces internal/adapters/sqlite/suggest.go's query. With
// area it additionally *restricts* to the area (production only orders by
// in_area); with cap > 0 the FTS candidate set is truncated by bm25 before
// the joins.
func suggestSQL(area bool, capN, limit int) string {
	matches := `SELECT rowid, bm25(fts_name) AS score FROM fts_name WHERE fts_name MATCH ?`
	if capN > 0 {
		matches += ` ORDER BY bm25(fts_name) LIMIT ?`
	}
	inArea := "0"
	restrict := ""
	if area {
		inArea = `EXISTS (SELECT 1 FROM distribution d WHERE d.concept_id = tc.id
			AND d.area_scheme = 'wgsrpd_l3' AND d.area_code IN (SELECT value FROM json_each(?)))`
		restrict = ` AND EXISTS (SELECT 1 FROM distribution d2 WHERE d2.concept_id = tc.id
			AND d2.area_scheme = 'wgsrpd_l3' AND d2.area_code IN (SELECT value FROM json_each(?)))`
	}
	return `WITH matches AS MATERIALIZED (` + matches + `)
		SELECT tc.id, an.canonical, an.rank, tc.status, MIN(m.score) AS score, ` + inArea + ` AS in_area
		FROM matches m
		JOIN fts_name_map fnm ON fnm.rowid = m.rowid
		JOIN taxon_concept tc ON tc.id = fnm.concept_id
		JOIN name an ON an.id = tc.accepted_name
		WHERE 1 = 1` + restrict + `
		GROUP BY tc.id
		ORDER BY in_area DESC, score ASC
		LIMIT ?`
}

func measure(ctx context.Context, conn *sql.DB, name string, build func(string) (string, []any), runs, reps, warmup int) error {
	perRunP50 := make([]time.Duration, 0, runs)
	perRunP95 := make([]time.Duration, 0, runs)
	perPrefix := map[string][]time.Duration{}

	for r := 0; r < runs; r++ {
		var all []time.Duration
		for _, p := range prefixes {
			q, args := build(p)
			for i := 0; i < warmup; i++ {
				if _, err := exec(ctx, conn, q, args); err != nil {
					return fmt.Errorf("%s %q: %w", name, p, err)
				}
			}
			for i := 0; i < reps; i++ {
				d, err := exec(ctx, conn, q, args)
				if err != nil {
					return fmt.Errorf("%s %q: %w", name, p, err)
				}
				all = append(all, d)
				perPrefix[p] = append(perPrefix[p], d)
			}
		}
		perRunP50 = append(perRunP50, quantile(all, 0.50))
		perRunP95 = append(perRunP95, quantile(all, 0.95))
	}

	fmt.Printf("## %s\n", name)
	fmt.Printf("p50 per run: %s\n", fmtDurs(perRunP50))
	fmt.Printf("p95 per run: %s\n", fmtDurs(perRunP95))
	fmt.Printf("p50 band: %s .. %s (median %s, CV %.1f%%)\n",
		minD(perRunP50), maxD(perRunP50), quantile(perRunP50, 0.5), cv(perRunP50))
	fmt.Printf("p95 band: %s .. %s (median %s, CV %.1f%%)\n",
		minD(perRunP95), maxD(perRunP95), quantile(perRunP95, 0.5), cv(perRunP95))
	fmt.Println()
	fmt.Println("| Prefix | p50 | p95 | max |")
	fmt.Println("|---|---:|---:|---:|")
	for _, p := range sortedPrefixes(perPrefix) {
		fmt.Printf("| `%s` | %s | %s | %s |\n", p,
			quantile(perPrefix[p], 0.50), quantile(perPrefix[p], 0.95), quantile(perPrefix[p], 1))
	}
	fmt.Println()
	return nil
}

func exec(ctx context.Context, conn *sql.DB, q string, args []any) (time.Duration, error) {
	start := time.Now()
	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n := 0
	for rows.Next() {
		n++
	}
	err = rows.Err()
	_ = rows.Close()
	return time.Since(start), err
}

// composition answers the non-latency questions: candidate volume global
// vs in-area, how often a cap truncates, how many genera a pre-diversity
// cap would drop, and the rank histogram of today's top 10.
func composition(ctx context.Context, conn *sql.DB, codes []string, capN, limit int) error {
	codesJSON, err := json.Marshal(codes)
	if err != nil {
		return err
	}

	fmt.Printf("## Composition (cap=%d, limit=%d, fetch budget=%d)\n\n", capN, limit, fetchBudget(limit))
	fmt.Println("| Prefix | FTS-Namenszeilen | Kandidaten global | davon Gattungen | Kandidaten in-area | davon Gattungen | Cap greift | Kandidaten nach Cap | Gattungen nach Cap | Gattungen verloren |")
	fmt.Println("|---|---:|---:|---:|---:|---:|:--:|---:|---:|---:|")

	histFor := map[string][]item{}
	for _, p := range prefixes {
		ftsRows, err := ftsRowCount(ctx, conn, p)
		if err != nil {
			return err
		}
		global, err := candidates(ctx, conn, p, nil, 0)
		if err != nil {
			return err
		}
		inArea, err := candidates(ctx, conn, p, codesJSON, 0)
		if err != nil {
			return err
		}
		capped, err := candidates(ctx, conn, p, nil, capN)
		if err != nil {
			return err
		}
		gGlobal := countRank(global, "GENUS")
		gArea := countRank(inArea, "GENUS")
		gCapped := countRank(capped, "GENUS")
		fmt.Printf("| `%s` | %d | %d | %d | %d | %d | %s | %d | %d | %d |\n",
			p, ftsRows, len(global), gGlobal, len(inArea), gArea,
			yesNo(ftsRows > capN), len(capped), gCapped, gGlobal-gCapped)

		if p == "ac" || p == "ca" || p == "al" {
			histFor[p] = global
		}
	}
	fmt.Println()

	fmt.Println("## Rang-Histogramm der heutigen Top 10 (ohne area)")
	fmt.Println()
	for _, p := range []string{"ac", "ca", "al"} {
		got := histFor[p]
		budget := fetchBudget(limit)
		if len(got) > budget {
			got = got[:budget]
		}
		top := rankSuggestions(got)
		if len(top) > limit {
			top = top[:limit]
		}
		hist := map[string]int{}
		var order []string
		for _, it := range top {
			if hist[it.rank] == 0 {
				order = append(order, it.rank)
			}
			hist[it.rank]++
		}
		sort.Slice(order, func(i, j int) bool { return ord(order[i]) < ord(order[j]) })
		parts := make([]string, 0, len(order))
		for _, r := range order {
			parts = append(parts, fmt.Sprintf("%s=%d", r, hist[r]))
		}
		fmt.Printf("`%s`: %s\n", p, strings.Join(parts, " "))
		for i, it := range top {
			fmt.Printf("  %2d. %-40s %-12s %s\n", i+1, it.canonical, it.rank, it.status)
		}
		fmt.Println()
	}

	// What a per-rank quota would actually surface: the best-scoring
	// genera, and how deep in the score order the first one sits.
	fmt.Println("## Was eine Gattungs-Quote liefern würde (beste GENUS-Kandidaten nach bm25)")
	fmt.Println()
	for _, p := range []string{"ac", "ca", "al"} {
		for _, scope := range []string{"global", "in-area"} {
			var list []item
			var err error
			if scope == "global" {
				list = histFor[p]
			} else {
				list, err = candidates(ctx, conn, p, codesJSON, 0)
				if err != nil {
					return err
				}
			}
			firstAt, n := -1, 0
			var names []string
			for i, it := range list {
				if it.rank != "GENUS" {
					continue
				}
				if firstAt < 0 {
					firstAt = i + 1
				}
				if n < 5 {
					names = append(names, it.canonical)
					n++
				}
			}
			fmt.Printf("`%s` %-8s erste Gattung an Position %d von %d; Top-5: %s\n",
				p, scope, firstAt, len(list), strings.Join(names, ", "))
		}
	}
	fmt.Println()
	return nil
}

// candidates returns the full, ungapped candidate list in the production
// order (in_area DESC, bm25 ASC). codesJSON nil means: no area restriction.
func candidates(ctx context.Context, conn *sql.DB, prefix string, codesJSON []byte, capN int) ([]item, error) {
	q := suggestSQL(codesJSON != nil, capN, 0)
	q = strings.Replace(q, "\n\t\tLIMIT ?", "", 1)
	args := []any{ftsToken(prefix)}
	if capN > 0 {
		args = append(args, capN)
	}
	if codesJSON != nil {
		args = append(args, string(codesJSON), string(codesJSON))
	}
	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("candidates %q: %w", prefix, err)
	}
	defer func() { _ = rows.Close() }()
	var out []item
	for rows.Next() {
		var it item
		var in int
		if err := rows.Scan(&it.id, &it.canonical, &it.rank, &it.status, &it.score, &in); err != nil {
			return nil, err
		}
		it.inArea = in != 0
		out = append(out, it)
	}
	return out, rows.Err()
}

func ftsRowCount(ctx context.Context, conn *sql.DB, prefix string) (int, error) {
	var n int
	err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM fts_name WHERE fts_name MATCH ?`, ftsToken(prefix)).Scan(&n)
	return n, err
}

func countRank(items []item, rank string) int {
	n := 0
	for _, it := range items {
		if it.rank == rank {
			n++
		}
	}
	return n
}

func ord(r string) int {
	if o, ok := rankOrder[r]; ok {
		return o
	}
	return unknownRankOrder
}

// rankSuggestions mirrors internal/domain.RankSuggestions. PrefixHit is
// always true here, so that key is omitted.
func rankSuggestions(items []item) []item {
	out := append([]item(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.inArea != b.inArea {
			return a.inArea
		}
		aa, ba := a.status == "accepted", b.status == "accepted"
		if aa != ba {
			return aa
		}
		if ao, bo := ord(a.rank), ord(b.rank); ao != bo {
			return ao < bo
		}
		return a.score < b.score
	})
	return out
}

func sortedPrefixes(m map[string][]time.Duration) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}

func quantile(ds []time.Duration, p float64) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if len(s) == 0 {
		return 0
	}
	return s[int(p*float64(len(s)-1))].Round(time.Microsecond)
}

func minD(ds []time.Duration) time.Duration { return quantile(ds, 0) }
func maxD(ds []time.Duration) time.Duration { return quantile(ds, 1) }

func cv(ds []time.Duration) float64 {
	if len(ds) < 2 {
		return 0
	}
	var sum float64
	for _, d := range ds {
		sum += float64(d)
	}
	mean := sum / float64(len(ds))
	var v float64
	for _, d := range ds {
		v += (float64(d) - mean) * (float64(d) - mean)
	}
	v /= float64(len(ds) - 1)
	if mean == 0 {
		return 0
	}
	return 100 * math.Sqrt(v) / mean
}

func fmtDurs(ds []time.Duration) string {
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = d.Round(time.Microsecond).String()
	}
	return strings.Join(parts, " ")
}

func yesNo(b bool) string {
	if b {
		return "ja"
	}
	return "nein"
}
