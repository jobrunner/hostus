package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO-free, FTS5 built in) — see ADR-0010

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

//go:embed schema.sql
var schemaSQL string

// DB is a modernc.org/sqlite-backed output.Repository.
type DB struct {
	sql *sql.DB
}

var _ output.Repository = (*DB)(nil)

// Open opens (or creates) the SQLite database at path and applies the
// embedded schema (idempotent — every DDL statement is IF NOT EXISTS).
// path may be ":memory:" for an ephemeral, in-process database, as used by
// tests. Foreign-key enforcement is turned on for the returned connection.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	if _, err := sqlDB.ExecContext(context.Background(), schemaSQL); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("sqlite: applying schema: %w", err)
	}
	return &DB{sql: sqlDB}, nil
}

// Close releases the underlying database handle.
func (db *DB) Close() error {
	return db.sql.Close()
}

// BackboneVersions lists every ingested backbone artifact.
func (db *DB) BackboneVersions(ctx context.Context) ([]domain.BackboneVersion, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, version, license, source_url, ingested_at, manifest_sha
		FROM backbone_version
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying backbone_version: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.BackboneVersion
	for rows.Next() {
		var bv domain.BackboneVersion
		var license, sourceURL sql.NullString
		if err := rows.Scan(&bv.ID, &bv.Version, &license, &sourceURL, &bv.IngestedAt, &bv.ManifestSHA); err != nil {
			return nil, fmt.Errorf("sqlite: scanning backbone_version row: %w", err)
		}
		bv.License = license.String
		bv.SourceURL = sourceURL.String
		out = append(out, bv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating backbone_version rows: %w", err)
	}
	return out, nil
}

// BeginIngest records bv into backbone_version and starts a transaction
// scoping every write of one backbone import, so a failed/partial ingest
// never leaves the index half-written. Callers must Commit or Rollback the
// returned IngestTx.
func (db *DB) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: beginning ingest transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO backbone_version (id, version, license, source_url, ingested_at, manifest_sha)
		VALUES (?, ?, ?, ?, ?, ?)`,
		bv.ID, bv.Version, bv.License, bv.SourceURL, bv.IngestedAt, bv.ManifestSHA,
	); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("sqlite: recording backbone_version %q: %w", bv.ID, err)
	}
	return &ingestTx{ctx: ctx, tx: tx}, nil
}

// ingestTx implements output.IngestTx over a single *sql.Tx.
type ingestTx struct {
	ctx context.Context
	tx  *sql.Tx
}

var _ output.IngestTx = (*ingestTx)(nil)

// nullableFK converts an optional foreign-key value into a driver value
// that SQLite treats as NULL when empty, rather than as the literal empty
// string. This matters for nullable FK columns (name.basionym_id,
// taxon_concept.parent_id): SQLite only exempts NULL child keys from FK
// checking, so writing "" instead of NULL would fail the constraint unless
// a row with id="" happens to exist.
func nullableFK(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (t *ingestTx) UpsertName(n domain.Name) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Canonical, domain.Canonicalize(n.Canonical), n.Authorship, string(n.Rank), n.IPNIID, n.PublishedIn, n.NomStatus, nullableFK(n.BasionymID),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting name %q: %w", n.ID, err)
	}
	return nil
}

func (t *ingestTx) UpsertConcept(c domain.Concept) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.BackboneID, c.AcceptedName.ID, string(c.Rank), nullableFK(c.ParentID), c.SecReference, string(c.Status),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting concept %q: %w", c.ID, err)
	}
	return nil
}

func (t *ingestTx) LinkName(conceptID, nameID, role string, homotypic *bool) error {
	var h any
	if homotypic != nil {
		if *homotypic {
			h = 1
		} else {
			h = 0
		}
	}
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO concept_name (concept_id, name_id, role, homotypic)
		VALUES (?, ?, ?, ?)`,
		conceptID, nameID, role, h,
	)
	if err != nil {
		return fmt.Errorf("sqlite: linking name %q to concept %q: %w", nameID, conceptID, err)
	}
	return nil
}

func (t *ingestTx) AddXref(conceptID string, x domain.Xref) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO xref (concept_id, authority, ext_id)
		VALUES (?, ?, ?)`,
		conceptID, x.Authority, x.ExtID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding xref %s:%s for concept %q: %w", x.Authority, x.ExtID, conceptID, err)
	}
	return nil
}

func (t *ingestTx) AddDistribution(conceptID string, d domain.Distribution) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO distribution (concept_id, area_scheme, area_code)
		VALUES (?, ?, ?)`,
		conceptID, d.AreaScheme, d.AreaCode,
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding distribution %s/%s for concept %q: %w", d.AreaScheme, d.AreaCode, conceptID, err)
	}
	return nil
}

func (t *ingestTx) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: committing ingest transaction: %w", err)
	}
	return nil
}

func (t *ingestTx) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("sqlite: rolling back ingest transaction: %w", err)
	}
	return nil
}
