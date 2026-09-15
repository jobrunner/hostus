package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// areaCodes resolves a Repository.Suggest/output.SuggestOpts.Area value
// into the set of WGSRPD level-3 area codes to match against
// distribution.area_code. An empty area returns nil (no area filter — see
// Suggest's doc comment on the empty-Area convention).
//
// The alias table itself lives in domain.AreaCodes: the application layer
// validates a caller-supplied area against the same table and must not
// import this adapter (depguard), so a copy here would be a second,
// silently divergent truth.
func areaCodes(area string) []string {
	return domain.AreaCodes(area)
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
	query, args, ok := buildSuggestQuery(q, opts)
	if !ok {
		return nil, nil
	}

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

	if err := db.attachTargetSpaceNames(ctx, out, opts.TargetSpace, domain.IsAggregateName(q)); err != nil {
		return nil, err
	}
	// TargetSpaceHit can only be decided once the space name is known, so it
	// is set here rather than in the query. Both sides go through
	// StripAggregateMarkers, the same measure ftsPrefixToken and
	// nameStartFilter already apply — so the spaces' inconsistent marker
	// SPELLINGS cannot decide this signal. Measured on the production index:
	// eurosl (251 aggregate entries) and floraveg (211) write "… aggr.",
	// germansl (614) writes "… agg.", and "… s. l." occurs as well, so the raw
	// canonical forms made "Alyssum montanum agg." miss eurosl's "Alyssum
	// montanum aggr." on the "." vs "r".
	//
	// That was only HALF of what broke aggregate queries, and an earlier
	// version of this comment wrongly presented it as the whole fix. The other
	// half is the EMPTY name ResolveTargetSpace returns when a space carries
	// no aggregate entry at all — by far the more common case, and the reason
	// this criterion was dead for whole pages. It is handled in
	// attachTargetSpaceNames (see the AGGREGATE FALLBACK note there); marker
	// folding alone would not have helped, because there is no spelling to
	// compare when there is no name.
	//
	// Without a requested space TargetSpaceName is "" for every item, and ""
	// never has a non-empty query as its prefix, so the signal stays false
	// throughout — no guard needed.
	prefix := domain.StripAggregateMarkers(domain.Canonicalize(q))
	for i := range out {
		spaceName := domain.StripAggregateMarkers(domain.Canonicalize(out[i].TargetSpaceName))
		out[i].TargetSpaceHit = strings.HasPrefix(spaceName, prefix)
	}

	if err := db.attachMatchedNames(ctx, out, prefix); err != nil {
		return nil, err
	}
	return out, nil
}

