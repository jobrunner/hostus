package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
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
// must not be folded to the canonical match key. e.Resolution follows the
// "absence is information" rule: an empty resolution (an exact canonical
// match) is stored as NULL, not as ”.
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

// AddAggregateMember records one aggregate->member edge (schema.sql's
// concept_aggregate table). INSERT OR REPLACE on the (aggregate_concept_id,
// member_concept_id) primary key, mirroring AddNameSpaceEntry's
// re-ingest-is-idempotent rule.
func (t *ingestTx) AddAggregateMember(aggregateConceptID, memberConceptID string) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO concept_aggregate (aggregate_concept_id, member_concept_id)
		VALUES (?, ?)`,
		aggregateConceptID, memberConceptID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: linking aggregate member %q -> %q: %w", aggregateConceptID, memberConceptID, err)
	}
	return nil
}

// ResolveNameSpaceMember reads name_space_entry for (space, extID) within
// THIS transaction and returns its concept_id, or "" (no error) if no such
// entry exists — a Fall-A crosswalk (Task 4) may simply not have resolved
// that row.
func (t *ingestTx) ResolveNameSpaceMember(space, extID string) (string, error) {
	var conceptID string
	err := t.tx.QueryRowContext(t.ctx, `
		SELECT concept_id FROM name_space_entry WHERE space = ? AND ext_id = ?`,
		space, extID,
	).Scan(&conceptID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: resolving name space member %s:%s: %w", space, extID, err)
	}
	return conceptID, nil
}

// AggregateMembers returns the WCVP concept ids aggregateConceptID includes,
// via concept_aggregate. An aggregate with no linked members returns an
// empty, non-error slice.
func (db *DB) AggregateMembers(ctx context.Context, aggregateConceptID string) ([]string, error) {
	return db.queryConceptAggregateIDs(ctx,
		`SELECT member_concept_id FROM concept_aggregate WHERE aggregate_concept_id = ?`,
		aggregateConceptID)
}

// AggregatesByMember returns every Fall-B aggregate concept id that lists
// memberConceptID among its concept_aggregate members, via
// idx_concept_aggregate_member. A member linked into no aggregate returns
// an empty, non-error slice.
func (db *DB) AggregatesByMember(ctx context.Context, memberConceptID string) ([]string, error) {
	return db.queryConceptAggregateIDs(ctx,
		`SELECT aggregate_concept_id FROM concept_aggregate WHERE member_concept_id = ?`,
		memberConceptID)
}

// queryConceptAggregateIDs is the shared scan loop AggregateMembers/
// AggregatesByMember both run: a single-column, single-placeholder query
// against concept_aggregate, returning the matched column's values (empty,
// non-nil slice for no match). query is always one of the two literal
// SELECTs above — never built from caller input — so passing it straight to
// QueryContext alongside whereVal keeps both parameterized.
func (db *DB) queryConceptAggregateIDs(ctx context.Context, query, whereVal string) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, query, whereVal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying concept_aggregate for %q: %w", whereVal, err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("sqlite: scanning concept_aggregate row for %q: %w", whereVal, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating concept_aggregate rows for %q: %w", whereVal, err)
	}
	return out, nil
}

// VernacularNames returns every vernacular-name row for conceptID, ordered
// by (lang, name). A concept with no vernacular name returns an empty,
// non-error slice.
func (db *DB) VernacularNames(ctx context.Context, conceptID string) ([]domain.VernacularName, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT lang, name FROM vernacular WHERE concept_id = ? ORDER BY lang, name`,
		conceptID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying vernacular names for concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.VernacularName{}
	for rows.Next() {
		var v domain.VernacularName
		if err := rows.Scan(&v.Language, &v.Name); err != nil {
			return nil, fmt.Errorf("sqlite: scanning vernacular name for concept %q: %w", conceptID, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating vernacular names for concept %q: %w", conceptID, err)
	}
	return out, nil
}

// AggregateConcepts returns every taxon_concept in backboneID whose rank is
// one of ranks — the native Fall-B aggregate/collective-species concepts
// (Task 5/6) application.ComputeConceptAgreement pairs up across name
// spaces. tc.accepted_name references name.id directly (no junction table
// needed here, unlike concept_name, which serves synonym roles).
func (db *DB) AggregateConcepts(ctx context.Context, backboneID string, ranks []domain.Rank) ([]output.AggregateConceptSummary, error) {
	if len(ranks) == 0 {
		return []output.AggregateConceptSummary{}, nil
	}
	query, args := aggregateConceptsQuery(backboneID, ranks)
	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying aggregate concepts for backbone %q: %w", backboneID, err)
	}
	defer func() { _ = rows.Close() }()

	out := []output.AggregateConceptSummary{}
	for rows.Next() {
		var s output.AggregateConceptSummary
		if err := rows.Scan(&s.ConceptID, &s.Canonical); err != nil {
			return nil, fmt.Errorf("sqlite: scanning aggregate concept for backbone %q: %w", backboneID, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating aggregate concepts for backbone %q: %w", backboneID, err)
	}
	return out, nil
}

// aggregateConceptsQuery builds the query+args AggregateConcepts runs,
// mirroring nameSpaceEntriesQuery/traitsQuery's split-out-query-builder
// style in this package: len(ranks) is caller-bounded (the two Fall-B
// aggregate ranks, see aggregateRanks), so a placeholder list is the right
// trade-off here too.
func aggregateConceptsQuery(backboneID string, ranks []domain.Rank) (string, []any) {
	args := make([]any, 0, len(ranks)+1)
	args = append(args, backboneID)
	for _, r := range ranks {
		args = append(args, string(r))
	}
	query := `
		SELECT tc.id, n.canonical FROM taxon_concept tc
		JOIN name n ON n.id = tc.accepted_name
		WHERE tc.backbone_id = ? AND tc.rank IN (` + placeholdersFor(len(ranks)) + `)
		ORDER BY tc.id`
	return query, args
}

// WriteConceptAgreement (re)writes concept_agreement for every given pair,
// one INSERT OR REPLACE per pair. Deliberately not part of IngestTx — see
// output.Repository.WriteConceptAgreement's doc comment.
func (db *DB) WriteConceptAgreement(ctx context.Context, pairs []domain.ConceptAgreementPair) error {
	stmt, err := db.sql.PrepareContext(ctx, `
		INSERT OR REPLACE INTO concept_agreement
			(eurosl_concept_id, germansl_concept_id, agreement, agreement_text, only_in_eurosl, only_in_germansl)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("sqlite: preparing concept_agreement insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, p := range pairs {
		if _, err := stmt.ExecContext(ctx,
			nullString(p.EuroslConceptID), nullString(p.GermanslConceptID),
			string(p.Agreement), p.AgreementText,
			strings.Join(p.OnlyInEurosl, ","), strings.Join(p.OnlyInGermansl, ","),
		); err != nil {
			return fmt.Errorf("sqlite: writing concept_agreement for %q/%q: %w", p.EuroslConceptID, p.GermanslConceptID, err)
		}
	}
	return nil
}

// ConceptAgreement returns the precomputed concept_agreement row involving
// conceptID on either side, or (nil, nil) if none exists — see
// output.Repository.ConceptAgreement's doc comment. eurosl_concept_id/
// germansl_concept_id are NULLable (a one-sided pair), and
// only_in_eurosl/only_in_germansl are comma-joined WCVP concept id lists
// (see WriteConceptAgreement) split back into slices here — an empty string
// splits to an empty, non-nil slice, never [""].
func (db *DB) ConceptAgreement(ctx context.Context, conceptID string) (*domain.ConceptAgreementPair, error) {
	var (
		eurosl, germansl         sql.NullString
		agreement, agreementText string
		onlyEurosl, onlyGermansl string
	)
	err := db.sql.QueryRowContext(ctx, `
		SELECT eurosl_concept_id, germansl_concept_id, agreement, agreement_text, only_in_eurosl, only_in_germansl
		FROM concept_agreement
		WHERE eurosl_concept_id = ? OR germansl_concept_id = ?`,
		conceptID, conceptID,
	).Scan(&eurosl, &germansl, &agreement, &agreementText, &onlyEurosl, &onlyGermansl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading concept_agreement for %q: %w", conceptID, err)
	}
	return &domain.ConceptAgreementPair{
		EuroslConceptID:   eurosl.String,
		GermanslConceptID: germansl.String,
		Agreement:         domain.Agreement(agreement),
		AgreementText:     agreementText,
		OnlyInEurosl:      splitCommaList(onlyEurosl),
		OnlyInGermansl:    splitCommaList(onlyGermansl),
	}, nil
}

// splitCommaList splits a comma-joined list (see WriteConceptAgreement) back
// into a slice, mapping "" to an empty, non-nil slice rather than [""].
func splitCommaList(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
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
