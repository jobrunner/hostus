package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// fixedBundleClock is injected via sqlite.BundleOpts.Now so bundle_meta's
// created_at is deterministic instead of depending on time.Now.
var fixedBundleClock = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

// bundleMetaRow is the single bundle_meta row readBundleMeta reads back.
type bundleMetaRow struct {
	SnapshotVersion string
	Area            string
	CreatedAt       string
	ManifestSHA     string
}

// readBundleMeta opens path directly with database/sql (the "sqlite"
// driver is already registered process-wide, since importing package
// sqlite — required for ingestWCVPFixture/ExportBundle above — transitively
// blank-imports modernc.org/sqlite) and reads back its single bundle_meta
// row, so tests can assert on it without sqlite.DB exposing its internal
// *sql.DB.
func readBundleMeta(t *testing.T, path string) bundleMetaRow {
	t.Helper()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", path, err)
	}
	defer func() { _ = raw.Close() }()

	var got bundleMetaRow
	row := raw.QueryRow(`SELECT snapshot_version, area, created_at, source_manifest_sha FROM bundle_meta`)
	if err := row.Scan(&got.SnapshotVersion, &got.Area, &got.CreatedAt, &got.ManifestSHA); err != nil {
		t.Fatalf("reading bundle_meta from %q: unexpected error: %v", path, err)
	}
	return got
}

// exportAUTBundle ingests the real WCVP fixture into a source DB, exports
// an AUT-scoped bundle with a fixed clock, and opens the resulting file
// with a fresh sqlite.Open — the "prove it's an independently queryable
// standalone index" step every AUT-scoped test below builds on. It is
// shared (rather than inlined into one large test) so each assertion group
// stays its own small, single-purpose test function.
func exportAUTBundle(t *testing.T) (out string, report sqlite.BundleReport, bundle *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	src := ingestWCVPFixture(t)

	out = filepath.Join(t.TempDir(), "bundle.sqlite")
	report, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{
		Area:            "AUT",
		SnapshotVersion: "v1",
		Now:             func() time.Time { return fixedBundleClock },
	})
	if err != nil {
		t.Fatalf("ExportBundle: unexpected error: %v", err)
	}

	bundle, err = sqlite.Open(out)
	if err != nil {
		t.Fatalf("sqlite.Open(bundle): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = bundle.Close() })
	return out, report, bundle
}

// TestExportBundle_AreaFilter_ReportReflectsWhatWasCopied is the core
// RED/GREEN test's report-shape assertion: ExportBundle succeeds and
// reports the path it wrote plus exact counts. The fixture has exactly 3
// accepted concepts with an AUT distribution row (Corynephorus canescens
// 405825, Festuca ovina 415853, Jacobaea vulgaris 3082777 — see
// internal/adapters/wcvp/testdata/wcvp-sample/wcvp_distribution.csv), 13
// names linked to them (accepted + every synonym/illegitimate/invalid row
// under those 3 taxonids), and 16 distinct WGSRPD level-3 area codes across
// their combined distribution. Exact numbers (not just "> 0") are pinned
// deliberately: an off-by-one in copyRows' row-counting loop would
// otherwise go unnoticed.
func TestExportBundle_AreaFilter_ReportReflectsWhatWasCopied(t *testing.T) {
	out, report, _ := exportAUTBundle(t)
	if report.Path != out {
		t.Errorf("report.Path = %q, want %q", report.Path, out)
	}
	if report.Concepts != 3 {
		t.Errorf("report.Concepts = %d, want %d", report.Concepts, 3)
	}
	if report.Names != 13 {
		t.Errorf("report.Names = %d, want %d", report.Names, 13)
	}
	if report.Areas != 16 {
		t.Errorf("report.Areas = %d, want %d", report.Areas, 16)
	}
}

// TestExportBundle_AreaFilter_SuggestFindsInAreaConcept proves the bundle
// is a genuinely independent, standalone index — Suggest works against it
// through a fresh sqlite.Open (i.e. the rebuilt FTS index), not merely that
// the raw tables were copied.
func TestExportBundle_AreaFilter_SuggestFindsInAreaConcept(t *testing.T) {
	ctx := context.Background()
	_, _, bundle := exportAUTBundle(t)

	got, err := bundle.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10, Area: "AUT"})
	if err != nil {
		t.Fatalf("bundle.Suggest: unexpected error: %v", err)
	}
	var found *domain.SuggestItem
	for i := range got {
		if got[i].ConceptID == "wcvp:concept:405825" {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("bundle.Suggest(\"coryn\") = %+v, want an item for wcvp:concept:405825 (Corynephorus canescens)", got)
	}
	if !found.InArea {
		t.Error("found.InArea = false, want true (concept has an AUT distribution row)")
	}
}

