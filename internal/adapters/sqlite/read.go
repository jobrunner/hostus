package sqlite

import (
	"context"
	"database/sql"
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
func (db *DB) Concept(ctx context.Context, id string) (*domain.Concept, []domain.Name, []domain.Xref, []domain.Distribution, error) {
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

func (db *DB) conceptSynonyms(ctx context.Context, conceptID string) ([]domain.Name, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.ipni_id, ''), COALESCE(n.published_in, ''), COALESCE(n.nom_status, ''), COALESCE(n.basionym_id, '')
		FROM concept_name cn
		JOIN name n ON n.id = cn.name_id
		WHERE cn.concept_id = ? AND cn.role = 'synonym'
		ORDER BY n.id`, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying synonyms of concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Name
	for rows.Next() {
		n, err := scanName(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scanning synonym of concept %q: %w", conceptID, err)
		}
		out = append(out, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating synonyms of concept %q: %w", conceptID, err)
	}
	return out, nil
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
// plays there. It binds domain.Canonicalize(canon) — not the raw input —
// into the SQL filter, so whitespace runs are collapsed before compare;
// the SQL LOWER() comparison is a cheap ASCII prefilter, and the
// authoritative check re-applies domain.Canonicalize per row so
// diacritic-folding stays in lockstep with the FTS5 tokenizer parity
// established in fts_parity_internal_test.go, rather than depending on
// SQLite's own (ASCII-only) LOWER() for that part.
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
		WHERE LOWER(n.canonical) = LOWER(?)
		ORDER BY tc.id, n.id`, want)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying MatchExact %q: %w", canon, err)
	}
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
			return nil, fmt.Errorf("sqlite: scanning MatchExact %q row: %w", canon, err)
		}

		if domain.Canonicalize(matched.Canonical) != want {
			continue
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
		return nil, fmt.Errorf("sqlite: iterating MatchExact %q rows: %w", canon, err)
	}
	return out, nil
}
