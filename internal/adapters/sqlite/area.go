package sqlite

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
)

// UpsertArea records one area's name keyed by (scheme, code). INSERT OR IGNORE:
// the first non-empty name for a code wins, so calling it once per distinct
// area during ingest is safe and order-independent. An empty name writes
// nothing (the code stays nameless rather than storing "").
func (t *ingestTx) UpsertArea(a domain.Area) error {
	if a.Name == "" {
		return nil
	}
	if _, err := t.tx.ExecContext(t.ctx,
		`INSERT OR IGNORE INTO area (scheme, code, name) VALUES (?, ?, ?)`,
		a.Scheme, a.Code, a.Name); err != nil {
		return fmt.Errorf("sqlite: upserting area %s:%s: %w", a.Scheme, a.Code, err)
	}
	return nil
}

// Areas lists every distribution area that carries data — a DISTINCT
// (area_scheme, area_code) from the distribution table — each joined to its
// ingested name (empty when none), ordered by (scheme, code). Only
// areas-with-data are returned, so a picker built from this never offers a
// region that yields nothing.
func (db *DB) Areas(ctx context.Context) ([]domain.Area, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT d.area_scheme, d.area_code, COALESCE(a.name, '')
		FROM (SELECT DISTINCT area_scheme, area_code FROM distribution) d
		LEFT JOIN area a ON a.scheme = d.area_scheme AND a.code = d.area_code
		ORDER BY d.area_scheme, d.area_code`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying areas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Area
	for rows.Next() {
		var a domain.Area
		if err := rows.Scan(&a.Scheme, &a.Code, &a.Name); err != nil {
			return nil, fmt.Errorf("sqlite: scanning area row: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating area rows: %w", err)
	}
	return out, nil
}
