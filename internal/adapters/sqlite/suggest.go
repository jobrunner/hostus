package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// wgsrpdGermanyL3 is the single WGSRPD level-3 code WCVP uses for Germany.
const wgsrpdGermanyL3 = "GER"

// wgsrpdAlias maps a small set of convenience area names to their WGSRPD
// level-3 area code(s). Any output.SuggestOpts.Area value not found here
// (case-insensitively) is treated as a raw WGSRPD level-3 code and passed
// through unchanged (upper-cased) — so a caller can always bypass the alias
// table entirely by supplying an exact L3 code (e.g. "GER") directly. Add
// further aliases here as UC1's frontend needs them. "AT"/"CH" were added
// alongside Task 4's multi-area bundle scoping (BundleOpts.Area,
// resolveAreaCodes in bundle.go) so a Mitteleuropa bundle can be requested
// as "hostus bundle --area DE,AT,CH", mirroring the ISO-3166 alpha-2 style
// "DE" already used, rather than requiring the raw WGSRPD codes
// (GER/AUT/SWI) for two of the three countries but not the first.
var wgsrpdAlias = map[string][]string{
	"DE": {wgsrpdGermanyL3},
	"AT": {"AUT"},
	"CH": {"SWI"},
}

// areaCodes resolves a Repository.Suggest/output.SuggestOpts.Area value
// into the set of WGSRPD level-3 area codes to match against
// distribution.area_code. An empty area returns nil (no area filter — see
// Suggest's doc comment on the empty-Area convention).
func areaCodes(area string) []string {
	if strings.TrimSpace(area) == "" {
		return nil
	}
	key := strings.ToUpper(strings.TrimSpace(area))
	if codes, ok := wgsrpdAlias[key]; ok {
		return codes
	}
	return []string{key}
}

// minQueryRunes is the minimum domain.Canonicalize'd length of q that
// Suggest will search on. Below this, ftsPrefixToken returns "" and Suggest
// returns an empty result without ever touching FTS5: a 0- or 1-rune
// prefix is both a meaningless autosuggest signal and a pathologically
// broad FTS5 MATCH (effectively "everything").
const minQueryRunes = 2

// suggestFetchMultiplier and suggestFetchFloor set Suggest's SQL-level
// fetch budget (see the Repository.Suggest doc comment): Suggest asks
// SQLite for more rows than the caller's target Limit so that the Ranks
// filter and the per-concept de-duplication (one concept can own many
// fts_name rows — one per accepted+synonym name) never starve the
// caller's later domain.RankSuggestions + truncate step of candidates
// that would otherwise have made the cut.
const (
	suggestFetchMultiplier = 4
	suggestFetchFloor      = 20
)

func fetchBudget(limit int) int {
	if limit <= 0 {
		limit = suggestFetchFloor
	}
	if n := limit * suggestFetchMultiplier; n > suggestFetchFloor {
		return n
	}
	return suggestFetchFloor
}

// suggestMatchPool caps how many FTS prefix matches Suggest ranks and groups.
// A 2-rune prefix like "ca" matches ~100k names on the full index; joining,
// grouping and computing in_area over all of them costs ~1.8s (and behind a
// proxy shows up as a 502/504). The FTS prefix scan itself is cheap (~12ms) —
// the cost is the downstream work per matched row. So the matches CTE keeps
// only the top suggestMatchPool rows by bm25 relevance before that work; the
// dropped tail is the least-relevant matches, which never survive
// domain.RankSuggestions into the caller's (much smaller) limit anyway. This
// is a NO-OP for any query matching fewer than suggestMatchPool names (Poa,
// care, essentially every non-pathological query) — those are unchanged bit
// for bit. It is a package var, not a const, only so tests can shrink it.
var suggestMatchPool = 5000