// matchedNameQuery is attachMatchedNames' query, a package-level constant for
// the same reason targetSpaceQuery is one: so the EXPLAIN regression test
// plans the EXACT string production runs.
//
// It selects every name of the listed concepts that the query prefix reaches
// and lets SQLite apply MOST of the precedence, so the caller walks the rows
// of a concept in order and only has to apply the ONE key SQLite cannot
// express (see below):
//
//  1. exact equality before a mere prefix — someone who typed the whole name
//     is shown that name, not a longer one that also starts with it;
//  2. accepted before synonym — the same tie-break domain.RankSuggestions
//     uses between concepts, applied here between one concept's names;
//  3. canonical, then authorship, then name id — not preferences, just the
//     tie-breaks that make the choice DETERMINISTIC, so two identical
//     requests cannot show the operator two different "Treffer-Name" values.
//     Authorship and id are NOT optional garnish: a homonym is precisely two
//     names of one concept with the same role and the same canonical that
//     differ ONLY in authorship, and the first three keys cannot separate
//     them. Measured on the production index: 5,784 (concept_id, role,
//     canonical_fold) groups hold more than one name — e.g.
//     wcvp:concept:1020 carries "Acalypha villicaulis Müll.Arg." and
//     "Acalypha villicaulis Hochst. ex A.Rich.". Without the last two keys
//     rowid order decides between them, and a plan or build change silently
//     flips which author the console shows. nm.id is the final backstop for
//     the (data-error) case of two rows identical in all four.
//
// THE ONE KEY THAT IS NOT HERE (spec 2026-09-15): "not disqualified before
// disqualified", which belongs between keys 1 and 2. It is missing on purpose,
// not by omission. Whether a nom_status cell records a DEFECT is decided by
// domain.ClassifyNomStatus — a botanically reviewed rule table, shared with
// the synonym endpoint — and restating it in SQL would be a second, silently
// divergent truth. The obvious SQL stand-in is worse than none:
// `COALESCE(nm.nom_status,”) <> ”` sorts by whether a status EXISTS, and
// "nom. cons." (1,237 names) exists while being the strongest possible
// assertion that the name is valid — it would demote conserved names exactly
// like illegitimate ones. So this query carries the raw cell as a column and
// attachMatchedNames applies that key in Go, over rows this ORDER BY already
// made deterministic. Nothing is lost: the query has no LIMIT, so every
// candidate row of a concept reaches Go.
//
// The LIKE term needs no unary-+ guard: SQLite's LIKE optimization only
// applies to a column with COLLATE NOCASE (or with case_sensitive_like ON),
// and canonical_fold has neither, so idx_name_canonical_fold is not a
// candidate driver for it in the first place — the json_each id list is.
const matchedNameQuery = `
	SELECT cn.concept_id, nm.canonical, COALESCE(nm.authorship, ''), cn.role,
	       COALESCE(nm.nom_status, ''),
	       CASE WHEN nm.canonical_fold = ? THEN 1 ELSE 0 END AS exact_name
	FROM concept_name cn
	JOIN name nm ON nm.id = cn.name_id
	WHERE cn.concept_id IN (SELECT value FROM json_each(?))
	  AND nm.canonical_fold LIKE ? || '%'
	ORDER BY cn.concept_id ASC, exact_name DESC,
	         CASE cn.role WHEN 'accepted' THEN 0 ELSE 1 END ASC,
	         nm.canonical ASC, COALESCE(nm.authorship, '') ASC, nm.id ASC`

