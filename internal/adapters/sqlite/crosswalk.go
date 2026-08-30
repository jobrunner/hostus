package sqlite

import (
	"context"
	"fmt"
)

// CrosswalkEntry is one name->concept_id row for hostus export-crosswalk's
// eurosl_crosswalk.csv (spec docs/superpowers/specs/2026-08-29-eurosl-
// crosswalk-export-design.md §2) — either Fall A (an eurosl name_space_entry
// spelling of an existing WCVP concept) or Fall B (a native eurosl
// concept's own accepted name).
type CrosswalkEntry struct {
	Name      string
	ConceptID string
}

// EuroslCrosswalkEntries returns Fall A of the eurosl crosswalk: every
// name_space_entry row pinned to space "eurosl", ordered by (name,
// concept_id) for a deterministic export. An eurosl ingest that resolved
// no rows returns an empty, non-error slice.
func (db *DB) EuroslCrosswalkEntries(ctx context.Context) ([]CrosswalkEntry, error) {
	return db.queryCrosswalkEntries(ctx,
		`SELECT name, concept_id FROM name_space_entry WHERE space = 'eurosl' ORDER BY name, concept_id`)
}

// NativeEuroslConcepts returns Fall B of the eurosl crosswalk: every
// taxon_concept native to the eurosl backbone (aggregates, sections,
// families, ...; Task 5/6 of the namensraum redesign), keyed by its OWN
// accepted name, ordered by (name, concept_id) for a deterministic export.
// A database with no eurosl-native ingest returns an empty, non-error
// slice.
func (db *DB) NativeEuroslConcepts(ctx context.Context) ([]CrosswalkEntry, error) {
	return db.queryCrosswalkEntries(ctx, `
		SELECT n.canonical, tc.id FROM taxon_concept tc
		JOIN name n ON n.id = tc.accepted_name
		WHERE tc.backbone_id = 'eurosl'
		ORDER BY n.canonical, tc.id`)
}

// queryCrosswalkEntries is the shared scan loop EuroslCrosswalkEntries/
// NativeEuroslConcepts both run: a fixed two-column query with no
// caller-supplied arguments (both callers pass a literal SELECT), scanned
// into CrosswalkEntry.
func (db *DB) queryCrosswalkEntries(ctx context.Context, query string) ([]CrosswalkEntry, error) {
	rows, err := db.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying eurosl crosswalk entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []CrosswalkEntry{}
	for rows.Next() {
		var e CrosswalkEntry
		if err := rows.Scan(&e.Name, &e.ConceptID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning eurosl crosswalk entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating eurosl crosswalk entries: %w", err)
	}
	return out, nil
}

// AggregateMemberRow is one concept_aggregate edge joined to the member's
// accepted canonical name, for hostus export-crosswalk's
// aggregate_members.csv.
type AggregateMemberRow struct {
	AggregateConceptID string
	MemberConceptID    string
	MemberName         string
}

// AllAggregateMembers returns every concept_aggregate edge across every
// native name space (not eurosl-only — germansl aggregates are included
// too, matching /v1/concept/{id}'s own aggregateMembers join in
// internal/adapters/http/taxa.go), joined to the member's accepted
// canonical name, ordered by (aggregate_concept_id, member_concept_id) for
// a deterministic export. An empty concept_aggregate table (no Fall-B
// ingest ran) returns an empty, non-error slice — never an error.
func (db *DB) AllAggregateMembers(ctx context.Context) ([]AggregateMemberRow, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT ca.aggregate_concept_id, ca.member_concept_id, n.canonical
		FROM concept_aggregate ca
		JOIN taxon_concept tc ON tc.id = ca.member_concept_id
		JOIN name n ON n.id = tc.accepted_name
		ORDER BY ca.aggregate_concept_id, ca.member_concept_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying concept_aggregate members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []AggregateMemberRow{}
	for rows.Next() {
		var r AggregateMemberRow
		if err := rows.Scan(&r.AggregateConceptID, &r.MemberConceptID, &r.MemberName); err != nil {
			return nil, fmt.Errorf("sqlite: scanning concept_aggregate member row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating concept_aggregate member rows: %w", err)
	}
	return out, nil
}
