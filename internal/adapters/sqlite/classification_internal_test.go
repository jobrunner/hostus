package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// mustExecClassification runs a raw statement against db.sql, failing the
// test on error. It lives in this file (package sqlite, not sqlite_test)
// specifically so Classification's tests can hand-build parent_id chains —
// including a deliberately cyclic one no real ingest could ever produce —
// without going through application.Ingest.
func mustExecClassification(t *testing.T, db *DB, stmt string) {
	t.Helper()
	if _, err := db.sql.Exec(stmt); err != nil {
		t.Fatalf("exec %q: unexpected error: %v", stmt, err)
	}
}

// seedClassificationChain builds a fresh FAMILY -> GENUS -> SPECIES
// taxon_concept chain (c-family <- c-genus <- c-species, via parent_id),
// entirely by hand, so Classification's tests don't depend on
// application.Ingest or the WCVP fixture.
func seedClassificationChain(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	mustExecClassification(t, db, `INSERT INTO backbone_version (id, version, ingested_at, manifest_sha) VALUES ('wcvp', 'v1', '2026-07-31T00:00:00Z', 'x')`)
	mustExecClassification(t, db, `INSERT INTO name (id, canonical, canonical_fold, rank) VALUES ('n-family', 'Poaceae', 'poaceae', 'FAMILY')`)
	mustExecClassification(t, db, `INSERT INTO name (id, canonical, canonical_fold, rank) VALUES ('n-genus', 'Corynephorus', 'corynephorus', 'GENUS')`)
	mustExecClassification(t, db, `INSERT INTO name (id, canonical, canonical_fold, rank) VALUES ('n-species', 'Corynephorus canescens', 'corynephorus canescens', 'SPECIES')`)
	mustExecClassification(t, db, `INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, status) VALUES ('c-family', 'wcvp', 'n-family', 'FAMILY', NULL, 'ACCEPTED')`)
	mustExecClassification(t, db, `INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, status) VALUES ('c-genus', 'wcvp', 'n-genus', 'GENUS', 'c-family', 'ACCEPTED')`)
	mustExecClassification(t, db, `INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, status) VALUES ('c-species', 'wcvp', 'n-species', 'SPECIES', 'c-genus', 'ACCEPTED')`)
	return db
}

// TestClassification_ReturnsRootFirstAncestorChain proves Classification
// walks parent_id upward and returns the chain ROOT-FIRST: the topmost
// ancestor (FAMILY) first, the immediate parent (GENUS) last, and the
// concept itself (SPECIES) never included.
func TestClassification_ReturnsRootFirstAncestorChain(t *testing.T) {
	db := seedClassificationChain(t)
	ctx := context.Background()

	chain, err := db.Classification(ctx, "c-species")
	if err != nil {
		t.Fatalf("Classification(c-species): unexpected error: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("Classification(c-species) = %+v, want exactly 2 ancestors", chain)
	}
	if chain[0].ConceptID != "c-family" || chain[0].Rank != domain.RankFamily || chain[0].Canonical != "Poaceae" {
		t.Errorf("Classification(c-species)[0] = %+v, want the FAMILY ancestor (root) first", chain[0])
	}
	if chain[1].ConceptID != "c-genus" || chain[1].Rank != domain.RankGenus {
		t.Errorf("Classification(c-species)[1] = %+v, want the GENUS ancestor (immediate parent) last", chain[1])
	}
}

// TestClassification_NoParentReturnsEmptyChain proves a top-level concept
// (no parent_id at all) returns an empty, non-error chain rather than
// failing or fabricating an ancestor.
func TestClassification_NoParentReturnsEmptyChain(t *testing.T) {
	db := seedClassificationChain(t)
	ctx := context.Background()

	chain, err := db.Classification(ctx, "c-family")
	if err != nil {
		t.Fatalf("Classification(c-family): unexpected error: %v", err)
	}
	if len(chain) != 0 {
		t.Errorf("Classification(c-family) = %+v, want empty (no parent_id)", chain)
	}
}

// TestClassification_UnknownConceptReturnsNotFound proves Classification
// surfaces domain.ErrNotFound for an unknown id exactly like Concept does,
// rather than silently returning an empty chain that could be confused with
// "known, top-level concept".
func TestClassification_UnknownConceptReturnsNotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Classification(context.Background(), "does-not-exist"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Classification(does-not-exist) error = %v, want domain.ErrNotFound", err)
	}
}

// TestClassification_BoundedWalkTerminatesOnCycle proves the depth bound
// actually protects against a cyclic/corrupt parent_id chain: no real
// application.Ingest run can ever produce one (parent_id only ever names an
// ACCEPTED taxonID resolved in memory before any write, see
// internal/application/ingest.go), but a corrupted database could — and
// Classification must terminate rather than looping forever. The 2-cycle
// below (c-a's parent is c-b, c-b's parent is c-a) is built via two
// UpsertConcept calls: c-a is first written with no parent (c-b doesn't
// exist yet), then c-b is written with parent_id=c-a (now safe, c-a already
// exists), then c-a is re-written (INSERT OR REPLACE) with parent_id=c-b
// (now also safe) — the exact same "insert without the self-reference,
// then link it in" two-sub-pass technique Ingest itself uses, deliberately
// misused here to produce the otherwise-impossible cycle.
func TestClassification_BoundedWalkTerminatesOnCycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"}
	tx, err := db.BeginIngest(ctx, bv)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	nameA := domain.Name{ID: "n-a", Canonical: "A", Rank: domain.RankGenus}
	nameB := domain.Name{ID: "n-b", Canonical: "B", Rank: domain.RankGenus}
	if err := tx.UpsertName(nameA); err != nil {
		t.Fatalf("UpsertName(a): unexpected error: %v", err)
	}
	if err := tx.UpsertName(nameB); err != nil {
		t.Fatalf("UpsertName(b): unexpected error: %v", err)
	}

	conceptA := domain.Concept{ID: "c-a", BackboneID: "wcvp", AcceptedName: nameA, Rank: domain.RankGenus, Status: domain.StatusAccepted}
	conceptB := domain.Concept{ID: "c-b", BackboneID: "wcvp", AcceptedName: nameB, Rank: domain.RankGenus, Status: domain.StatusAccepted}
	if err := tx.UpsertConcept(conceptA); err != nil {
		t.Fatalf("UpsertConcept(a, no parent): unexpected error: %v", err)
	}
	conceptB.ParentID = "c-a"
	if err := tx.UpsertConcept(conceptB); err != nil {
		t.Fatalf("UpsertConcept(b, parent=a): unexpected error: %v", err)
	}
	conceptA.ParentID = "c-b"
	if err := tx.UpsertConcept(conceptA); err != nil {
		t.Fatalf("UpsertConcept(a, parent=b, closing the cycle): unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	chain, err := db.Classification(ctx, "c-a")
	if err != nil {
		t.Fatalf("Classification(c-a) on a cyclic parent_id chain: unexpected error: %v", err)
	}
	// maxClassificationDepth (see read.go) bounds the walk; a genuine
	// 2-cycle keeps alternating c-b/c-a forever, so hitting exactly the
	// depth bound (not looping forever, and not stopping early) is the
	// proof the bound is what stopped it.
	if len(chain) != maxClassificationDepth {
		t.Fatalf("len(Classification(c-a)) = %d, want exactly %d (the depth bound stopping a genuine cycle)", len(chain), maxClassificationDepth)
	}
}
