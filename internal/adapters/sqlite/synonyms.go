package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
)

// synonymCandidateQuery reads one concept's synonyms in the shape UC5's
// relevance model needs. It differs from conceptSynonyms' query in exactly
// two ways, and both are the point of the method existing:
//
//   - it joins taxon_concept -> its ACCEPTED name, so `is_basionym` can be
//     computed as "this synonym's name id IS the accepted name's
//     basionym_id" — the relation UC5 rule 4 orders by. Doing it in SQL
//     rather than in Go is what keeps it a single round trip; doing it at
//     all is what keeps the flag from being silently false everywhere (see
//     output.Repository.SynonymCandidates).
//   - it returns cn.homotypic raw (tri-state) alongside n.nom_status
//     verbatim.
//
// The `an.basionym_id IS NOT NULL AND ...` guard is not redundant with the
// equality: SQLite's `NULL = x` yields NULL, which the Go driver would then
// have to scan into a bool. Comparing explicitly keeps the column a plain
// 0/1 integer.
const synonymCandidateQuery = `
	SELECT n.id, n.canonical, COALESCE(n.authorship, ''), n.rank, COALESCE(n.nom_status, ''),
	       cn.homotypic,
	       (an.basionym_id IS NOT NULL AND an.basionym_id = n.id) AS is_basionym
	FROM concept_name cn
	JOIN name n ON n.id = cn.name_id
	JOIN taxon_concept tc ON tc.id = cn.concept_id
	JOIN name an ON an.id = tc.accepted_name
	WHERE cn.concept_id = ? AND cn.role = 'synonym'
	ORDER BY n.id`

// SynonymCandidates returns conceptID's synonyms as domain.SynonymCandidates,
// carrying nom_status, the tri-state homotypic flag and IsBasionym. See
// output.Repository.SynonymCandidates for the contract.
//
// Existence is checked first and separately, so an unknown concept id
// (domain.ErrNotFound) is never confused with a known concept that simply
// has no synonyms (an empty slice) — the row-count of the query below
// cannot tell those apart.
func (db *DB) SynonymCandidates(ctx context.Context, conceptID string) ([]domain.SynonymCandidate, error) {
	exists, err := db.conceptExists(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: checking concept %q exists: %w", conceptID, err)
	}
	if !exists {
		return nil, fmt.Errorf("sqlite: concept %q: %w", conceptID, domain.ErrNotFound)
	}

	rows, err := db.sql.QueryContext(ctx, synonymCandidateQuery, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying synonym candidates of concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.SynonymCandidate{}
	for rows.Next() {
		var (
			c         domain.SynonymCandidate
			rank      string
			homotypic sql.NullBool
		)
		if err := rows.Scan(&c.NameID, &c.Canonical, &c.Authorship, &rank, &c.NomStatus, &homotypic, &c.IsBasionym); err != nil {
			return nil, fmt.Errorf("sqlite: scanning synonym candidate of concept %q: %w", conceptID, err)
		}
		parsed, err := domain.ParseRank(rank)
		if err != nil {
			return nil, fmt.Errorf("sqlite: synonym %q of concept %q: %w", c.NameID, conceptID, err)
		}
		c.Rank = parsed
		c.ConceptID = conceptID
		if homotypic.Valid {
			// homotypic.Bool must be copied into a fresh variable: taking
			// &homotypic.Bool would alias the loop's scan destination, so
			// every candidate would end up pointing at the LAST row's value.
			value := homotypic.Bool
			c.Homotypic = &value
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating synonym candidates of concept %q: %w", conceptID, err)
	}
	return out, nil
}
