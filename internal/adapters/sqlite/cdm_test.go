package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// cdmBackbone is the CDM rl_standardliste harvest as a second backbone:
// redistribution "unknown", because no license is findable anywhere on the
// portal, the API or the payloads (pipelines/cdm/README.md).
func cdmBackbone() domain.BackboneVersion {
	return domain.BackboneVersion{
		ID:             "cdm",
		Version:        "2026-08-02",
		SourceURL:      "https://api.cybertaxonomy.org/rl_standardliste",
		IngestedAt:     "2026-08-02T00:00:00Z",
		ManifestSHA:    "cafebabe",
		Redistribution: domain.RedistributionUnknown,
	}
}

// twoSecConcepts is the SP5 shape: one name, two sec. reference spaces, two
// deliberately distinct concept rows, and one relation between them.
func twoSecConcepts() []application.CDMConceptRow {
	return []application.CDMConceptRow{
		{
			ConceptUUID: "aaa", ScientificName: "Abies alba", Authorship: "Mill.",
			Rank: "Species", Status: "Accepted",
			SecUUID: "sec-wh98", SecTitle: "Wisskirchen & Haeupler 1998: Standardliste",
		},
		{
			ConceptUUID: "bbb", ScientificName: "Abies alba", Authorship: "Mill.",
			Rank: "Species", Status: "Accepted",
			SecUUID: "sec-hegi", SecTitle: "HEGI: Illustrierte Flora von Mitteleuropa",
		},
	}
}

func ingestCDMFixture(t *testing.T, db *sqlite.DB, relations []application.CDMRelationRow) application.CDMIngestReport {
	t.Helper()
	rep, err := application.IngestCDM(context.Background(), db, twoSecConcepts(), relations, cdmBackbone())
	if err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}
	return rep
}

func openTempDB(t *testing.T) (*sqlite.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hostus.sqlite")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func TestCDMConceptWithSecRoundTrips(t *testing.T) {
	db, _ := openTempDB(t)
	ingestCDMFixture(t, db, nil)

	ctx := context.Background()
	c, syns, xrefs, dists, err := db.Concept(ctx, "cdm:concept:aaa")
	if err != nil {
		t.Fatalf("Concept: unexpected error: %v", err)
	}
	if c.SecReference != "sec-wh98" {
		t.Errorf("sec_reference = %q, want sec-wh98", c.SecReference)
	}
	if c.BackboneID != "cdm" {
		t.Errorf("backbone = %q, want cdm", c.BackboneID)
	}
	if c.AcceptedName.Canonical != "Abies alba" {
		t.Errorf("canonical = %q", c.AcceptedName.Canonical)
	}
	// A CDM concept carries its accepted name and its sec., nothing else:
	// the harvest has no synonym, xref or distribution data.
	if len(syns) != 0 || len(xrefs) != 0 || len(dists) != 0 {
		t.Errorf("unexpected synonyms/xrefs/distributions: %v / %v / %v", syns, xrefs, dists)
	}

	secs, err := db.SecReferences(ctx)
	if err != nil {
		t.Fatalf("SecReferences: unexpected error: %v", err)
	}
	if len(secs) != 2 {
		t.Fatalf("got %d sec references, want 2", len(secs))
	}
	if secs[0].ID != "sec-hegi" || !strings.HasPrefix(secs[0].Title, "HEGI") {
		t.Errorf("sec references not ordered/titled as expected: %+v", secs)
	}
}

func TestCDMSameNameDifferentSecStaysTwoRows(t *testing.T) {
	db, path := openTempDB(t)
	ingestCDMFixture(t, db, nil)

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = raw.Close() }()

	var n int
	if err := raw.QueryRow(`
		SELECT COUNT(*) FROM taxon_concept tc
		JOIN name n ON n.id = tc.accepted_name
		WHERE n.canonical = 'Abies alba'`).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 2 {
		t.Fatalf("got %d concepts for one name, want 2 distinct sec. spaces", n)
	}
}

