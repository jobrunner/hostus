package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// conceptColumns is the shared column list for joining a taxon_concept to
// its accepted name and backbone version, used by Concept, ConceptByXref,
// and MatchExact so the three reads decode identically.
const conceptColumns = `
	tc.id, tc.backbone_id, bv.version, tc.rank, COALESCE(tc.parent_id, ''), COALESCE(tc.sec_reference, ''), tc.status,
	an.id, an.canonical, COALESCE(an.authorship, ''), an.rank, COALESCE(an.ipni_id, ''), COALESCE(an.published_in, ''), COALESCE(an.nom_status, ''), COALESCE(an.basionym_id, '')`

const conceptJoin = `
	FROM taxon_concept tc
	JOIN name an ON an.id = tc.accepted_name
	JOIN backbone_version bv ON bv.id = tc.backbone_id`

// scanConcept reads one row shaped like conceptColumns into a domain.Concept.
func scanConcept(scan func(dest ...any) error) (*domain.Concept, error) {
	var (
		c                                           domain.Concept
		conceptRank, parentID, secReference, status string
		an                                          domain.Name
		nameRank                                    string
	)
	if err := scan(
		&c.ID, &c.BackboneID, &c.BackboneVersion, &conceptRank, &parentID, &secReference, &status,
		&an.ID, &an.Canonical, &an.Authorship, &nameRank, &an.IPNIID, &an.PublishedIn, &an.NomStatus, &an.BasionymID,
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

	nRank, err := domain.ParseRank(nameRank)
	if err != nil {
		return nil, fmt.Errorf("sqlite: accepted name %q: %w", an.ID, err)
	}
	an.Rank = nRank
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
		SELECT n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, ''), cn.homotypic
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
	var rank string
	if err := scan(&n.ID, &n.Canonical, &n.Authorship, &rank, &n.IPNIID, &n.PublishedIn, &n.NomStatus, &n.BasionymID); err != nil {
		return nil, err
	}
	r, err := domain.ParseRank(rank)
	if err != nil {
		return nil, fmt.Errorf("name %q: %w", n.ID, err)
	}
	n.Rank = r
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
		SELECT cn.role,
			n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, ''),`+
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
func (db *DB) MatchFuzzyCandidates(ctx context.Context, canon string, limit int) ([]output.MatchCandidate, error) {
	want := domain.Canonicalize(canon)
	if want == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	ids, err := fuzzyCandidateNameIDs(ctx, db.sql, want, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying MatchFuzzyCandidates %q: %w", canon, err)
	}
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
		SELECT cn.role,
			n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, ''),`+
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
func fuzzyCandidateNameIDs(ctx context.Context, db *sql.DB, want string, limit int) ([]string, error) {
	firstRunePrefix := string([]rune(want)[:1]) + "*"

	rows, err := db.QueryContext(ctx, `
		SELECT id FROM name
		WHERE canonical_fold GLOB ?
		  AND ABS(length(canonical_fold) - length(?)) <= ?
		ORDER BY ABS(length(canonical_fold) - length(?)), canonical_fold
		LIMIT ?`, firstRunePrefix, want, fuzzyCandidateLengthWindow, want, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
		var matched domain.Name
		var matchedRank string
		var c domain.Concept
		var conceptRank, parentID, secReference, status string
		var an domain.Name
		var nameRank string
		if err := rows.Scan(
			&role,
			&matched.ID, &matched.Canonical, &matched.Authorship, &matchedRank, &matched.IPNIID, &matched.PublishedIn, &matched.NomStatus, &matched.BasionymID,
			&c.ID, &c.BackboneID, &c.BackboneVersion, &conceptRank, &parentID, &secReference, &status,
			&an.ID, &an.Canonical, &an.Authorship, &nameRank, &an.IPNIID, &an.PublishedIn, &an.NomStatus, &an.BasionymID,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scanning %s %q row: %w", op, arg, err)
		}

		mRank, err := domain.ParseRank(matchedRank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: matched name %q: %w", matched.ID, err)
		}
		matched.Rank = mRank

		cRank, err := domain.ParseRank(conceptRank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: concept %q: %w", c.ID, err)
		}
		c.Rank = cRank
		c.ParentID = parentID
		c.SecReference = secReference
		c.Status = domain.ParseStatus(status)

		aRank, err := domain.ParseRank(nameRank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: accepted name %q: %w", an.ID, err)
		}
		an.Rank = aRank
		c.AcceptedName = an

		out = append(out, output.MatchCandidate{Concept: c, MatchedName: matched, Role: role})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating %s %q rows: %w", op, arg, err)
	}
	return out, nil
}
