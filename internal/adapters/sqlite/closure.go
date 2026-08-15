package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// BuildDistributionClosure (re)builds distribution_effective from scratch: every
// concept's own distribution (origin 'own'), plus — for a concept with NO own
// distribution and a non-empty accepted canonical_fold — the areas of any WCVP
// concept sharing that fold (origin 'name', the precomputed in_area name
// fallback). Idempotent: safe to run repeatedly (ingest finalize + Open
// self-heal). The `wtc.backbone_id = 'wcvp'` join is fine here (batch build, not
// a per-row correlated subquery, so no adverse plan — unlike Suggest).
func (db *DB) BuildDistributionClosure(ctx context.Context) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: closure begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := []string{
		`DELETE FROM distribution_effective`,
		`INSERT OR IGNORE INTO distribution_effective (concept_id, area_scheme, area_code, origin)
		 SELECT concept_id, area_scheme, area_code, 'own' FROM distribution`,
		`INSERT OR IGNORE INTO distribution_effective (concept_id, area_scheme, area_code, origin)
		 SELECT c.id, wd.area_scheme, wd.area_code, 'name'
		 FROM taxon_concept c
		 JOIN name an ON an.id = c.accepted_name
		 JOIN name wn ON wn.canonical_fold = an.canonical_fold
		 JOIN concept_name wcn ON wcn.name_id = wn.id
		 JOIN taxon_concept wtc ON wtc.id = wcn.concept_id AND wtc.backbone_id = 'wcvp'
		 JOIN distribution wd ON wd.concept_id = wtc.id
		 WHERE an.canonical_fold <> ''
		   AND NOT EXISTS (SELECT 1 FROM distribution d0 WHERE d0.concept_id = c.id)`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite: closure build: %w", err)
		}
	}
	return tx.Commit()
}

// distributionClosureEmpty reports whether distribution_effective needs a
// self-heal build: it has no rows but distribution does.
func distributionClosureEmpty(ctx context.Context, sqlDB *sql.DB) (bool, error) {
	var effN, distN int
	if err := sqlDB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM distribution_effective),
		(SELECT count(*) FROM distribution)`).Scan(&effN, &distN); err != nil {
		return false, fmt.Errorf("sqlite: closure emptiness check: %w", err)
	}
	return effN == 0 && distN > 0, nil
}
