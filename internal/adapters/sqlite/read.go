package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// conceptColumns is the shared column list for joining a taxon_concept to
// its accepted name and backbone version, used by Concept, ConceptByXref,
// and MatchExact so the three reads decode identically.
const conceptColumns = `
	tc.id, tc.backbone_id, bv.version, tc.rank, COALESCE(tc.parent_id, ''), COALESCE(tc.sec_reference, ''), tc.status, COALESCE(tc.rank_verbatim, ''),
	an.id, an.canonical, COALESCE(an.authorship, ''), an.rank, COALESCE(an.ipni_id, ''), COALESCE(an.published_in, ''), COALESCE(an.nom_status, ''), COALESCE(an.basionym_id, ''), COALESCE(an.rank_verbatim, '')`

const conceptJoin = `
	FROM taxon_concept tc
	JOIN name an ON an.id = tc.accepted_name
	JOIN backbone_version bv ON bv.id = tc.backbone_id`

// scanConcept reads one row shaped like conceptColumns into a domain.Concept.
func scanConcept(scan func(dest ...any) error) (*domain.Concept, error) {
	var (
		c                                           domain.Concept
		conceptRank, parentID, secReference, status string
		conceptRankVerbatim                         string
		an                                          domain.Name
		nameRank, nameRankVerbatim                  string
	)
	if err := scan(
		&c.ID, &c.BackboneID, &c.BackboneVersion, &conceptRank, &parentID, &secReference, &status, &conceptRankVerbatim,
		&an.ID, &an.Canonical, &an.Authorship, &nameRank, &an.IPNIID, &an.PublishedIn, &an.NomStatus, &an.BasionymID, &nameRankVerbatim,
	); err != nil {
		return nil, err
	}

	rank, err := domain.ParseRank(conceptRank)
	if err != nil {
		return nil, fmt.Errorf("sqlite: concept %q: %w", c.ID, err)
	}
	c.Rank = rank
	c.ParentID = parentID
	c.SecReference = secReference
	c.Status = domain.ParseStatus(status)
	if rank == domain.RankOther {
		c.RankVerbatim = conceptRankVerbatim
	}

	nRank, err := domain.ParseRank(nameRank)
	if err != nil {
		return nil, fmt.Errorf("sqlite: accepted name %q: %w", an.ID, err)
	}
	an.Rank = nRank
	if nRank == domain.RankOther {
		an.RankVerbatim = nameRankVerbatim
	}
	c.AcceptedName = an

	return &c, nil
}