// TestExportBundle_AreaFilter_ConceptResolvesWithSynonymsXrefsDistribution
// proves Concept resolves on the bundle with its synonyms, xrefs, and
// distribution intact.
func TestExportBundle_AreaFilter_ConceptResolvesWithSynonymsXrefsDistribution(t *testing.T) {
	ctx := context.Background()
	_, _, bundle := exportAUTBundle(t)

	concept, synonyms, xrefs, dists, err := bundle.Concept(ctx, "wcvp:concept:405825")
	if err != nil {
		t.Fatalf("bundle.Concept: unexpected error: %v", err)
	}
	if concept.AcceptedName.Canonical != "Corynephorus canescens" {
		t.Errorf("concept.AcceptedName.Canonical = %q, want %q", concept.AcceptedName.Canonical, "Corynephorus canescens")
	}
	if len(synonyms) == 0 {
		t.Error("synonyms = empty, want the fixture's synonym names carried over")
	}
	if len(xrefs) == 0 {
		t.Error("xrefs = empty, want the fixture's powo xref carried over")
	}
	if len(dists) == 0 {
		t.Error("distribution = empty, want the fixture's distribution rows carried over")
	}
}

// TestExportBundle_AreaFilter_ExcludesConceptsOutsideArea proves a concept
// with no AUT distribution row (the GENUS-rank Corynephorus,
// wcvp:concept:451295, has none in the fixture) is absent from the bundle.
func TestExportBundle_AreaFilter_ExcludesConceptsOutsideArea(t *testing.T) {
	ctx := context.Background()
	_, _, bundle := exportAUTBundle(t)

	if _, _, _, _, err := bundle.Concept(ctx, "wcvp:concept:451295"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("bundle.Concept(451295) error = %v, want domain.ErrNotFound (concept outside AUT must not be in the bundle)", err)
	}
}

// TestExportBundle_AreaFilter_BundleMetaRecordsProvenance proves
// bundle_meta carries the snapshot version, area, injected clock, and the
// source manifest checksum.
func TestExportBundle_AreaFilter_BundleMetaRecordsProvenance(t *testing.T) {
	out, _, _ := exportAUTBundle(t)

	meta := readBundleMeta(t, out)
	if meta.SnapshotVersion != "v1" {
		t.Errorf("bundle_meta.snapshot_version = %q, want %q", meta.SnapshotVersion, "v1")
	}
	if meta.Area != "AUT" {
		t.Errorf("bundle_meta.area = %q, want %q", meta.Area, "AUT")
	}
	if want := fixedBundleClock.UTC().Format(time.RFC3339); meta.CreatedAt != want {
		t.Errorf("bundle_meta.created_at = %q, want %q", meta.CreatedAt, want)
	}
	if meta.ManifestSHA == "" {
		t.Error("bundle_meta.source_manifest_sha = \"\", want the source manifest's checksum")
	}
}