func TestCDMRelationIsWrittenWithTheMappedType(t *testing.T) {
	db, path := openTempDB(t)
	rep := ingestCDMFixture(t, db, []application.CDMRelationRow{
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Includes", IsConceptRelation: boolTrue()},
	})
	if rep.RelationsWritten != 1 {
		t.Fatalf("RelationsWritten = %d, want 1", rep.RelationsWritten)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = raw.Close() }()

	var from, to, rel, source string
	if err := raw.QueryRow(`SELECT from_concept, to_concept, relation, source FROM concept_relation`).Scan(&from, &to, &rel, &source); err != nil {
		t.Fatalf("reading concept_relation: %v", err)
	}
	if from != "cdm:concept:bbb" || to != "cdm:concept:aaa" {
		t.Errorf("ends = %q -> %q", from, to)
	}
	if rel != string(domain.RelationIncludes) || source != "cdm" {
		t.Errorf("relation/source = %q/%q", rel, source)
	}

	// Directionality: exactly one row, no synthesized inverse.
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM concept_relation`).Scan(&n); err != nil {
		t.Fatalf("counting relations: %v", err)
	}
	if n != 1 {
		t.Errorf("got %d relation rows, want 1 (the source direction only)", n)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM concept_relation WHERE relation = ?`, string(domain.RelationIncludedIn)).Scan(&n); err != nil {
		t.Fatalf("counting inverse: %v", err)
	}
	if n != 0 {
		t.Errorf("got %d included_in rows, want 0 (inversion is a query-time concern)", n)
	}
}

