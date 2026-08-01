package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// openBundleWithoutMeta opens a fresh bundle DB (schema applied, including
// bundle_meta) and immediately drops bundle_meta, so populateBundle's
// insertBundleMeta call fails with a genuine SQL error ("no such table") —
// letting tests prove ExportBundle/populateBundle actually SURFACE that
// error rather than silently swallowing it.
func openBundleWithoutMeta(t *testing.T) *DB {
	t.Helper()
	bundle := openTestDBAt(t, filepath.Join(t.TempDir(), "bundle.sqlite"))
	if _, err := bundle.sql.Exec(`DROP TABLE bundle_meta`); err != nil {
		t.Fatalf("dropping bundle_meta: %v", err)
	}
	return bundle
}

// openTestDBAt is openTestDB (db_internal_test.go) for a real file path
// instead of ":memory:", needed here because ExportBundle's own Open(out)
// call always creates a fresh file — these tests instead pre-build the
// bundle DB by hand so they can sabotage it before calling populateBundle.
func openTestDBAt(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): unexpected error: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestPopulateBundle_EmptyConceptIDs_MetaInsertErrorPropagates proves the
// empty-scope branch of populateBundle (no concepts in area) does not
// swallow a genuine bundle_meta insert failure: src is never touched on
// this path (conceptIDs is empty), so nil is a safe stand-in.
func TestPopulateBundle_EmptyConceptIDs_MetaInsertErrorPropagates(t *testing.T) {
	ctx := context.Background()
	bundle := openBundleWithoutMeta(t)

	if _, err := populateBundle(ctx, nil, bundle, nil, nil, BundleOpts{}, ""); err == nil {
		t.Fatal("populateBundle(no concepts): want an error when bundle_meta is missing, got nil")
	}
}

// TestPopulateBundle_NonEmptyConceptIDs_MetaInsertErrorPropagates is the
// non-empty-scope counterpart: every copy step (backbone_version, name,
// taxon_concept, concept_name, xref, distribution, vernacular, FTS
// rebuild) must still succeed against a real seeded concept, so the only
// possible failure is the final bundle_meta insert — proving that failure
// isn't swallowed either.
func TestPopulateBundle_NonEmptyConceptIDs_MetaInsertErrorPropagates(t *testing.T) {
	ctx := context.Background()
	src := openTestDB(t)
	conceptID := seedCorynephorusConcept(t, src)
	bundle := openBundleWithoutMeta(t)

	if _, err := populateBundle(ctx, src, bundle, []string{conceptID}, nil, BundleOpts{}, ""); err == nil {
		t.Fatal("populateBundle(with concepts): want an error when bundle_meta is missing, got nil")
	}
}

// TestCopyConceptScopedTables_TraitVocabularyInsertErrorPropagates proves
// copyConceptScopedTables' trait_vocabulary copy step does not swallow a
// genuine SQL error: src carries one trait_vocabulary row (so the copy
// actually attempts an INSERT), but bundle is missing the table, so that
// INSERT must fail and populateBundle must surface it — every earlier copy
// step (backbone_version, name, taxon_concept, concept_name, xref,
// distribution, vernacular, trait_value) still succeeds against the real
// seeded concept, isolating the failure to this one step.
func TestCopyConceptScopedTables_TraitVocabularyInsertErrorPropagates(t *testing.T) {
	ctx := context.Background()
	src := openTestDB(t)
	conceptID := seedCorynephorusConcept(t, src)

	if _, err := src.sql.ExecContext(ctx, `
		INSERT INTO trait_vocabulary (vocab, version, taxonomy, license, source_url, ingested_at)
		VALUES ('eive', '1.0', 'euromed-aligned', '', '', '2026-07-31T00:00:00Z')`); err != nil {
		t.Fatalf("seeding src trait_vocabulary: %v", err)
	}

	bundle := openTestDBAt(t, filepath.Join(t.TempDir(), "bundle.sqlite"))
	if _, err := bundle.sql.Exec(`DROP TABLE trait_vocabulary`); err != nil {
		t.Fatalf("dropping bundle trait_vocabulary: %v", err)
	}

	if _, err := populateBundle(ctx, src, bundle, []string{conceptID}, nil, BundleOpts{}, ""); err == nil {
		t.Fatal("populateBundle: want an error when bundle's trait_vocabulary table is missing, got nil")
	}
}

// TestTraits_JoinsAgainstMissingVocabularyMetadataAsEmptyTaxonomy proves
// Traits' LEFT JOIN against trait_vocabulary degrades gracefully (empty
// Taxonomy, not an error) when a trait_value row's (vocab, vocab_version)
// has no corresponding trait_vocabulary metadata row — the two are written
// by separate IngestTx calls (AddTraitValue vs. UpsertTraitVocabulary), so
// nothing enforces they always arrive together.
func TestTraits_JoinsAgainstMissingVocabularyMetadataAsEmptyTaxonomy(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	conceptID := seedCorynephorusConcept(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.AddTraitValue(conceptID, domain.TraitValue{Vocab: domain.VocabEIVE, VocabVersion: "1.0", Dim: domain.DimM, Value: 5.5}); err != nil {
		t.Fatalf("AddTraitValue: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	got, err := db.Traits(ctx, conceptID, nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Taxonomy != "" {
		t.Fatalf("Traits() = %+v, want one set with Taxonomy=\"\" (no trait_vocabulary metadata row)", got)
	}
}