// TestExportBundle_EmptyArea_IncludesEverything pins the documented
// whole-DB export convention: an empty Area copies every concept in src,
// including ones with no distribution row at all (the GENUS-rank
// Corynephorus, which the AUT-scoped test above proves is excluded from an
// area-filtered bundle).
func TestExportBundle_EmptyArea_IncludesEverything(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)

	out := filepath.Join(t.TempDir(), "bundle-all.sqlite")
	report, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("ExportBundle: unexpected error: %v", err)
	}

	bundle, err := sqlite.Open(out)
	if err != nil {
		t.Fatalf("sqlite.Open(bundle): unexpected error: %v", err)
	}
	defer func() { _ = bundle.Close() }()

	if _, _, _, _, err := bundle.Concept(ctx, "wcvp:concept:451295"); err != nil {
		t.Errorf("bundle.Concept(451295): unexpected error: %v (whole-DB export must include concepts without a distribution row)", err)
	}

	// 20 taxonid rows in the fixture; every accepted concept becomes one
	// taxon_concept row (see internal/application/ingest_test.go's report
	// counts). The exact number isn't pinned elsewhere, so just require
	// the report reflects what was actually copied.
	if report.Concepts == 0 {
		t.Fatal("report.Concepts = 0 for a whole-DB export, want > 0")
	}

	meta := readBundleMeta(t, out)
	if meta.Area != "" {
		t.Errorf("bundle_meta.area = %q, want empty (whole-DB export)", meta.Area)
	}
}

// TestExportBundle_CarriesTraitValuesAndVocabularyMetadata proves the
// offline bundle gap the Task 3 brief calls out is closed: trait_value and
// trait_vocabulary rows survive ExportBundle, and the exported file's own
// Traits/TraitVocabularies work against them through a fresh sqlite.Open —
// the offline field app needs trait data, not just taxonomy.
func TestExportBundle_CarriesTraitValuesAndVocabularyMetadata(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)

	bvs, err := src.BackboneVersions(ctx)
	if err != nil || len(bvs) == 0 {
		t.Fatalf("BackboneVersions: unexpected error/empty result: %v / %+v", err, bvs)
	}

	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, in AUT scope
	tx, err := src.BeginIngest(ctx, bvs[0])
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	niche := 2.5
	nsys := 12
	if err := tx.AddTraitValue(conceptID, domain.TraitValue{
		Vocab: domain.VocabEIVE, VocabVersion: "1.0", Dim: domain.DimM, Value: 5.5,
		NicheWidth: &niche, NSystems: &nsys,
	}); err != nil {
		t.Fatalf("AddTraitValue: unexpected error: %v", err)
	}
	if err := tx.UpsertTraitVocabulary(domain.TraitVocabMeta{
		Vocab: domain.VocabEIVE, Version: "1.0", Taxonomy: "euromed-aligned", License: "CC-BY-4.0",
	}); err != nil {
		t.Fatalf("UpsertTraitVocabulary: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	out := filepath.Join(t.TempDir(), "bundle-traits.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", Now: func() time.Time { return fixedBundleClock },
	}); err != nil {
		t.Fatalf("ExportBundle: unexpected error: %v", err)
	}

	bundle, err := sqlite.Open(out)
	if err != nil {
		t.Fatalf("sqlite.Open(bundle): unexpected error: %v", err)
	}
	defer func() { _ = bundle.Close() }()

	assertBundleTraits(t, bundle, conceptID)
	assertBundleTraitVocabularies(t, bundle)
}