// attachMatchedNames fills MatchedName on every item for which one of the
// concept's own names is reached by the query prefix. It is a second pass
// over the whole page in ONE query (like attachTargetSpaceNames) rather than
// a column on the main query, because the main query GROUP BYs to one row
// per concept and cannot carry a per-name choice through that aggregation.
//
// An item keeps the zero MatchedName when no name of its concept starts with
// the prefix — the "anywhere" match mode's case, where the hit came from a
// token inside a name. domain.SuggestItem.MatchedName documents that empty
// value as "could not be determined", so there is nothing to invent here:
// that item keeps an empty NomStatusJudgement too, which is the one place the
// field is unset rather than JudgementAbsent, because there is no NAME to
// judge — not a name judged to have nothing recorded.
//
// It also decides MatchedNameDisqualified, the ranking signal
// domain.RankSuggestions consumes (priority 2). Both live here for the same
// reason: they are two faces of ONE choice — which name is "the" hit — and
// deriving the flag anywhere else would let it describe a different name than
// the one shown.
func (db *DB) attachMatchedNames(ctx context.Context, items []domain.SuggestItem, prefix string) error {
	if len(items) == 0 {
		return nil
	}

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ConceptID
	}
	idsJSON, err := marshalIDs(ids)
	if err != nil {
		return err
	}

	rows, err := db.sql.QueryContext(ctx, matchedNameQuery, prefix, idsJSON, prefix)
	if err != nil {
		return fmt.Errorf("sqlite: suggest matched names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	matched := make(map[string]matchedNameChoice, len(items))
	for rows.Next() {
		var conceptID, rawStatus string
		var name domain.MatchedName
		var exact int
		if err := rows.Scan(&conceptID, &name.Canonical, &name.Authorship, &name.Role, &rawStatus, &exact); err != nil {
			return fmt.Errorf("sqlite: scanning suggest matched name row: %w", err)
		}
		// The verdict comes from domain, never from a token list restated
		// here: ClassifyNomStatus is what /v1/concept/{id}/synonyms judges
		// with, and suggest must not disagree with it about the same cell.
		verdict := domain.ClassifyNomStatus(rawStatus)
		name.NomStatus = verdict.Normalized
		name.NomStatusJudgement = verdict.Judgement
		candidate := matchedNameChoice{
			// ONLY JudgementDisqualifying lowers a name. JudgementAbsent
			// ("nothing recorded") and JudgementUnclassified ("recorded, but
			// no rule claims it" — "sensu auct.", "fossil name") are
			// uncertainty, and uncertainty must not read as a defect.
			name:         name,
			disqualified: verdict.Judgement == domain.JudgementDisqualifying,
		}

		previous, seen := matched[conceptID]
		if !seen {
			matched[conceptID] = candidate
			continue
		}
		// Rows arrive in matchedNameQuery's order, so the FIRST row already
		// wins every key except the disqualification one. Applying that key
		// is therefore a single rule: a disqualified incumbent yields to the
		// first valid row behind it, and nothing else ever displaces an
		// incumbent. Because the missing key sits directly BELOW exact
		// equality, skipping disqualified rows this way is exactly the full
		// ordering — among the non-disqualified rows the earliest one is
		// still the most exact, then accepted, then canonical/authorship/id.
		if previous.disqualified && !candidate.disqualified {
			matched[conceptID] = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterating suggest matched name rows: %w", err)
	}

	for i := range items {
		choice := matched[items[i].ConceptID]
		items[i].MatchedName = choice.name
		items[i].MatchedNameDisqualified = choice.disqualified
	}
	return nil
}

// matchedNameChoice is one candidate for a concept's "Treffer-Name", carrying
// the judgement-derived flag alongside the name so the selection loop compares
// booleans instead of re-classifying rows it has already judged.
type matchedNameChoice struct {
	name         domain.MatchedName
	disqualified bool
}

// buildSuggestQuery constructs the SQL and positional args for Suggest's FTS5
// prefix-match query. It is a pure function (no I/O) so the query plan can be
// asserted against a seeded test DB without exercising QueryContext/Scan —
// see TestSuggestQueryPlanDoesNotScanBackboneIndex. ok is false exactly when
// q's canonicalized prefix is shorter than minQueryRunes (see
// ftsPrefixToken), signaling Suggest to return an empty result without ever
// touching FTS5.
func buildSuggestQuery(q string, opts output.SuggestOpts) (query string, args []any, ok bool) {
	match := ftsPrefixToken(q)
	if match == "" {
		return "", nil, false
	}

	// args must be built in the same left-to-right order the placeholders
	// appear in the final query text below: match + pool cap (pool CTE), then
	// — only with an area — match again (match_rows CTE), the area codes for
	// in_area_rows, and the area codes for the in_area EXISTS (SELECT list),
	// then the exact_hit and prefix_hit prefixes (SELECT list), then the
	// rank-filter codes (WHERE), the backbone id (WHERE), the name_start
	// prefix (WHERE), the required target space (WHERE), then the LIMIT
	// budget.
	args = []any{match, suggestMatchPool}

	// prefix is the canonicalized, marker-stripped query the three
	// name-level signals below are all measured against — the same value
	// ftsPrefixToken built its MATCH token from, so "X agg." asks about "X"
	// here exactly as it does there.
	prefix := domain.StripAggregateMarkers(domain.Canonicalize(q))

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

	// name_match is ONE correlated subquery over the concept's names that
	// answers all three name-level questions as a step value:
	//
	//	2 = some name of the concept EQUALS the query prefix   -> exact_hit
	//	1 = some name STARTS with it                           -> prefix_hit,
	//	                                                          and the
	//	                                                          name_start
	//	                                                          admission test
	//	0 = neither (only an FTS token matched somewhere inside a name)
	//
	// It goes through name + concept_name because fts_name is contentless
	// (content='', see schema.sql): the text that matched cannot be read back
	// outside a MATCH, so this join is the only practical source of a
	// concept's name strings.
	//
	// WHY ONE INSTEAD OF THREE (performance, spec 2026-09-14 review): this
	// started as three separate correlated subqueries — a name_start EXISTS in
	// the WHERE plus an exact_hit and a prefix_hit EXISTS in the SELECT list.
	// The WHERE one was the expensive one: it ran once per MATCHED ROW, and a
	// concept owns one row per name, while the area path's in_area_rows UNION
	// pushes that row count far past the bm25 pool. Measured against the
	// production index, q=ca&area=GER went from ~2s to ~5s. Folding all three
	// into this single per-CONCEPT subquery, with the admission test applied
	// to its result in the outer query (see nameStartFilter below), does the
	// name lookup ONCE per concept instead of once per name plus twice per
	// concept.
	//
	// exact_hit is the match_mode-independent signal PrefixHit cannot be: in
	// the default name_start mode every returned row is a prefix hit by
	// construction, so only equality still distinguishes "I typed the whole
	// name" from "I typed the beginning of a longer one". prefix_hit stops
	// being the constant true it was hard-coded as in scanSuggestItem as soon
	// as MatchMode is "anywhere", where a row can arrive via a token match
	// somewhere INSIDE a name.
	//
	// It needs no unary-+ planner guard, unlike the backbone and space terms:
	// it is correlated on cn.concept_id = tc.id from the outer row, not a join
	// term the planner could pick as the outer driver.
	//
	// COALESCE because MAX() over an empty set is NULL, and a NULL step value
	// would make both derived flags NULL rather than false.
	args = append(args, prefix) // name_match: equality arm
	args = append(args, prefix) // name_match: prefix arm
	const nameMatchExpr = `COALESCE((
			SELECT MAX(CASE WHEN nm.canonical_fold = ? THEN 2
			                WHEN nm.canonical_fold LIKE ? || '%' THEN 1
			                ELSE 0 END)
			FROM name nm JOIN concept_name cn ON cn.name_id = nm.id
			WHERE cn.concept_id = tc.id
		), 0)`

	rankFilter := ""
	if len(opts.Ranks) > 0 {
		placeholders := make([]string, len(opts.Ranks))
		for i, r := range opts.Ranks {
			placeholders[i] = "?"
			args = append(args, string(r))
		}
		rankFilter = fmt.Sprintf(" AND tc.rank IN (%s)", strings.Join(placeholders, ","))
	}

	// The backbone filter sits in the WHERE clause, so SQLite applies it
	// before GROUP BY and LIMIT. Filtering after the query would be useless
	// for the case this exists for: "Inula" in area GER matches ~19 CDM
	// concepts (one per German flora) and one WCVP concept, so a post-filter
	// on a page of results keeps a single row at best.
	//
	// It deliberately does NOT filter the bm25 pool above. The pool is filled
	// with no backbone awareness, so a majority backbone can crowd out a
	// minority one: for the prefix "ca" CDM has 4345 matching names but only
	// 292 reach the 5000-row pool. Those 292 are still CDM's MOST relevant
	// matches, which is more than a result page needs — remeasured on the
	// real index (spec 2026-09-05, `serve` against
	// out/hostus-deploy-v3.1.0-alpha.0.sqlite): entry_backbone=cdm&q=ca
	// returns a full, sensible page in ~0.13s steady-state (curl time_total;
	// supersedes the earlier 0.29s measurement, same index shape). Pushing
	// the filter into the pool was tried and rejected: it requires
	// joining taxon_concept (a TEXT primary key) for every one of the ~104k
	// matches BEFORE the cap applies, which measured 5-8x slower (0.29s ->
	// 1.7-2.4s) — reintroducing exactly the 502 latency the pool prevents,
	// for candidates domain.RankSuggestions would discard anyway.
	// The one case where relevance-truncation is not acceptable is in_area,
	// which outranks bm25 — and the in-area union above already recovers those
	// rows for every backbone (pinned by TestSuggest_BackboneKeepsInAreaBeyondPool).
	//
	// PLANNER TRAP (spec 2026-09-05, real 502 on the Synology instance): a
	// plain `tc.backbone_id = ?` LOOKS selective — one predicate, one value —
	// but is not on the MAJORITY backbone: wcvp alone holds ~440k of
	// taxon_concept's rows, so "backbone = wcvp" only excludes a small
	// minority of the table. SQLite's planner, seeing idx_taxon_concept_
	// backbone_id and no ANALYZE statistics to tell it otherwise, still
	// picked that index to DRIVE the join — scanning every one of those
	// ~440k concepts and running the correlated nameStartFilter EXISTS
	// subquery once per row, instead of starting from the ~40 rows the FTS5
	// MATCH above already narrowed things down to. Measured on the real
	// index (entry_backbone=wcvp&q=Inula+hirta): 6.96s driven by the index
	// vs 0.0023s once suppressed — identical result set. The unary `+` on
	// the column (`+tc.backbone_id = ?`) is SQLite's documented idiom for
	// "do not use an index for this term": it makes the expression opaque
	// to the index-selection analysis without changing its truth value, so
	// the planner falls back to driving from `matches`/fts_name_map as it
	// does without a backbone filter, and applies this predicate only as a
	// post-join check. SQLite parses `+X` as an expression, not a column
	// reference, so the column's type AFFINITY no longer applies to the
	// comparison (datatype3 §4.2) — harmless here because backbone_id is
	// TEXT/BINARY with no COLLATE in schema.sql and the bound parameter is
	// already a Go string, so no affinity conversion was happening anyway;
	// this does NOT carry over to an INTEGER column compared against a
	// string parameter, where dropping the affinity conversion would change
	// which rows match.
	//
	// rankFilter (`tc.rank IN (...)`, above) does NOT need the same `+`:
	// tc.rank carries no index (see schema.sql — no idx_taxon_concept_rank),
	// so there is no index for the planner to mistakenly prefer here today;
	// anyone adding one must re-read this comment first.
	backboneFilter := ""
	if opts.Backbone != "" {
		backboneFilter = " AND +tc.backbone_id = ?"
		args = append(args, opts.Backbone)
	}

	// nameStartFilter is applied in the OUTER query, to name_match's already
	// computed step value — not as its own EXISTS in the inner WHERE, where
	// it used to sit. Two reasons, one of them measured:
	//
	//   - correctness is unchanged: name_match is a property of the CONCEPT
	//     (it looks at every name of tc.id), not of the individual matched
	//     row, so filtering per row before GROUP BY and filtering per group
	//     after it admit exactly the same concepts;
	//   - cost is not: the inner WHERE ran the subquery once per MATCHED ROW
	//     — one row per name, and with an area the in_area_rows UNION pushes
	//     that count far past the pool — while this runs it once per concept.
	//
	// The LIMIT still applies after this filter (it sits in the outer query
	// alongside it), so the fetch budget is spent on admitted concepts exactly
	// as before.
	nameStartFilter := ""
	if opts.MatchMode != "anywhere" {
		// name_start (Default): only concepts carrying AT LEAST ONE name
		// (accepted OR synonym) whose FULL canonicalized string starts with
		// the query prefix — not merely some FTS5 token inside it. Fixes the
		// SP7 finding ("ca" matched Corynephorus canescens via the epithet
		// token "canescens"). This checks ANY name of the concept, not only
		// the specific fts_name row the outer query happened to join through
		// — fts_name is a contentless FTS5 table (content='', see schema.sql),
		// so its own canonical value cannot be read back outside a MATCH; the
		// name+concept_name join is the only practical route to it.
		//
		// A real aggregate name-space alias (e.g. "Corynephorus canescens
		// agg.") is deliberately NOT given its own exemption arm here: per
		// internal/application/namespace_ingest.go's aggregate-to-nominate
		// rule, a resolved aggregate alias always carries the SAME genus+
		// epithet as its concept's accepted name, only with a marker
		// appended — so a genuine aggregate query (marker-stripped by the
		// same domain.StripAggregateMarkers call below) already reduces to
		// that accepted name's own prefix and is caught by the EXISTS below
		// without any extra clause. An earlier version of this filter added
		// a second EXISTS arm that exempted a concept from name_start
		// whenever ANY of its is_aggregate=1 fts_name rows was among the
		// FTS matches — that arm checked only ROW MEMBERSHIP, never whether
		// the matched text itself started with the prefix, so a bare-
		// epithet query (e.g. "canescens", which only ever matches the
		// aggregate alias's EPITHET TOKEN via the same position-independent
		// FTS5 behavior this filter exists to close) silently exempted the
		// whole concept from name_start again — reopening the exact SP7 bug
		// for every concept carrying an aggregate alias. See
		// TestSuggest_NameStart_AggregateAliasDoesNotExemptBareEpithetQuery.
		//
		// No placeholder and no arg: the prefix was already bound once, into
		// name_match.
		nameStartFilter = " AND name_match >= 1"
	}

	// spaceFilter is the only clause that makes opts.TargetSpace FILTER
	// rather than merely enrich. It sits in the same WHERE block as the
	// backbone filter and for the same reason: ahead of GROUP BY and LIMIT.
	// Dropping entry-less concepts from the returned page afterwards would
	// keep almost nothing — they are exactly the rows the unfiltered query
	// hands back first.
	//
	// WHAT IT CANNOT RECOVER: suggestMatchPool caps the bm25 pool BEFORE this
	// WHERE ever runs, and unlike the area filter the space has no union arm
	// that fetches back what the cap dropped. Measured for q="ca" on the
	// production index: 623 concepts with a eurosl entry are inside the pool,
	// 2,963 are in the full match set — 79% are unreachable no matter what
	// this filter does. Harmless today (623 already exceeds any fetch budget,
	// 120 for limit=30), but a sparsely populated space plus a broad prefix
	// can structurally starve the page. If that ever shows up, the fix is an
	// in_area_rows-style union arm for the space, not a bigger pool.
	//
	// It requires BOTH halves: RequireTargetSpace alone has no space to
	// require an entry in, and binding an empty space here would drop every
	// row instead. (The HTTP layer rejects that combination with a 400, so
	// this branch is the defensive half of the same rule.)
	//
	// PLANNER GUARD, same table and same term as targetSpaceQuery's SECOND
	// instance of the trap (see its doc comment): `nse.space = ?` on
	// name_space_entry's (space, ext_id) primary key is what made the
	// planner drive from that autoindex and scan the whole space (124,730
	// eurosl rows in the current production index) instead of probing
	// idx_name_space_entry_concept_id. `+nse.space` makes the term opaque to
	// index selection without changing its truth value; it is sound here for
	// the same reason it is there — space is a TEXT column with no COLLATE
	// and the bound parameter is a Go string, so no affinity conversion was
	// in play.
	//
	// Honest scope of the guard: in THIS shape the flip does not currently
	// occur even without the +. The EXISTS is correlated on concept_id =
	// tc.id, so the space term is not a join term the planner could promote
	// to the driver; EXPLAIN QUERY PLAN against the real 2.2 GB index
	// (out/hostus-deploy-v3.4.0-alpha.0.sqlite, which carries no sqlite_stat1)
	// gives the identical plan with and without it, probing
	// idx_name_space_entry_concept_id either way. The + stays because it
	// costs nothing, because the same term two tables over DID flip, and
	// because a future ANALYZE would give the planner exactly the row counts
	// that make the space index look attractive. Pinned by
	// TestSuggestQueryPlanDoesNotScanNameSpaceEntry, which controls over the
	// FORM instead: written as a plain JOIN, this same filter DOES flip onto
	// the PK autoindex — so the correlated shape is load-bearing here, and
	// anyone rewriting it into a join reopens the trap the + alone would not
	// close.
	spaceFilter := ""
	if opts.RequireTargetSpace && opts.TargetSpace != "" {
		spaceFilter = ` AND EXISTS (
			SELECT 1 FROM name_space_entry nse
			WHERE nse.concept_id = tc.id AND +nse.space = ?
		)`
		args = append(args, opts.TargetSpace)
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
	// exact_hit LEADS the ORDER BY, ahead of in_area, for the same reason
	// in_area leads score (see TestSuggest_InAreaCandidateSurvivesFetchBudget
	// Overflow): this LIMIT is the fetch budget, applied BEFORE the caller's
	// domain.RankSuggestions ever runs, so a signal that outranks in_area in
	// the final ranking must also outrank it here — otherwise the budget can
	// truncate away the very row the caller typed in full.
	//
	// TargetSpaceHit, the ranking key BETWEEN exact_hit and in_area, has no
	// counterpart here on purpose: it is only known after the second pass
	// (attachTargetSpaceNames), which needs this query's page as its input.
	// The concepts it would pull forward are already inside the budget
	// whenever RequireTargetSpace narrows the page to them, and it never
	// decides membership on its own — only order among rows that all matched.
	//
	// The two-level shape (inner grouped SELECT, outer filter/sort/limit) is
	// what lets name_match be computed ONCE and then used three times: SQLite
	// has no LATERAL join, and repeating the subquery text in the WHERE would
	// hand the planner the very per-row work this fold removed. The outer
	// query's WHERE therefore reads the inner query's output column.
	query = `WITH ` + cteClause + `
		SELECT id, canonical, rank, status, score, in_area,
		       name_match >= 2 AS exact_hit, name_match >= 1 AS prefix_hit,
		       sec_reference, aggregate
		FROM (
			SELECT tc.id AS id, an.canonical AS canonical, an.rank AS rank, tc.status AS status,
			       MIN(m.score) AS score, ` + inAreaExpr + ` AS in_area, ` + nameMatchExpr + ` AS name_match,
			       COALESCE(tc.sec_reference, '') AS sec_reference, MAX(fnm.is_aggregate) AS aggregate
			FROM matches m
			JOIN fts_name_map fnm ON fnm.rowid = m.rowid
			JOIN taxon_concept tc ON tc.id = fnm.concept_id
			JOIN name an ON an.id = tc.accepted_name
			WHERE 1 = 1` + rankFilter + backboneFilter + spaceFilter + `
			GROUP BY tc.id
		)
		WHERE 1 = 1` + nameStartFilter + `
		ORDER BY exact_hit DESC, in_area DESC, score ASC
		LIMIT ?`

	return query, args, true
}

// targetSpaceQuery is attachTargetSpaceNames' query. It is a package-level
// constant (not inlined at the call site) so
// TestAttachTargetSpaceNamesQueryPlanDoesNotScanSpace can EXPLAIN QUERY PLAN
// the EXACT string production runs, rather than a hand-copied stand-in that
// could silently drift from it.
//
// SECOND INSTANCE of the backboneFilter planner trap (see that doc comment
// in Suggest for the full mechanism) — same fix, same reasoning, different
// table: `space = ?` looks selective but name_space_entry's PRIMARY KEY is
// (space, ext_id), so the space column is only the LEADING part of that
// composite index; for a populous space (eurosl: 116k rows) the planner
// still picked that PK-derived autoindex (sqlite_autoindex_name_space_entry_1)
// to drive the query, scanning every eurosl row and testing concept_id
// membership per row instead of starting from idx_name_space_entry_concept_id
// via the ≤ suggestFetchMultiplier*limit concept ids already selected by
// Suggest. Measured on the real index (target_space=eurosl, a Suggest page's
// worth of concept ids): 0.455s driven by the PK index vs 0.001s once
// suppressed — identical 5-row result. `+space = ?` applies the same unary-+
// idiom to fix it. The fix does give up the row order the (space, ext_id)
// PK used to hand `ORDER BY ext_id ASC` for free, in exchange for an
// explicit TEMP B-TREE sort — semantically identical (still ext_id ASC,
// so the resolver's tie-break stays deterministic) and confirmed to
// produce the same row order on the real index.
const targetSpaceQuery = `
	SELECT concept_id, name, aggregate, COALESCE(status, '')
	FROM name_space_entry
	WHERE +space = ? AND concept_id IN (SELECT value FROM json_each(?))
	ORDER BY ext_id ASC`

// attachTargetSpaceNames fills TargetSpaceName on every item that has a
// spelling in space. It runs ONE query for the whole page rather than one per
// hit: a suggest page holds up to the fetch budget of concepts, and a
// per-concept lookup would turn a single keystroke into dozens of round trips.
func (db *DB) attachTargetSpaceNames(ctx context.Context, items []domain.SuggestItem, space string, queryIsAggregate bool) error {
	if space == "" || len(items) == 0 {
		return nil
	}

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ConceptID
	}
	idsJSON, err := marshalIDs(ids)
	if err != nil {
		return err
	}

	// A concept usually holds SEVERAL spellings in one space — measured on the
	// real index, 45% of concepts with a eurosl entry carry 2 to 391 of them,
	// because a space maps its own synonyms onto one backbone concept. The
	// space's ACCEPTED spelling is therefore the only non-arbitrary answer,
	// and it is what a caller carrying the name downstream needs.
	//
	// The rows are fetched and then resolved by domain.ResolveTargetSpace —
	// the SAME function /v1/match resolves with. Restating the precedence as
	// an ORDER BY was tried and rejected: it made the two endpoints answer
	// differently for one concept, and it cannot express the rule at all,
	// because which spelling is right depends on the HIT (was it reached via
	// an aggregate alias?) and not only on the entries.
	//
	// ext_id ASC only makes the input order stable, so the resolver's own
	// fallback ("no entry is marked accepted") is deterministic rather than
	// whatever the store returns first.
	rows, err := db.sql.QueryContext(ctx, targetSpaceQuery, space, idsJSON)
	if err != nil {
		return fmt.Errorf("sqlite: suggest target space %q: %w", space, err)
	}
	defer func() { _ = rows.Close() }()

	entries := make(map[string][]domain.NameSpaceEntry, len(items))
	for rows.Next() {
		var conceptID string
		var e domain.NameSpaceEntry
		var aggregate int
		if err := rows.Scan(&conceptID, &e.Name, &aggregate, &e.Status); err != nil {
			return fmt.Errorf("sqlite: scanning target space %q row: %w", space, err)
		}
		e.Space = space
		e.Aggregate = aggregate != 0
		entries[conceptID] = append(entries[conceptID], e)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterating target space %q rows: %w", space, err)
	}

	// queryIsAggregate comes from the QUERY, exactly as it does on the match
	// path (isAggregate over the caller's verbatim). SuggestItem.Aggregate is
	// a different thing and must not be used here: it is MAX(is_aggregate)
	// over the matched names, so a concept that owns any aggregate alias
	// carries it even when the query matched only the plain name — using it
	// would name a plain query with the aggregate spelling.
	// AGGREGATE FALLBACK, and why it lives here and not in
	// domain.ResolveTargetSpace: on an aggregate query that function answers
	// AggregatePolicyUnresolvable and NO name whenever the space carries no
	// is_aggregate entry for the concept. That is right for /v1/match and
	// /v1/translate, whose caller (Habitatus) uses the name to ASSERT that a
	// vegetation record meets an ESy expression — handing back the
	// microspecies spelling there produces exactly the false "not met" the
	// source document warns against, which is why the rule exists.
	//
	// Suggest asks a different question. It is a picker: target_space_name
	// answers "what is this concept called in that space", and "Alyssum
	// montanum" IS eurosl's (accepted) name for wcvp:concept:2632304 — saying
	// so claims nothing about whether eurosl can express the AGGREGATE. Worse,
	// without this fallback the endpoint contradicted itself:
	// require_target_space keeps only concepts that HAVE an entry in the
	// space, and every one of those rows then came back with an empty name,
	// which the console badges as "kein Name … lässt sich dort nicht
	// benennen". And since TargetSpaceHit is derived from this name, the
	// spec's decisive ranking criterion was dead for EVERY aggregate query —
	// not a rare case: eurosl carries 251 aggregate entries, floraveg 211,
	// germansl 614, against ~125k entries in eurosl alone.
	//
	// The guard is on the empty NAME rather than on the policy so the
	// non-aggregate path (which already falls back internally) stays a no-op
	// here, and so a concept with no entries at all keeps "" instead of
	// gaining a second meaning.
	for i := range items {
		conceptEntries := entries[items[i].ConceptID]
		choice, _ := domain.ResolveTargetSpace(queryIsAggregate, conceptEntries)
		if choice.Name == "" { // ONLY when the space offered no aggregate name
			choice, _ = domain.ResolveTargetSpace(false, conceptEntries)
		}
		items[i].TargetSpaceName = choice.Name
	}
	return nil
}

// scanSuggestItem decodes one Suggest result row into a domain.SuggestItem.
// ExactHit and PrefixHit are read from the query's own columns (see
// buildSuggestQuery): PrefixHit used to be hard-coded true here on the
// grounds that every row came from an FTS5 MATCH, which is only true in the
// default name_start mode — under MatchMode "anywhere" a row can arrive via
// a token match INSIDE a name, and claiming a prefix hit for it fed
// domain.RankSuggestions a constant where it expects a signal.
func scanSuggestItem(scan func(dest ...any) error) (domain.SuggestItem, error) {
	var item domain.SuggestItem
	var rank, status string
	var inArea, exactHit, prefixHit, aggregate int
	if err := scan(&item.ConceptID, &item.Canonical, &rank, &status, &item.Score, &inArea, &exactHit, &prefixHit, &item.SecReference, &aggregate); err != nil {
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
	item.ExactHit = exactHit != 0
	item.PrefixHit = prefixHit != 0
	return item, nil
}