// ftsPrefixToken turns q into a SQLite FTS5 MATCH query string performing a
// left-anchored prefix search over q's canonical form. It is
// injection-safe against FTS5's query syntax (which gives special meaning
// to *, -, (, ), " and bareword operators like AND/OR/NOT): the
// canonicalized token is wrapped in a double-quoted FTS5 string literal,
// with any embedded `"` doubled per FTS5's own escaping rule, which strips
// special meaning from every character inside the quotes; the single
// trailing `*` placed OUTSIDE the quotes is what turns the quoted phrase
// into a prefix query (FTS5 supports "phrase"* as a documented prefix-query
// form). Returns "" if the canonicalized query is shorter than
// minQueryRunes, signaling Suggest to skip the query entirely.
func ftsPrefixToken(q string) string {
	// Strip any trailing aggregate marker so the marker SPELLING is irrelevant:
	// "X agg.", "X aggr." and "X s.l." all search the base X (see
	// domain.StripAggregateMarkers). Combined with the aggregate name-space
	// aliases indexed at ingest, an aggregate query reliably reaches its taxon.
	token := domain.StripAggregateMarkers(domain.Canonicalize(q))
	if len([]rune(token)) < minQueryRunes {
		return ""
	}
	escaped := strings.ReplaceAll(token, `"`, `""`)
	return `"` + escaped + `"*`
}