func assertBundleTraits(t *testing.T, bundle *sqlite.DB, conceptID string) {
	t.Helper()
	sets, err := bundle.Traits(context.Background(), conceptID, nil)
	if err != nil {
		t.Fatalf("bundle.Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != domain.VocabEIVE || sets[0].Taxonomy != "euromed-aligned" {
		t.Fatalf("bundle.Traits(%q) = %+v, want one eive set with Taxonomy=euromed-aligned", conceptID, sets)
	}
	if len(sets[0].Values) != 1 || sets[0].Values[0].NicheWidth == nil || *sets[0].Values[0].NicheWidth != 2.5 {
		t.Fatalf("bundle.Traits(%q) values = %+v, want one M value with NicheWidth=2.5", conceptID, sets[0].Values)
	}
}

func assertBundleTraitVocabularies(t *testing.T, bundle *sqlite.DB) {
	t.Helper()
	metas, err := bundle.TraitVocabularies(context.Background())
	if err != nil {
		t.Fatalf("bundle.TraitVocabularies: unexpected error: %v", err)
	}
	if len(metas) != 1 || metas[0].Vocab != domain.VocabEIVE {
		t.Fatalf("bundle.TraitVocabularies() = %+v, want one eive entry", metas)
	}
}

// mustBundleConcept resolves id via bundle.Concept, returning just the
// concept and error — discarding the synonyms/xrefs/distribution results
// this file's tests don't need — so callers can inspect the error
// themselves (e.g. asserting domain.ErrNotFound) without a multi-blank-
// identifier assignment.
func mustBundleConcept(ctx context.Context, t *testing.T, bundle *sqlite.DB, id string) (*domain.Concept, error) {
	t.Helper()
	concept, synonyms, xrefs, dists, err := bundle.Concept(ctx, id)
	_ = synonyms
	_ = xrefs
	_ = dists
	return concept, err
}

// mustBundleSynonyms resolves id via bundle.Concept, failing the test on
// error, for callers that only need the synonym names — not the
// concept/xrefs/distribution results repo.Concept always returns alongside
// them.
func mustBundleSynonyms(ctx context.Context, t *testing.T, bundle *sqlite.DB, id string) []output.SynonymName {
	t.Helper()
	concept, synonyms, xrefs, dists, err := bundle.Concept(ctx, id)
	if err != nil {
		t.Fatalf("bundle.Concept(%q): unexpected error: %v", id, err)
	}
	_ = concept
	_ = xrefs
	_ = dists
	return synonyms
}

// TestExportBundle_AreaFilter_NullsOutOfAreaParentReference is the SP2
// forward-note this task closes: the fixture's Corynephorus canescens
// (405825, SPECIES) is IN the AUT scope (it has an AUT distribution row),
// but its parent — Corynephorus (451295, GENUS) — has NO distribution row
// at all (see TestExportBundle_AreaFilter_ExcludesConceptsOutsideArea) and
// so is NEVER copied into an AUT-scoped bundle. Since T7's ingest now
// populates taxon_concept.parent_id, copying 405825's row verbatim would
// FK-fail against a parent that was never copied. ExportBundle must
// instead null out that one out-of-scope reference: the export still
// succeeds, the bundle's copy of 405825 has parent_id NULL (not a dangling
// FK, not a dropped row), and the bundle stays fully queryable afterward.
func TestExportBundle_AreaFilter_NullsOutOfAreaParentReference(t *testing.T) {
	ctx := context.Background()
	out, _, bundle := exportAUTBundle(t)

	const (
		childID  = "wcvp:concept:405825" // Corynephorus canescens, AUT distribution -> in scope
		parentID = "wcvp:concept:451295" // Corynephorus (GENUS), no distribution at all -> out of scope
	)

	// The out-of-area parent must genuinely be absent from the bundle —
	// otherwise nulling its reference wouldn't be the interesting case.
	if _, err := mustBundleConcept(ctx, t, bundle, parentID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bundle.Concept(%q) error = %v, want domain.ErrNotFound (the out-of-area parent must not be in the bundle)", parentID, err)
	}

	child, err := mustBundleConcept(ctx, t, bundle, childID)
	if err != nil {
		t.Fatalf("bundle.Concept(%q): unexpected error: %v", childID, err)
	}
	if child.ParentID != "" {
		t.Errorf("bundle Concept(%q).ParentID = %q, want empty (NULLed, since %q is outside the AUT scope)", childID, child.ParentID, parentID)
	}

	// Belt-and-suspenders: assert the NULLing directly against the raw
	// column too, not just through Concept's COALESCE(parent_id, '').
	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = raw.Close() }()
	var parentIDCol sql.NullString
	if err := raw.QueryRow(`SELECT parent_id FROM taxon_concept WHERE id = ?`, childID).Scan(&parentIDCol); err != nil {
		t.Fatalf("reading taxon_concept.parent_id for %q: unexpected error: %v", childID, err)
	}
	if parentIDCol.Valid {
		t.Errorf("taxon_concept.parent_id for %q = %q, want SQL NULL", childID, parentIDCol.String)
	}

	// The bundle must stay fully queryable (Suggest/Traits), not just
	// FK-consistent — proving nulling the reference didn't otherwise
	// corrupt the row.
	if _, err := bundle.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10}); err != nil {
		t.Errorf("bundle.Suggest after NULLing an out-of-scope parent: unexpected error: %v", err)
	}
	if _, err := bundle.Traits(ctx, childID, nil); err != nil {
		t.Errorf("bundle.Traits(%q) after NULLing an out-of-scope parent: unexpected error: %v", childID, err)
	}
}