// Concept resolves a taxon_concept by id, returning its accepted concept,
// its synonym names, its cross-references, and its distribution.
func (db *DB) Concept(ctx context.Context, id string) (*domain.Concept, []output.SynonymName, []domain.Xref, []domain.Distribution, error) {
	row := db.sql.QueryRowContext(ctx, `SELECT`+conceptColumns+conceptJoin+` WHERE tc.id = ?`, id)
	concept, err := scanConcept(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil, fmt.Errorf("sqlite: concept %q: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("sqlite: querying concept %q: %w", id, err)
	}

	synonyms, err := db.conceptSynonyms(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	xrefs, err := db.conceptXrefs(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	dists, err := db.conceptDistribution(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return concept, synonyms, xrefs, dists, nil
}

func (db *DB) conceptSynonyms(ctx context.Context, conceptID string) ([]output.SynonymName, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, ''), COALESCE(n.rank_verbatim, ''), cn.homotypic
		FROM concept_name cn
		JOIN name n ON n.id = cn.name_id
		WHERE cn.concept_id = ? AND cn.role = 'synonym'
		ORDER BY n.id`, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying synonyms of concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []output.SynonymName
	for rows.Next() {
		var homotypic sql.NullBool
		n, err := scanName(func(dest ...any) error {
			return rows.Scan(append(dest, &homotypic)...)
		})
		if err != nil {
			return nil, fmt.Errorf("sqlite: scanning synonym of concept %q: %w", conceptID, err)
		}
		sn := output.SynonymName{Name: *n}
		if homotypic.Valid {
			sn.Homotypic = &homotypic.Bool
		}
		out = append(out, sn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating synonyms of concept %q: %w", conceptID, err)
	}
	return out, nil
}

// maxClassificationDepth bounds Classification's upward parent_id walk, so
// a cyclic or otherwise corrupt parent_id chain can never hang the request
// — 10 hops comfortably exceeds any real taxonomic rank depth this system
// models (FAMILY > ... > FORM is far shallower).
const maxClassificationDepth = 10

// Classification walks conceptID's taxon_concept.parent_id chain upward,
// bounded to maxClassificationDepth hops, and returns the ancestor chain
// ROOT-FIRST: index 0 is the topmost ancestor reached, and the last element
// is conceptID's immediate parent. conceptID itself is never included. A
// NULL parent_id (no further ancestor) or hitting the depth bound (a
// cyclic/corrupt chain) both stop the walk without error — a partial or
// empty chain is a normal, valid result.
func (db *DB) Classification(ctx context.Context, conceptID string) ([]domain.ClassificationEntry, error) {
	exists, err := db.conceptExists(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: checking concept %q exists: %w", conceptID, err)
	}
	if !exists {
		return nil, fmt.Errorf("sqlite: concept %q: %w", conceptID, domain.ErrNotFound)
	}

	var chain []domain.ClassificationEntry
	current := conceptID
	for i := 0; i < maxClassificationDepth; i++ {
		var parentID sql.NullString
		if err := db.sql.QueryRowContext(ctx, `SELECT parent_id FROM taxon_concept WHERE id = ?`, current).Scan(&parentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, fmt.Errorf("sqlite: walking classification for concept %q: %w", conceptID, err)
		}
		if !parentID.Valid || parentID.String == "" {
			break
		}

		var canonical, rank string
		err := db.sql.QueryRowContext(ctx, `
			SELECT an.canonical, tc.rank
			FROM taxon_concept tc JOIN name an ON an.id = tc.accepted_name
			WHERE tc.id = ?`, parentID.String).Scan(&canonical, &rank)
		if errors.Is(err, sql.ErrNoRows) {
			// parent_id names a concept id no longer present (shouldn't
			// happen under FK enforcement, but stop defensively rather
			// than erroring the whole request).
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sqlite: reading classification ancestor %q: %w", parentID.String, err)
		}
		r, err := domain.ParseRank(rank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: classification ancestor %q: %w", parentID.String, err)
		}
		chain = append(chain, domain.ClassificationEntry{ConceptID: parentID.String, Canonical: canonical, Rank: r})
		current = parentID.String
	}

	// i < j is a genuinely equivalent mutant at CONDITIONALS_BOUNDARY (<=):
	// for an odd-length chain, i <= j would run one further iteration where
	// i == j, swapping chain[i] with itself — a no-op, producing the exact
	// same final order either way. No test can observe the difference.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// scanName reads a name row shaped like conceptSynonyms'/MatchExact's
// per-name column list into a domain.Name.
func scanName(scan func(dest ...any) error) (*domain.Name, error) {
	var n domain.Name
	var rank, rankVerbatim string
	if err := scan(&n.ID, &n.Canonical, &n.Authorship, &rank, &n.IPNIID, &n.PublishedIn, &n.NomStatus, &n.BasionymID, &rankVerbatim); err != nil {
		return nil, err
	}
	r, err := domain.ParseRank(rank)
	if err != nil {
		return nil, fmt.Errorf("name %q: %w", n.ID, err)
	}
	n.Rank = r
	if r == domain.RankOther {
		n.RankVerbatim = rankVerbatim
	}
	return &n, nil
}

// conceptStringPairs runs a two-column (concept_id-scoped) query and
// collects each row's two string columns via collect. It backs both
// conceptXrefs and conceptDistribution, which otherwise differ only in
// their SQL text and the two-field struct they build.
func conceptStringPairs(ctx context.Context, db *DB, query, what, conceptID string, collect func(a, b string)) error {
	rows, err := db.sql.QueryContext(ctx, query, conceptID)
	if err != nil {
		return fmt.Errorf("sqlite: querying %s of concept %q: %w", what, conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return fmt.Errorf("sqlite: scanning %s of concept %q: %w", what, conceptID, err)
		}
		collect(a, b)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterating %s of concept %q: %w", what, conceptID, err)
	}
	return nil
}

func (db *DB) conceptXrefs(ctx context.Context, conceptID string) ([]domain.Xref, error) {
	var out []domain.Xref
	err := conceptStringPairs(ctx, db, `
		SELECT authority, ext_id FROM xref WHERE concept_id = ? ORDER BY authority, ext_id`,
		"xrefs", conceptID, func(authority, extID string) {
			out = append(out, domain.Xref{Authority: authority, ExtID: extID})
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (db *DB) conceptDistribution(ctx context.Context, conceptID string) ([]domain.Distribution, error) {
	var out []domain.Distribution
	err := conceptStringPairs(ctx, db, `
		SELECT area_scheme, area_code FROM distribution WHERE concept_id = ? ORDER BY area_scheme, area_code`,
		"distribution", conceptID, func(areaScheme, areaCode string) {
			out = append(out, domain.Distribution{AreaScheme: areaScheme, AreaCode: areaCode})
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ConceptByXref resolves a taxon_concept via an external cross-reference.
func (db *DB) ConceptByXref(ctx context.Context, authority, extID string) (*domain.Concept, error) {
	row := db.sql.QueryRowContext(ctx, `SELECT`+conceptColumns+conceptJoin+`
		JOIN xref x ON x.concept_id = tc.id
		WHERE x.authority = ? AND x.ext_id = ?`, authority, extID)
	concept, err := scanConcept(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("sqlite: concept for xref %s:%s: %w", authority, extID, domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying concept for xref %s:%s: %w", authority, extID, err)
	}
	return concept, nil
}

// ConceptIDsByXref batch-resolves extIDs against the xref table for a single
// authority, returning only the ones that matched (map[extID]conceptID) — an
// extID absent from the returned map simply has no xref row for authority,
// exactly like MatchExact returning zero candidates.
//
// It binds extIDs as ONE parameter via `IN (SELECT value FROM json_each(?))`
// rather than a "?,?,?..." placeholder list, the same pattern
// MatchFuzzyCandidates and internal/adapters/sqlite/bundle.go established
// (see marshalIDs' doc comment there): the xref ingest resolves against the
// full 440k+-row backbone in one call, well past SQLite's
// SQLITE_MAX_VARIABLE_NUMBER for a placeholder list.
func (db *DB) ConceptIDsByXref(ctx context.Context, authority string, extIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(extIDs))
	if len(extIDs) == 0 {
		return out, nil
	}
	idsJSON, err := json.Marshal(extIDs)
	if err != nil {
		return nil, fmt.Errorf("sqlite: encoding ext_id list for authority %q: %w", authority, err)
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT ext_id, concept_id FROM xref
		WHERE authority = ? AND ext_id IN (SELECT value FROM json_each(?))`, authority, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying xref for authority %q: %w", authority, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var extID, conceptID string
		if err := rows.Scan(&extID, &conceptID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning xref row for authority %q: %w", authority, err)
		}
		out[extID] = conceptID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating xref rows for authority %q: %w", authority, err)
	}
	return out, nil
}

// MatchExact returns every name (accepted or synonym) whose canonical form
// equals canon, together with the concept it belongs to and the role it
// plays there. The match happens on the stored name.canonical_fold column
// — domain.Canonicalize(canonical), populated by IngestTx.UpsertName at
// write time — compared by plain equality against domain.Canonicalize(canon)
// computed here. This is diacritic-correct by construction: SQLite's own
// LOWER() only folds ASCII case, so filtering on LOWER(n.canonical)
// directly would silently miss diacritic-bearing names (the exact
// Central-European names this system exists for). The per-row
// domain.Canonicalize recheck below is belt-and-suspenders in case a
// canonical_fold value is ever stale or unpopulated (e.g. a row written
// outside IngestTx.UpsertName).
func (db *DB) MatchExact(ctx context.Context, canon string) ([]output.MatchCandidate, error) {
	want := domain.Canonicalize(canon)

	rows, err := db.sql.QueryContext(ctx, `
		SELECT cn.role, cn.homotypic,
			n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, ''), COALESCE(n.rank_verbatim, ''),`+
		conceptColumns+`
		FROM name n
		JOIN concept_name cn ON cn.name_id = n.id
		JOIN taxon_concept tc ON tc.id = cn.concept_id
		JOIN name an ON an.id = tc.accepted_name
		JOIN backbone_version bv ON bv.id = tc.backbone_id
		WHERE n.canonical_fold = ?
		ORDER BY tc.id, n.id`, want)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying MatchExact %q: %w", canon, err)
	}
	all, err := scanMatchCandidateRows(rows, "MatchExact", canon)
	if err != nil {
		return nil, err
	}

	// Belt-and-suspenders recheck (see doc comment above): canonical_fold
	// drives the SQL filter, but a stale/unpopulated fold column must never
	// silently widen the result beyond an exact match.
	var out []output.MatchCandidate
	for _, c := range all {
		if domain.Canonicalize(c.MatchedName.Canonical) != want {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// fuzzyCandidateLengthWindow bounds how far a fuzzy prefilter candidate's
// canonical-fold length may differ (in runes/bytes — canonical_fold is
// ASCII-folded, so the two coincide) from the query's, on either side.
const fuzzyCandidateLengthWindow = 3

// fuzzyCandidatePrefixRunes is how many leading runes of the query the
// prefilter's GLOB prefix pins.
//
// It is 4, not 1, and the difference is the difference between a working
// prefilter and a dead one. With one rune the pool is every name starting
// with that letter — measured against a full index, ~41.000 rows per query
// (docs/research/fuzzy-prefilter.md), which is more than any candidate budget
// can carry, so the intended target was never scored: 0.0 % recall in every
// error class. Four runes cuts the same pool to ~597 rows (largest observed:
// 8.081) and makes scoring the WHOLE pool affordable, which is what actually
// fixes recall — 96,6-100 % on typo classes, at 14-20 ms p95 instead of
// 651-1195 ms.
//
// The cost is a typo inside the prefix itself: a name misspelled in its
// first four runes is not in this pool (measured recall 1,2 %). That gap is
// covered by the second, epithet-keyed prefilter route, not by shortening
// the prefix — 3 runes leaves the same gap at position 2 while tripling the
// pool.
const fuzzyCandidatePrefixRunes = 4

// fuzzyCandidateCap bounds how many prefilter rows a single fuzzy lookup may
// carry, as a LOAD guard — not as a selection rule.
//
// That distinction is the bug this replaces. The old budget of 20 rows was
// applied to an alphabetically ordered pool of tens of thousands, so it
// decided WHICH candidates got scored, and it decided wrong every time. A cap
// only earns its place by being high enough never to decide anything on real
// data: the largest pool measured over 1.198 queries against a full index was
// 8.081 rows (docs/research/fuzzy-prefilter.md), and 20.000 clears that by
// ~2,5x while still bounding a pathological query.
//
// If it ever does bite, recall is silently degraded again — which is exactly
// the failure this whole change exists to end. fuzzyCandidateCapHits counts
// that, so the degradation is observable instead of invisible.
const fuzzyCandidateCap = 20000

// fuzzyCandidateCapHits counts fuzzy lookups whose candidate pool was
// truncated by fuzzyCandidateCap.
//
// It exists because the failure it watches for is invisible by nature. The
// prefilter returned the wanted name in 0.0 % of real queries for as long as
// it existed, and nothing anywhere said so — no error, no log line, just an
// answer that was quietly worse than it should be. A truncated pool is that
// same state. Any value above zero here means recall is degraded and the cap
// or the prefix needs re-measuring, not that a request failed.
var fuzzyCandidateCapHits = promauto.NewCounter(prometheus.CounterOpts{
	Name: "hostus_fuzzy_prefilter_cap_hits_total",
	Help: "Fuzzy lookups whose candidate pool hit the prefilter cap (recall may be degraded)",
})

// MatchFuzzyCandidates returns up to limit names that are cheap-to-find
// near-misses of canon, for the application layer to score with
// domain.Similarity. The prefilter is deliberately narrow so a fuzzy lookup
// never scans the whole name table:
//
//   - same first rune of canonical_fold as canon's (a typo essentially
//     never changes the first letter of a scientific name's genus),
//     expressed as a GLOB prefix pattern rather than substr(...)=? or a
//     LIKE pattern — SQLite's query planner turns a GLOB prefix into an
//     indexed range scan over idx_name_canonical_fold (confirmed via EXPLAIN
//     QUERY PLAN), whereas both substr() and (the by-default
//     case-insensitive) LIKE force a full table scan despite the index
//     existing; and
//   - canonical_fold length within fuzzyCandidateLengthWindow runes of
//     canon's, applied as a residual filter over that already-narrowed set.
//
// This filtering runs as its OWN query against the name table alone (see
// fuzzyCandidateNameIDs), not folded into one big join with
// concept_name/taxon_concept/backbone_version: tried as a single query, the
// planner (reasonably, by its own row-count estimates) chose to drive the
// join from taxon_concept and probe into the indexed name column per row —
// i.e. it still touched every taxon_concept row despite the index existing,
// exactly the whole-table scan this prefilter exists to avoid. Resolving
// the (at most limit) matched name IDs first, then joining ONLY those IDs
// outward to their concept/accepted-name/backbone-version context, keeps
// that second step's cost bounded by limit regardless of which join order
// the planner picks for it.
//
// Recall trade-off: a genuine near-miss whose first letter was itself
// mistyped, or whose length differs by more than the window (e.g. a
// dropped/added word), will NOT be returned — this is intentional; a
// prefilter that must also catch those would have to scan every row,
// defeating the purpose. limit <= 0 uses a modest built-in default.
func (db *DB) MatchFuzzyCandidates(ctx context.Context, canon string, limit int, backbone, sec string) ([]output.MatchCandidate, error) {
	want := domain.Canonicalize(canon)
	if want == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = fuzzyCandidateCap
	}

	ids, err := fuzzyCandidateNameIDs(ctx, db.sql, want, limit, backbone, sec)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying MatchFuzzyCandidates %q: %w", canon, err)
	}
	epithetIDs, err := fuzzyEpithetNameIDs(ctx, db.sql, want, limit, backbone, sec)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying MatchFuzzyCandidates %q by epithet: %w", canon, err)
	}
	ids = mergeIDs(ids, epithetIDs)
	if len(ids) == 0 {
		return nil, nil
	}

	// idsJSON binds the whole ID list as ONE parameter via json_each,
	// rather than building a "?,?,?..." placeholder list by runtime string
	// concatenation: the query text below is a fixed literal regardless of
	// len(ids), which keeps it a plain parameterized query (gosec's G202
	// rule flags any runtime-assembled SQL string, even placeholder-only
	// concatenation, and this repo's suppression-directive budget is zero
	// — see debt-guard.sh).
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("sqlite: encoding MatchFuzzyCandidates %q id list: %w", canon, err)
	}

	rows, err := db.sql.QueryContext(ctx, `
		SELECT cn.role, cn.homotypic,
			n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, ''), COALESCE(n.rank_verbatim, ''),`+
		conceptColumns+`
		FROM name n
		JOIN concept_name cn ON cn.name_id = n.id
		JOIN taxon_concept tc ON tc.id = cn.concept_id
		JOIN name an ON an.id = tc.accepted_name
		JOIN backbone_version bv ON bv.id = tc.backbone_id
		WHERE n.id IN (SELECT value FROM json_each(?))
		ORDER BY tc.id, n.id`, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying MatchFuzzyCandidates %q: %w", canon, err)
	}
	return scanMatchCandidateRows(rows, "MatchFuzzyCandidates", canon)
}