// Suggest returns FTS5 prefix-match candidates for q. See the
// output.Repository.Suggest doc comment for the full contract (unranked
// results, the fetch-budget note, and the empty-q/empty-Area conventions).
func (db *DB) Suggest(ctx context.Context, q string, opts output.SuggestOpts) ([]domain.SuggestItem, error) {
	match := ftsPrefixToken(q)
	if match == "" {
		return nil, nil
	}

	// args must be built in the same left-to-right order the placeholders
	// appear in the final query text below: match + pool cap (pool CTE), then
	// — only with an area — match again (match_rows CTE), the area codes for
	// in_area_rows, and the area codes for the in_area EXISTS (SELECT list),
	// then the rank-filter codes (WHERE), then the LIMIT budget.
	args := []any{match, suggestMatchPool}

	codes := areaCodes(opts.Area)

	// cteClause feeds the final SELECT's `matches` source. Without an area it is
	// just the bm25 relevance pool (top suggestMatchPool matches). With an area
	// the pool alone would silently drop in-area concepts whose prefix relevance
	// is poor — and in_area is the PRIMARY rank key, so in a SPARSE area (fewer
	// in-area concepts than a result page) those would vanish from page 1. So we
	// UNION the pool with in_area_rows: every prefix match whose concept has an
	// effective (own OR closure-derived name-fallback) distribution in the area,
	// found cheaply via idx_distribution_effective_area joined to the bm25-free
	// match_rows membership set (no second full ranking pass). Union-only rows
	// carry a sentinel score so they sort after real pool hits but, being
	// in_area, still ahead of every not-in-area concept.
	cteClause := `matches AS MATERIALIZED (
			SELECT rowid, bm25(fts_name) AS score
			FROM fts_name WHERE fts_name MATCH ? ORDER BY score LIMIT ?
		)`

	// in_area is a POSITIVE presence test against the precomputed
	// distribution_effective closure, which already folds in both a concept's
	// own distribution and — for a concept with none of its own — its WCVP
	// name twin's distribution (see BuildDistributionClosure). A false result
	// means "no positive evidence", never "absent". The codes are bound twice
	// with an area (in_area_rows, in_area EXISTS). Built with literal-format
	// Sprintf so gosec sees untainted SQL.
	inAreaExpr := "0"
	if len(codes) != 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(codes)), ",")
		codeArgs := make([]any, len(codes))
		for i, c := range codes {
			codeArgs[i] = c
		}
		args = append(args, match)       // match_rows MATCH ?
		args = append(args, codeArgs...) // in_area_rows area codes
		args = append(args, codeArgs...) // in_area EXISTS area codes

		// match_rows is the FULL prefix match set as bare rowids (no bm25, so
		// cheap ~12ms) purely to test membership; the bm25 ranking still only
		// happens on the bounded pool. in_area_rows recovers in-area matches
		// (own or closure-derived) that the pool dropped.
		cteClause = fmt.Sprintf(`pool AS MATERIALIZED (
			SELECT rowid, bm25(fts_name) AS score
			FROM fts_name WHERE fts_name MATCH ? ORDER BY score LIMIT ?
		),
		match_rows AS MATERIALIZED (SELECT rowid FROM fts_name WHERE fts_name MATCH ?),
		in_area_rows AS (
			SELECT DISTINCT fnm.rowid
			FROM distribution_effective de
			JOIN fts_name_map fnm ON fnm.concept_id = de.concept_id
			WHERE de.area_scheme = 'wgsrpd_l3' AND de.area_code IN (%s)
			  AND fnm.rowid IN (SELECT rowid FROM match_rows)
		),
		matches AS (
			SELECT rowid, score FROM pool
			UNION
			SELECT rowid, 1e18 FROM in_area_rows WHERE rowid NOT IN (SELECT rowid FROM pool)
		)`, ph)

		inAreaExpr = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM distribution_effective de
			WHERE de.concept_id = tc.id AND de.area_scheme = 'wgsrpd_l3' AND de.area_code IN (%s)
		)`, ph)
	}

	rankFilter := ""
	if len(opts.Ranks) > 0 {
		placeholders := make([]string, len(opts.Ranks))
		for i, r := range opts.Ranks {
			placeholders[i] = "?"
			args = append(args, string(r))
		}
		rankFilter = fmt.Sprintf(" AND tc.rank IN (%s)", strings.Join(placeholders, ","))
	}

	args = append(args, fetchBudget(opts.Limit))

	// bm25(fts_name) can only be evaluated directly against fts_name's own
	// MATCH'd cursor — SQLite rejects it ("unable to use function bm25 in
	// the requested context") once the outer query GROUP BYs. A plain
	// (non-MATERIALIZED) CTE is not enough to isolate it either: SQLite's
	// query flattening optimization can still inline the CTE's SELECT
	// into the outer GROUP BY query, reproducing the same error (verified
	// empirically against modernc.org/sqlite). MATERIALIZED forces the CTE
	// to run and store its results before the outer query touches them, so
	// bm25(fts_name) only ever executes in a plain, unaggregated SELECT;
	// the outer query then aggregates the already-materialized score
	// column (MIN(m.score), not MIN(bm25(...))) when collapsing a
	// concept's several matching names (accepted + synonyms) into one row.
	query := `WITH ` + cteClause + `
		SELECT tc.id, an.canonical, an.rank, tc.status, MIN(m.score) AS score, ` + inAreaExpr + ` AS in_area, COALESCE(tc.sec_reference, '') AS sec_reference, MAX(fnm.is_aggregate) AS aggregate
		FROM matches m
		JOIN fts_name_map fnm ON fnm.rowid = m.rowid
		JOIN taxon_concept tc ON tc.id = fnm.concept_id
		JOIN name an ON an.id = tc.accepted_name
		WHERE 1 = 1` + rankFilter + `
		GROUP BY tc.id
		ORDER BY in_area DESC, score ASC
		LIMIT ?`

	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: suggest %q: %w", q, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.SuggestItem
	for rows.Next() {
		item, err := scanSuggestItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scanning suggest %q row: %w", q, err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating suggest %q rows: %w", q, err)
	}
	return out, nil
}

// scanSuggestItem decodes one Suggest result row into a domain.SuggestItem.
// PrefixHit is always true: every row Suggest produces came from an FTS5
// MATCH, so there is no other value it could carry here.
func scanSuggestItem(scan func(dest ...any) error) (domain.SuggestItem, error) {
	var item domain.SuggestItem
	var rank, status string
	var inArea, aggregate int
	if err := scan(&item.ConceptID, &item.Canonical, &rank, &status, &item.Score, &inArea, &item.SecReference, &aggregate); err != nil {
		return domain.SuggestItem{}, err
	}
	r, err := domain.ParseRank(rank)
	if err != nil {
		return domain.SuggestItem{}, fmt.Errorf("concept %q: %w", item.ConceptID, err)
	}
	item.Rank = r
	item.Status = domain.ParseStatus(status)
	item.Display = item.Canonical
	item.InArea = inArea != 0
	item.Aggregate = aggregate != 0
	item.PrefixHit = true
	return item, nil
}
