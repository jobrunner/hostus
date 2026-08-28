package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jobrunner/hostus/internal/domain"
)

// UpsertNameSpace records one name-space provenance row (SP9/UC4), the
// name-space counterpart of UpsertXrefSource. ingested_at is stamped here
// rather than taken from meta, for the same reason it is there: provenance
// timing is orthogonal to the domain-level fields callers construct.
func (t *ingestTx) UpsertNameSpace(meta domain.NameSpaceMeta) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO name_space (id, version, license, source_url, ingested_at, manifest_sha, redistribution)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		meta.ID, meta.Version, meta.License, meta.SourceURL, time.Now().UTC().Format(time.RFC3339), meta.ManifestSHA, string(meta.Redistribution),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting name space %s/%s: %w", meta.ID, meta.Version, err)
	}
	return nil
}

// AddNameSpaceEntry attaches one name-space spelling to conceptID. e.Name is
// written VERBATIM — it is the string a target-space caller gets back, so it
// must not be folded to the canonical match key. e.Resolution follows
// AddTraitValue's rule: an empty resolution (an exact canonical match) is
// stored as NULL, not as ”.
func (t *ingestTx) AddNameSpaceEntry(conceptID string, e domain.NameSpaceEntry) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO name_space_entry (space, ext_id, concept_id, name, aggregate, resolution, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Space, e.ExtID, conceptID, e.Name, boolToInt(e.Aggregate), nullString(e.Resolution), e.Status,
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding name space entry %s:%s for concept %q: %w", e.Space, e.ExtID, conceptID, err)
	}

	// Index a RESOLVED aggregate spelling as an fts_name alias for its concept,
	// flagged is_aggregate=1, so suggest finds the aggregate form and can badge
	// the hit. Only aggregate-marked, resolved entries — an unresolved one has
	// no concept to point at, and non-aggregate spellings are not suggest
	// aliases here.
	if e.Aggregate && conceptID != "" {
		res, err := t.tx.ExecContext(t.ctx,
			`INSERT INTO fts_name_map (concept_id, is_aggregate) VALUES (?, 1)`, conceptID)
		if err != nil {
			return fmt.Errorf("sqlite: indexing aggregate alias %s:%s: %w", e.Space, e.ExtID, err)
		}
		rowID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("sqlite: reading aggregate alias rowid %s:%s: %w", e.Space, e.ExtID, err)
		}
		if _, err := t.tx.ExecContext(t.ctx,
			`INSERT INTO fts_name (rowid, canonical, vernacular_de) VALUES (?, ?, '')`,
			rowID, domain.Canonicalize(e.Name)); err != nil {
			return fmt.Errorf("sqlite: indexing aggregate alias fts_name %s:%s: %w", e.Space, e.ExtID, err)
		}
	}
	return nil
}

// UpsertClassification records family/order/class for conceptID (see
// schema.sql's taxon_concept.family/order_name/class_name). Empty strings
// are written as SQL NULL via nullString — the same rule AddNameSpaceEntry
// applies to e.Resolution — since a blank column here means "unknown",
// never "empty on purpose".
func (t *ingestTx) UpsertClassification(conceptID string, family, orderName, className string) error {
	_, err := t.tx.ExecContext(t.ctx, `
		UPDATE taxon_concept SET family = ?, order_name = ?, class_name = ?
		WHERE id = ?`,
		nullString(family), nullString(orderName), nullString(className), conceptID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting classification for concept %q: %w", conceptID, err)
	}
	return nil
}

// AddVernacularName writes one vernacular-name row (schema.sql's
// `vernacular` table). INSERT OR REPLACE on the (concept_id, lang, name)
// primary key, mirroring AddNameSpaceEntry's re-ingest-is-idempotent rule.
func (t *ingestTx) AddVernacularName(conceptID string, v domain.VernacularName) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO vernacular (concept_id, lang, name, preferred)
		VALUES (?, ?, ?, 0)`,
		conceptID, v.Language, v.Name,
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding vernacular name %q for concept %q: %w", v.Name, conceptID, err)
	}
	return nil
}

// boolToInt renders a Go bool for SQLite's integer boolean columns.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NameSpaceEntries returns every name-space spelling attached to conceptID,
// ordered by (space, ext_id). spaces restricts which spaces are returned;
// nil/empty means every ingested space. Returns domain.ErrNotFound (wrapped)
// if conceptID does not exist; an existing concept with no entries returns an
// empty, non-nil-error slice — the two are never conflated.
func (db *DB) NameSpaceEntries(ctx context.Context, conceptID string, spaces []string) ([]domain.NameSpaceEntry, error) {
	exists, err := db.conceptExists(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: checking concept %q exists: %w", conceptID, err)
	}
	if !exists {
		return nil, fmt.Errorf("sqlite: concept %q: %w", conceptID, domain.ErrNotFound)
	}

	query, args := nameSpaceEntriesQuery(conceptID, spaces)
	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying name space entries for concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.NameSpaceEntry{}
	for rows.Next() {
		var (
			e          domain.NameSpaceEntry
			aggregate  int
			resolution sql.NullString
		)
		if err := rows.Scan(&e.Space, &e.ExtID, &e.Name, &aggregate, &resolution, &e.Status); err != nil {
			return nil, fmt.Errorf("sqlite: scanning name space entry for concept %q: %w", conceptID, err)
		}
		e.Aggregate = aggregate != 0
		// A NULL resolution is the ordinary exact match and maps back to the
		// empty string, exactly as AddNameSpaceEntry wrote it.
		e.Resolution = resolution.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating name space entries for concept %q: %w", conceptID, err)
	}
	return out, nil
}

// nameSpaceEntriesQuery builds the query+args NameSpaceEntries runs. The
// space filter uses a bounded placeholder list rather than json_each: unlike
// bundle.go's concept-id lists it is caller-bounded (a handful of ingested
// spaces at most), the same trade-off traitsQuery makes for its vocab list.
func nameSpaceEntriesQuery(conceptID string, spaces []string) (string, []any) {
	query := `
		SELECT space, ext_id, name, aggregate, resolution, status
		FROM name_space_entry
		WHERE concept_id = ?`
	args := []any{conceptID}
	if len(spaces) > 0 {
		query += ` AND space IN (` + placeholdersFor(len(spaces)) + `)`
		for _, s := range spaces {
			args = append(args, s)
		}
	}
	query += ` ORDER BY space, ext_id`
	return query, args
}

// NameSpaces lists every ingested name-space provenance row, ordered by id.
func (db *DB) NameSpaces(ctx context.Context) ([]domain.NameSpaceMeta, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, version, COALESCE(license, ''), COALESCE(source_url, ''), manifest_sha, redistribution
		FROM name_space
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying name_space: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.NameSpaceMeta
	for rows.Next() {
		var (
			meta           domain.NameSpaceMeta
			redistribution string
		)
		if err := rows.Scan(&meta.ID, &meta.Version, &meta.License, &meta.SourceURL, &meta.ManifestSHA, &redistribution); err != nil {
			return nil, fmt.Errorf("sqlite: scanning name_space row: %w", err)
		}
		meta.Redistribution = domain.Redistribution(redistribution)
		out = append(out, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating name_space rows: %w", err)
	}
	return out, nil
}