// TestExportBundle_AreaFilter_PreservesInScopeBasionymReference is the
// mirror image of TestExportBundle_AreaFilter_NullsOutOfAreaParentReference:
// a self-reference that DOES resolve within the copied scope must survive
// the export, not just an out-of-scope one being nulled. Bromus ovinus
// (401569, a synonym of Festuca ovina, AUT distribution) has its
// basionym_id resolved to Festuca ovina's own name (415853) — and BOTH
// names are copied into the AUT bundle (Festuca ovina is itself in scope),
// so this reference must come out linked, not NULLed by mistake.
func TestExportBundle_AreaFilter_PreservesInScopeBasionymReference(t *testing.T) {
	ctx := context.Background()
	out, _, bundle := exportAUTBundle(t)

	const (
		bromusOvinusNameID  = "wcvp:name:401569"
		festucaOvinaNameID  = "wcvp:name:415853"
		festucaOvinaConcept = "wcvp:concept:415853"
	)

	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = raw.Close() }()
	var basionymIDCol sql.NullString
	if err := raw.QueryRow(`SELECT basionym_id FROM name WHERE id = ?`, bromusOvinusNameID).Scan(&basionymIDCol); err != nil {
		t.Fatalf("reading name.basionym_id for %q: unexpected error: %v", bromusOvinusNameID, err)
	}
	if !basionymIDCol.Valid || basionymIDCol.String != festucaOvinaNameID {
		t.Errorf("name.basionym_id for %q = %+v, want %q (both names are in the AUT-scoped bundle)", bromusOvinusNameID, basionymIDCol, festucaOvinaNameID)
	}

	// Same fact, via the public Repository surface: Bromus ovinus must
	// still render homotypic:true (concept_name.homotypic, computed from
	// this exact basionym linkage at ingest time) after export.
	synonyms := mustBundleSynonyms(ctx, t, bundle, festucaOvinaConcept)
	var bromus *output.SynonymName
	for i := range synonyms {
		if synonyms[i].Canonical == "Bromus ovinus" {
			bromus = &synonyms[i]
		}
	}
	if bromus == nil {
		t.Fatalf("synonyms of %q = %+v, want an entry for %q", festucaOvinaConcept, synonyms, "Bromus ovinus")
	}
	if bromus.Homotypic == nil || !*bromus.Homotypic {
		t.Errorf("bundle Concept(%q) Bromus ovinus.Homotypic = %v, want a pointer to true", festucaOvinaConcept, bromus.Homotypic)
	}
}

// TestExportBundle_AreaWithNoMatchingConcepts_ProducesEmptyBundle exercises
// the "GER does not exist in the fixture" case the task brief calls out: no
// concept has a GER distribution row, so the bundle must come out empty
// (zero concepts/names/areas) rather than erroring, and bundle_meta must
// still be written.
func TestExportBundle_AreaWithNoMatchingConcepts_ProducesEmptyBundle(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)

	out := filepath.Join(t.TempDir(), "bundle-empty.sqlite")
	report, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "GER", SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("ExportBundle: unexpected error: %v", err)
	}
	if report.Concepts != 0 || report.Names != 0 || report.Areas != 0 {
		t.Errorf("report = %+v, want all-zero counts for an area with no matching concepts", report)
	}

	meta := readBundleMeta(t, out)
	if meta.SnapshotVersion != "v1" {
		t.Errorf("bundle_meta.snapshot_version = %q, want %q", meta.SnapshotVersion, "v1")
	}
	if meta.Area != "GER" {
		t.Errorf("bundle_meta.area = %q, want %q", meta.Area, "GER")
	}
	if meta.ManifestSHA != "" {
		t.Errorf("bundle_meta.source_manifest_sha = %q, want empty (no backbone_version rows were in scope)", meta.ManifestSHA)
	}
}
