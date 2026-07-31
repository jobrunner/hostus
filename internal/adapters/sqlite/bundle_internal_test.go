package sqlite

import (
	"context"
	"path/filepath"
	"testing"
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

	if _, err := populateBundle(ctx, nil, bundle, nil, BundleOpts{}); err == nil {
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

	if _, err := populateBundle(ctx, src, bundle, []string{conceptID}, BundleOpts{}); err == nil {
		t.Fatal("populateBundle(with concepts): want an error when bundle_meta is missing, got nil")
	}
}