// fuzzyCandidateNameIDs runs MatchFuzzyCandidates' prefilter (GLOB first-rune
// prefix + length window) against the name table ALONE, returning up to
// limit matching name IDs. Isolating it from the enrichment join is what
// lets it lean on idx_name_canonical_fold — see MatchFuzzyCandidates' doc
// comment for why the combined-query version didn't.
//
// ORDER BY the length-window residual itself (closest length first, then
// canonical_fold for a deterministic tiebreak) before LIMIT: without an
// explicit order, a prefilter match count above limit would let SQLite
// return an arbitrary subset of the matching rows, potentially truncating
// away the true best (closest) match before domain.Similarity ever sees it.
// Ordering by the SAME residual the WHERE clause already computed doesn't
// change the query plan — confirmed via EXPLAIN QUERY PLAN, it still
// resolves via idx_name_canonical_fold, with the ordering applied as a
// cheap temp-B-tree sort over the already-narrowed row set, not a
// re-scan.
// globPrefix returns a GLOB pattern matching every string that begins with
// want's first n runes (fewer if want is shorter), those runes taken
// LITERALLY: each of GLOB's three metacharacters is wrapped in a
// single-character bracket set, since GLOB has no backslash escape but
// "[*]", "[?]" and "[[]" are the documented literal forms ("]" is only
// special INSIDE a bracket set, so it needs no escaping here).
//
// The escaping is not cosmetic — the prefix comes from a caller-controlled
// query. An unescaped "[" would open a bracket set that the appended "*"
// never closes (an unterminated set matches nothing, silently disabling
// fuzzy matching), and an unescaped "*" or "?" would turn the whole prefix
// filter into a no-op that scans the entire name table — exactly the
// whole-table scan the prefilter exists to avoid.
func globPrefix(want string, n int) string {
	runes := []rune(want)
	if n > len(runes) {
		n = len(runes)
	}
	var b strings.Builder
	for _, r := range runes[:n] {
		switch r {
		case '*', '?', '[':
			b.WriteByte('[')
			b.WriteRune(r)
			b.WriteByte(']')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString("*")
	return b.String()
}

// fuzzyCandidateRows runs the near-miss prefilter SELECT: up to `limit` name
// ids whose fold shares canon's first rune and is within the length window,
// closest length first. It always JOINs through to taxon_concept and applies
// the backbone/sec filter (both "" = no restriction, via the `? = ” OR …`
// guards) BEFORE the LIMIT — so a SP5 resolution filter never loses the wanted
// space's genuine near-miss to the limit when out-of-space same-length names
// crowd the top-N (the multi-sec case the filter exists to serve). Joining
// unconditionally is also why an orphaned name (no concept link) is not a
// candidate here: it could never survive MatchFuzzyCandidates' own concept
// join downstream anyway, so excluding it up front only frees prefilter slots
// for real near-misses. One FIXED query literal (never runtime-assembled SQL —
// gosec G202, and this repo's suppression budget is zero).
func fuzzyCandidateRows(ctx context.Context, db *sql.DB, want, firstRunePrefix string, limit int, backbone, sec string) (*sql.Rows, error) {
	return db.QueryContext(ctx, `
		SELECT DISTINCT n.id FROM name n
		JOIN concept_name cn ON cn.name_id = n.id
		JOIN taxon_concept tc ON tc.id = cn.concept_id
		WHERE n.canonical_fold GLOB ?
		  AND ABS(length(n.canonical_fold) - length(?)) <= ?
		  AND (? = '' OR tc.backbone_id = ?)
		  AND (? = '' OR tc.sec_reference = ?)
		ORDER BY ABS(length(n.canonical_fold) - length(?)), n.canonical_fold
		LIMIT ?`, firstRunePrefix, want, fuzzyCandidateLengthWindow, backbone, backbone, sec, sec, want, limit)
}

// fuzzyEpithetNameIDs is MatchFuzzyCandidates' SECOND prefilter route: the
// names of every concept whose fts_name entry contains the query's epithet as
// a token, inside the same length window and under the same backbone/sec
// filter as the prefix route.
//
// It exists for one measured reason. The prefix route cannot see a name whose
// GENUS is misspelled — recall 1,2 % on that class — because a prefix filter
// keyed on the query can only find names the query spells the same way at the
// start. The epithet is the part a genus typo leaves intact, and fts_name's
// unicode61 tokenizer already indexes it as a word, so this needs no new
// index. Together the two routes cover every measured typo class at
// 99,4-100 %.
//
// Two things it deliberately does not do:
//
//   - It does not widen recall for a genus RENAME. The target does land in
//     the candidate set (measured: 100 %), but a renamed genus scores below
//     domain.FuzzyThreshold as a whole string and so is never resolved to
//     (measured: 0 %). That is the scorer's judgement and correct — see
//     domain.FuzzyResolves — and it is why this route needs the genus guard
//     alongside it: an identical epithet is the longer half of a binomial and
//     will otherwise drag unrelated genera over the threshold.
//   - It does not reach a synonym whose epithet differs from its concept's
//     indexed accepted canonical. fts_name is indexed per CONCEPT, on the
//     accepted name; this returns all of that concept's names, but the token
//     that has to match is the accepted one's.
//
// A single-word query has no epithet and yields nothing here, leaving the
// prefix route alone — which is right, since there is no second word to
// disagree about.
func fuzzyEpithetNameIDs(ctx context.Context, db *sql.DB, want string, limit int, backbone, sec string) ([]string, error) {
	fields := strings.Fields(want)
	if len(fields) < 2 {
		return nil, nil
	}
	// The epithet is bound as a QUOTED FTS5 string, never bare: FTS5's query
	// syntax would otherwise read a caller-supplied "OR", "NEAR", "*" or ":"
	// as an operator — at best a syntax error inside a request handler, at
	// worst a pattern matching far more than the epithet.
	match := `"` + strings.ReplaceAll(fields[1], `"`, `""`) + `"`

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT n.id FROM fts_name f
		JOIN fts_name_map m ON m.rowid = f.rowid
		JOIN concept_name cn ON cn.concept_id = m.concept_id
		JOIN name n ON n.id = cn.name_id
		JOIN taxon_concept tc ON tc.id = m.concept_id
		WHERE fts_name MATCH ?
		  AND ABS(length(n.canonical_fold) - length(?)) <= ?
		  AND (? = '' OR tc.backbone_id = ?)
		  AND (? = '' OR tc.sec_reference = ?)
		ORDER BY ABS(length(n.canonical_fold) - length(?)), n.canonical_fold
		LIMIT ?`,
		match, want, fuzzyCandidateLengthWindow, backbone, backbone, sec, sec, want, limit)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows, limit)
}

// mergeIDs concatenates two id lists, dropping duplicates and preserving the
// first list's order — the prefix route's candidates stay first, since it is
// the narrower and more precise of the two.
func mergeIDs(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, id := range append(a, b...) {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func fuzzyCandidateNameIDs(ctx context.Context, db *sql.DB, want string, limit int, backbone, sec string) ([]string, error) {
	prefix := globPrefix(want, fuzzyCandidatePrefixRunes)

	rows, err := fuzzyCandidateRows(ctx, db, want, prefix, limit, backbone, sec)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows, limit)
}

// scanIDs collects a prefilter route's name ids and counts a cap hit.
//
// Exactly `limit` rows means the SQL LIMIT was reached, so the pool may have
// held more and the rest was dropped unscored. (It can also mean the pool held
// exactly `limit` rows and nothing was lost — a false positive on a counter
// whose whole job is to say "look again" is the harmless direction, and
// distinguishing the two would cost a second query.)
func scanIDs(rows *sql.Rows, limit int) ([]string, error) {
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == limit {
		fuzzyCandidateCapHits.Inc()
	}
	return ids, nil
}

// scanMatchCandidateRows decodes rows shaped like MatchExact's/
// MatchFuzzyCandidates' shared SELECT (role, matched name, concept,
// accepted name) into MatchCandidates, closing rows before returning. It
// does no canonical-equality filtering itself — callers that need an exact
// match (MatchExact) apply that afterward; MatchFuzzyCandidates wants the
// near-misses this returns as-is.
func scanMatchCandidateRows(rows *sql.Rows, op, arg string) ([]output.MatchCandidate, error) {
	defer func() { _ = rows.Close() }()

	var out []output.MatchCandidate
	for rows.Next() {
		var role string
		var homotypic sql.NullBool
		var matched domain.Name
		var matchedRank, matchedRankVerbatim string
		var c domain.Concept
		var conceptRank, parentID, secReference, status, conceptRankVerbatim string
		var an domain.Name
		var nameRank, nameRankVerbatim string
		if err := rows.Scan(
			&role, &homotypic,
			&matched.ID, &matched.Canonical, &matched.Authorship, &matchedRank, &matched.IPNIID, &matched.PublishedIn, &matched.NomStatus, &matched.BasionymID, &matchedRankVerbatim,
			&c.ID, &c.BackboneID, &c.BackboneVersion, &conceptRank, &parentID, &secReference, &status, &conceptRankVerbatim,
			&an.ID, &an.Canonical, &an.Authorship, &nameRank, &an.IPNIID, &an.PublishedIn, &an.NomStatus, &an.BasionymID, &nameRankVerbatim,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scanning %s %q row: %w", op, arg, err)
		}

		mRank, err := domain.ParseRank(matchedRank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: matched name %q: %w", matched.ID, err)
		}
		matched.Rank = mRank
		if mRank == domain.RankOther {
			matched.RankVerbatim = matchedRankVerbatim
		}

		cRank, err := domain.ParseRank(conceptRank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: concept %q: %w", c.ID, err)
		}
		c.Rank = cRank
		c.ParentID = parentID
		c.SecReference = secReference
		c.Status = domain.ParseStatus(status)
		if cRank == domain.RankOther {
			c.RankVerbatim = conceptRankVerbatim
		}

		aRank, err := domain.ParseRank(nameRank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: accepted name %q: %w", an.ID, err)
		}
		an.Rank = aRank
		if aRank == domain.RankOther {
			an.RankVerbatim = nameRankVerbatim
		}
		c.AcceptedName = an

		var ht *bool
		if homotypic.Valid {
			b := homotypic.Bool
			ht = &b
		}
		out = append(out, output.MatchCandidate{Concept: c, MatchedName: matched, Role: role, Homotypic: ht})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating %s %q rows: %w", op, arg, err)
	}
	return out, nil
}

// ExistingConceptIDs reports which of ids already have a taxon_concept row.
// Ids are bound as ONE json_each parameter for the same reason
// MatchFuzzyCandidates and the bundle export do it (see marshalIDs in
// bundle.go): a CDM ingest resolves tens of thousands of relation ends in a
// single call, well past SQLITE_MAX_VARIABLE_NUMBER for a "?,?,?..." list.
func (db *DB) ExistingConceptIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("sqlite: encoding concept id list: %w", err)
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id FROM taxon_concept
		WHERE id IN (SELECT value FROM json_each(?))`, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying existing concept ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: scanning concept id row: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating concept id rows: %w", err)
	}
	return out, nil
}

// SecReferences lists every ingested sec. reference space, ordered by id so
// the result is deterministic.
func (db *DB) SecReferences(ctx context.Context) ([]domain.SecReference, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT id, title FROM sec_reference ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying sec_reference: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.SecReference
	for rows.Next() {
		var s domain.SecReference
		if err := rows.Scan(&s.ID, &s.Title); err != nil {
			return nil, fmt.Errorf("sqlite: scanning sec_reference row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating sec_reference rows: %w", err)
	}
	return out, nil
}

// SecReferenceByID resolves one sec. reference space by id, reporting
// domain.ErrNotFound for an unknown one so /v1/translate can answer "you
// named a space that does not exist" rather than "no relation found".
func (db *DB) SecReferenceByID(ctx context.Context, id string) (domain.SecReference, error) {
	var s domain.SecReference
	err := db.sql.QueryRowContext(ctx, `SELECT id, title FROM sec_reference WHERE id = ?`, id).Scan(&s.ID, &s.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SecReference{}, fmt.Errorf("sqlite: sec reference %q: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.SecReference{}, fmt.Errorf("sqlite: querying sec reference %q: %w", id, err)
	}
	return s, nil
}

// conceptRelationPartnerJoin resolves the OTHER end of a concept_relation
// row relative to the queried concept: whichever of from_concept/to_concept
// is not it. The CASE is what makes one query cover both stored directions
// — hostus stores only the direction the source states, so an incoming edge
// is the only way to see e.g. "X includes <me>".
const conceptRelationPartnerJoin = `
	FROM concept_relation cr
	JOIN taxon_concept tc ON tc.id = CASE WHEN cr.from_concept = ? THEN cr.to_concept ELSE cr.from_concept END
	JOIN name an ON an.id = tc.accepted_name
	JOIN backbone_version bv ON bv.id = tc.backbone_id
	LEFT JOIN sec_reference sr ON sr.id = tc.sec_reference`

// ConceptRelationsInSec returns the one-hop concept relations between
// conceptID and the concepts of targetSec, in the direction each is stored.
//
// The self-edge exclusion (tc.id <> conceptID) is deliberate: a relation
// from a concept to itself is not a translation into another
// circumscription, and it would also make Outgoing meaningless (both ends
// would satisfy the CASE).
func (db *DB) ConceptRelationsInSec(ctx context.Context, conceptID, targetSec string) (output.ConceptRelations, error) {
	row := db.sql.QueryRowContext(ctx, `SELECT`+conceptColumns+conceptJoin+` WHERE tc.id = ?`, conceptID)
	source, err := scanConcept(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return output.ConceptRelations{}, fmt.Errorf("sqlite: concept %q: %w", conceptID, domain.ErrNotFound)
	}
	if err != nil {
		return output.ConceptRelations{}, fmt.Errorf("sqlite: querying concept %q: %w", conceptID, err)
	}
	out := output.ConceptRelations{Source: *source}

	rows, err := db.sql.QueryContext(ctx, `SELECT cr.relation, cr.from_concept, COALESCE(cr.source, ''), COALESCE(sr.title, ''),`+
		conceptColumns+conceptRelationPartnerJoin+`
		WHERE (cr.from_concept = ? OR cr.to_concept = ?)
		  AND tc.sec_reference = ?
		  AND tc.id <> ?
		ORDER BY tc.id, cr.relation, cr.source`,
		conceptID, conceptID, conceptID, targetSec, conceptID)
	if err != nil {
		return output.ConceptRelations{}, fmt.Errorf("sqlite: querying relations of concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var relation, fromConcept, source, secTitle string
		partner, err := scanConcept(func(dest ...any) error {
			return rows.Scan(append([]any{&relation, &fromConcept, &source, &secTitle}, dest...)...)
		})
		if err != nil {
			return output.ConceptRelations{}, fmt.Errorf("sqlite: scanning relation of concept %q: %w", conceptID, err)
		}
		rel, err := domain.ParseRelation(relation)
		if err != nil {
			return output.ConceptRelations{}, fmt.Errorf("sqlite: relation of concept %q: %w", conceptID, err)
		}
		out.Edges = append(out.Edges, output.ConceptRelationEdge{
			Partner:    *partner,
			PartnerSec: domain.SecReference{ID: partner.SecReference, Title: secTitle},
			Relation:   rel,
			Outgoing:   fromConcept == conceptID,
			Source:     source,
		})
	}
	if err := rows.Err(); err != nil {
		return output.ConceptRelations{}, fmt.Errorf("sqlite: iterating relations of concept %q: %w", conceptID, err)
	}
	return out, nil
}
