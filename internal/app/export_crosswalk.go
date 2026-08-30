package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
)

// ExportCrosswalkReport summarizes one "hostus export-crosswalk" run: how
// many rows each output CSV received, and every name collision found
// between the eurosl crosswalk's two sources.
type ExportCrosswalkReport struct {
	CrosswalkRows  int
	MemberRows     int
	NameCollisions []CrosswalkCollision
}

// CrosswalkCollision names one eurosl name that resolves to two different
// concept ids depending on source: Fall A's name_space_entry crosswalk vs.
// Fall B's own native eurosl concept. Reported, never silently resolved to
// one side (spec's "Prüfbare Zusagen") — both rows still land in
// eurosl_crosswalk.csv, and situs' own ingest decides what to do with an
// ambiguous name.
type CrosswalkCollision struct {
	Name           string
	FallAConceptID string
	FallBConceptID string
}

// ExportCrosswalk writes eurosl_crosswalk.csv and aggregate_members.csv
// into outDir (created if missing) from dbPath's ingested database (spec
// docs/superpowers/specs/2026-08-29-eurosl-crosswalk-export-design.md).
// Unlike Bundle, there is no redistribution gate: this is a local pipeline
// handoff between two services run by the same operator, not a
// distribution to a third party (spec, owner decision 2026-08-29).
func ExportCrosswalk(ctx context.Context, dbPath, outDir string) (ExportCrosswalkReport, error) {
	src, err := sqlite.Open(dbPath)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: creating output directory %q: %w", outDir, err)
	}

	fallA, err := src.EuroslCrosswalkEntries(ctx)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: reading eurosl crosswalk Fall A: %w", err)
	}
	fallB, err := src.NativeEuroslConcepts(ctx)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: reading eurosl crosswalk Fall B: %w", err)
	}
	collisions := detectCrosswalkCollisions(fallA, fallB)

	crosswalkPath := filepath.Join(outDir, "eurosl_crosswalk.csv")
	if err := writeCrosswalkCSV(crosswalkPath, fallA, fallB); err != nil {
		return ExportCrosswalkReport{}, err
	}

	members, err := src.AllAggregateMembers(ctx)
	if err != nil {
		return ExportCrosswalkReport{}, fmt.Errorf("app: reading aggregate members: %w", err)
	}
	membersPath := filepath.Join(outDir, "aggregate_members.csv")
	if err := writeAggregateMembersCSV(membersPath, members); err != nil {
		return ExportCrosswalkReport{}, err
	}

	return ExportCrosswalkReport{
		CrosswalkRows:  len(fallA) + len(fallB),
		MemberRows:     len(members),
		NameCollisions: collisions,
	}, nil
}

// detectCrosswalkCollisions finds every name present in BOTH fallA and
// fallB — see CrosswalkCollision's doc comment. Never resolved
// automatically: every match is reported, and both sides' rows still get
// written to eurosl_crosswalk.csv by writeCrosswalkCSV.
func detectCrosswalkCollisions(fallA, fallB []sqlite.CrosswalkEntry) []CrosswalkCollision {
	byName := make(map[string]string, len(fallA))
	for _, e := range fallA {
		byName[e.Name] = e.ConceptID
	}
	var collisions []CrosswalkCollision
	for _, e := range fallB {
		if aID, ok := byName[e.Name]; ok {
			collisions = append(collisions, CrosswalkCollision{
				Name: e.Name, FallAConceptID: aID, FallBConceptID: e.ConceptID,
			})
		}
	}
	return collisions
}

// writeCrosswalkCSV writes eurosl_crosswalk.csv: a header row, then every
// Fall A row, then every Fall B row — a plain concatenation (spec's
// UNION), never a merge that would hide a collision.
func writeCrosswalkCSV(path string, fallA, fallB []sqlite.CrosswalkEntry) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("app: creating %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"name", "concept_id"}); err != nil {
		return fmt.Errorf("app: writing %q header: %w", path, err)
	}
	for _, e := range fallA {
		if err := w.Write([]string{e.Name, e.ConceptID}); err != nil {
			return fmt.Errorf("app: writing %q row: %w", path, err)
		}
	}
	for _, e := range fallB {
		if err := w.Write([]string{e.Name, e.ConceptID}); err != nil {
			return fmt.Errorf("app: writing %q row: %w", path, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("app: flushing %q: %w", path, err)
	}
	return nil
}

// writeAggregateMembersCSV writes aggregate_members.csv: a header row,
// then every concept_aggregate row. An empty members slice (no Fall-B
// aggregate ingest ran) writes only the header — not an error (spec's
// error table).
func writeAggregateMembersCSV(path string, members []sqlite.AggregateMemberRow) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("app: creating %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"aggregate_concept_id", "member_concept_id", "member_name"}); err != nil {
		return fmt.Errorf("app: writing %q header: %w", path, err)
	}
	for _, m := range members {
		if err := w.Write([]string{m.AggregateConceptID, m.MemberConceptID, m.MemberName}); err != nil {
			return fmt.Errorf("app: writing %q row: %w", path, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("app: flushing %q: %w", path, err)
	}
	return nil
}