func TestCDMTwoRelationTypesBetweenTheSamePairBothSurvive(t *testing.T) {
	// The widened primary key (from, to, relation, source). Under SP1's key
	// the second row would have silently replaced the first.
	db, path := openTempDB(t)
	rep := ingestCDMFixture(t, db, []application.CDMRelationRow{
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolTrue()},
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Overlaps", IsConceptRelation: boolTrue()},
	})
	if rep.RelationsWritten != 2 {
		t.Fatalf("RelationsWritten = %d, want 2", rep.RelationsWritten)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = raw.Close() }()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM concept_relation`).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 2 {
		t.Fatalf("got %d relation rows, want 2 — the PK must include `relation`", n)
	}
}

func TestCDMRelationRejectsADanglingEnd(t *testing.T) {
	// The FK is what makes application.IngestCDM's two-phase resolution
	// necessary rather than merely tidy.
	db, _ := openTempDB(t)
	ingestCDMFixture(t, db, nil)

	tx, err := db.BeginIngest(context.Background(), cdmBackbone())
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.AddConceptRelation("cdm:concept:aaa", "cdm:concept:ghost", domain.RelationCongruent, "cdm"); err == nil {
		t.Fatal("want an FK error for a relation whose partner concept does not exist")
	}
}

func TestExistingConceptIDs(t *testing.T) {
	db, _ := openTempDB(t)
	ingestCDMFixture(t, db, nil)

	got, err := db.ExistingConceptIDs(context.Background(), []string{"cdm:concept:aaa", "cdm:concept:ghost"})
	if err != nil {
		t.Fatalf("ExistingConceptIDs: %v", err)
	}
	if !got["cdm:concept:aaa"] {
		t.Error("an existing concept must be reported present")
	}
	if got["cdm:concept:ghost"] {
		t.Error("a missing concept must be absent from the result")
	}

	empty, err := db.ExistingConceptIDs(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty input: got %v / %v", empty, err)
	}
}

func TestSecReferencesEmptyDatabase(t *testing.T) {
	db, _ := openTempDB(t)
	secs, err := db.SecReferences(context.Background())
	if err != nil {
		t.Fatalf("SecReferences: %v", err)
	}
	if len(secs) != 0 {
		t.Errorf("got %d, want 0", len(secs))
	}
}

func TestOpenMigratesLegacyConceptRelationPrimaryKey(t *testing.T) {
	// A database created before SP5 carries concept_relation with the
	// three-column key. Open must widen it in place and keep the database
	// openable — the schema.sql CREATE TABLE alone cannot, since it is
	// IF NOT EXISTS.
	db, path := openTempDB(t)
	ingestCDMFixture(t, db, nil)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Roll concept_relation back to its pre-SP5 shape, with a row in it.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec(`
		DROP TABLE concept_relation;
		CREATE TABLE concept_relation (
			from_concept TEXT NOT NULL REFERENCES taxon_concept(id),
			to_concept   TEXT NOT NULL REFERENCES taxon_concept(id),
			relation     TEXT NOT NULL,
			source       TEXT,
			PRIMARY KEY (from_concept, to_concept, source)
		);
		INSERT INTO concept_relation VALUES ('cdm:concept:aaa','cdm:concept:bbb','congruent','legacy');`); err != nil {
		t.Fatalf("seeding legacy database: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing seed handle: %v", err)
	}

	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open on a legacy database must succeed: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = check.Close() }()

	var pk int
	if err := check.QueryRow(`SELECT pk FROM pragma_table_info('concept_relation') WHERE name = 'relation'`).Scan(&pk); err != nil {
		t.Fatalf("inspecting migrated key: %v", err)
	}
	if pk == 0 {
		t.Error("`relation` is still not part of the primary key")
	}
	// The pre-existing row must survive the rebuild.
	var n int
	if err := check.QueryRow(`SELECT COUNT(*) FROM concept_relation WHERE source = 'legacy'`).Scan(&n); err != nil {
		t.Fatalf("counting legacy rows: %v", err)
	}
	if n != 1 {
		t.Errorf("got %d legacy relation rows after migration, want 1", n)
	}
}

func TestOpenMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostus.sqlite")
	for i := range 2 {
		db, err := sqlite.Open(path)
		if err != nil {
			t.Fatalf("Open #%d: %v", i, err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
}

// --- the redistribution gate -----------------------------------------------
//
// SP4's review found the gate had a hole nobody noticed because each task was
// locally correct. It is therefore verified here for CDM specifically rather
// than assumed to generalise from the backbone case.

func TestExportBundleRefusesCDMDataByDefault(t *testing.T) {
	db, _ := openTempDB(t)
	ingestCDMFixture(t, db, []application.CDMRelationRow{
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolTrue()},
	})

	out := filepath.Join(t.TempDir(), "bundle.sqlite")
	_, err := sqlite.ExportBundle(context.Background(), db, out, sqlite.BundleOpts{
		SnapshotVersion: "v1", Now: func() time.Time { return fixedBundleClock },
	})
	if err == nil {
		t.Fatal("a bundle containing CDM data must be refused by default")
	}
	if !strings.Contains(err.Error(), "cdm") || !strings.Contains(err.Error(), "redistribution=unknown") {
		t.Errorf("refusal %q must name the offending source and its redistribution value", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a refused export must not leave a bundle behind")
	}
}

func TestExportBundleForceIncludeRestrictedRecordsCDM(t *testing.T) {
	db, _ := openTempDB(t)
	ingestCDMFixture(t, db, []application.CDMRelationRow{
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolTrue()},
	})

	out := filepath.Join(t.TempDir(), "bundle.sqlite")
	rep, err := sqlite.ExportBundle(context.Background(), db, out, sqlite.BundleOpts{
		SnapshotVersion: "v1", AllowRestricted: true, Now: func() time.Time { return fixedBundleClock },
	})
	if err != nil {
		t.Fatalf("forced export: %v", err)
	}
	if rep.Concepts != 2 {
		t.Errorf("Concepts = %d, want 2", rep.Concepts)
	}
	if got := readBundleMeta(t, out).RestrictedSources; got != "cdm" {
		t.Errorf("restricted_sources = %q, want cdm", got)
	}

	// The bundle must carry the sec. titles and the relation, or the copied
	// concepts assert nothing.
	bundle, err := sqlite.Open(out)
	if err != nil {
		t.Fatalf("opening bundle: %v", err)
	}
	defer func() { _ = bundle.Close() }()
	secs, err := bundle.SecReferences(context.Background())
	if err != nil || len(secs) != 2 {
		t.Errorf("bundle sec references: %+v / %v", secs, err)
	}

	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = raw.Close() }()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM concept_relation`).Scan(&n); err != nil {
		t.Fatalf("counting bundled relations: %v", err)
	}
	if n != 1 {
		t.Errorf("bundle carries %d relations, want 1", n)
	}
}

func boolTrue() *bool { b := true; return &b }
