package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// fixedBundleClock is injected via sqlite.BundleOpts.Now so bundle_meta's
// created_at is deterministic instead of depending on time.Now.
var fixedBundleClock = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

// bundleMetaRow is the single bundle_meta row readBundleMeta reads back.
type bundleMetaRow struct {
	SnapshotVersion   string
	Area              string
	CreatedAt         string
	ManifestSHA       string
	RestrictedSources string
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
	row := raw.QueryRow(`SELECT snapshot_version, area, created_at, source_manifest_sha, restricted_sources FROM bundle_meta`)
	if err := row.Scan(&got.SnapshotVersion, &got.Area, &got.CreatedAt, &got.ManifestSHA, &got.RestrictedSources); err != nil {
		t.Fatalf("reading bundle_meta from %q: unexpected error: %v", path, err)
	}
	return got
}

// addUnknownEIVETraitValue starts a fresh ingest transaction against src and
// records one trait_value row for conceptID under vocab=eive plus its
// trait_vocabulary metadata with Redistribution=unknown — the shared setup
// TestExportBundle_RefusesByDefaultWhenSourceNotAllowed and
// TestExportBundle_ForceIncludeRestricted_SucceedsAndRecordsSource both use
// to make "eive" a genuinely contributing, non-allowed source for the
// AUT-scoped bundle.
func addUnknownEIVETraitValue(t *testing.T, src *sqlite.DB, conceptID string) {
	t.Helper()
	ctx := context.Background()
	bvs, err := src.BackboneVersions(ctx)
	if err != nil || len(bvs) == 0 {
		t.Fatalf("BackboneVersions: unexpected error/empty result: %v / %+v", err, bvs)
	}
	tx, err := src.BeginIngest(ctx, bvs[0])
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.AddTraitValue(conceptID, domain.TraitValue{
		Vocab: domain.VocabEIVE, VocabVersion: "1.0", Dim: domain.DimM, Value: 5.5,
	}); err != nil {
		t.Fatalf("AddTraitValue: unexpected error: %v", err)
	}
	if err := tx.UpsertTraitVocabulary(domain.TraitVocabMeta{
		Vocab: domain.VocabEIVE, Version: "1.0", Taxonomy: "euromed-aligned", License: "",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertTraitVocabulary: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// addTwoEIVEVersionsWithDifferentRedistribution starts two fresh ingest
// transactions against src and records trait_value rows for conceptID under
// TWO versions of vocab=eive ("1.0" unknown, "2.0" restricted), each with
// its own trait_vocabulary metadata row. This mirrors what a real re-ingest
// at a new pinned version actually leaves behind: trait_vocabulary's
// primary key is (vocab, version), and IngestTraits never deletes an older
// version's row (see internal/application/traits_ingest.go), so both rows
// — and both trait_value rows — genuinely coexist in the database. It is
// the fixture for proving findRestrictedSources' dedup: without it, "eive"
// would be named/recorded twice under two different redistribution values.
func addTwoEIVEVersionsWithDifferentRedistribution(t *testing.T, src *sqlite.DB, conceptID string) {
	t.Helper()
	ctx := context.Background()
	bvs, err := src.BackboneVersions(ctx)
	if err != nil || len(bvs) == 0 {
		t.Fatalf("BackboneVersions: unexpected error/empty result: %v / %+v", err, bvs)
	}

	versions := []struct {
		version        string
		redistribution domain.Redistribution
	}{
		{"1.0", domain.RedistributionUnknown},
		{"2.0", domain.RedistributionRestricted},
	}
	for _, v := range versions {
		tx, err := src.BeginIngest(ctx, bvs[0])
		if err != nil {
			t.Fatalf("BeginIngest: unexpected error: %v", err)
		}
		if err := tx.AddTraitValue(conceptID, domain.TraitValue{
			Vocab: domain.VocabEIVE, VocabVersion: v.version, Dim: domain.DimM, Value: 5.5,
		}); err != nil {
			t.Fatalf("AddTraitValue(%s): unexpected error: %v", v.version, err)
		}
		if err := tx.UpsertTraitVocabulary(domain.TraitVocabMeta{
			Vocab: domain.VocabEIVE, Version: v.version, Taxonomy: "euromed-aligned", License: "",
			Redistribution: v.redistribution,
		}); err != nil {
			t.Fatalf("UpsertTraitVocabulary(%s): unexpected error: %v", v.version, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(%s): unexpected error: %v", v.version, err)
		}
	}
}

// TestExportBundle_SameVocabTwoVersionsDifferentRedistribution_NamedOnce is
// the fix-round-1 regression: trait_vocabulary's primary key is (vocab,
// version), so a bundle's scope can genuinely include trait_value rows from
// TWO versions of the same vocab id (eive 1.0 unknown, eive 2.0 restricted)
// — findRestrictedSources must still name "eive" exactly once in the
// refusal error (with the MORE SEVERE of the two values, "restricted", not
// silently the first-seen "unknown" — pinning dedupeRestrictedSourcesByID's
// severity-ranking choice, not just its presence/absence), and
// --force-include-restricted must record bundle_meta.restricted_sources as
// exactly "eive", never "eive,eive".
func TestExportBundle_SameVocabTwoVersionsDifferentRedistribution_NamedOnce(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, AUT scope
	addTwoEIVEVersionsWithDifferentRedistribution(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-dup-refused.sqlite")
	_, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err == nil {
		t.Fatal("ExportBundle: want an error when a vocab contributes under two non-allowed versions, got nil")
	}
	if got, want := strings.Count(err.Error(), "eive"), 1; got != want {
		t.Errorf("ExportBundle error = %q, want %q to appear exactly once, appeared %d times", err, "eive", got)
	}
	if !strings.Contains(err.Error(), "eive (redistribution=restricted)") {
		t.Errorf("ExportBundle error = %q, want it to report the MORE SEVERE value %q for eive, not %q", err, "restricted", "unknown")
	}

	forcedOut := filepath.Join(t.TempDir(), "bundle-dup-forced.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, forcedOut, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", AllowRestricted: true,
	}); err != nil {
		t.Fatalf("ExportBundle(AllowRestricted): unexpected error: %v", err)
	}
	meta := readBundleMeta(t, forcedOut)
	if meta.RestrictedSources != "eive" {
		t.Errorf("bundle_meta.restricted_sources = %q, want %q (not \"eive,eive\")", meta.RestrictedSources, "eive")
	}
}

// addRestrictedXrefSource records one xref_source row with
// Redistribution=restricted plus one xref row attributed to it for
// conceptID — the xref counterpart of addUnknownEIVETraitValue, making
// "wikidata" a genuinely contributing, non-allowed source for the AUT-scoped
// bundle.
func addRestrictedXrefSource(t *testing.T, src *sqlite.DB, conceptID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := src.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertXrefSource(domain.XrefSourceMeta{
		ID: "wikidata", Version: "2026-08-02", License: "CC0",
		SourceURL:   "https://query.wikidata.org/sparql",
		ManifestSHA: "cafebabe", Redistribution: domain.RedistributionRestricted,
	}); err != nil {
		t.Fatalf("UpsertXrefSource: unexpected error: %v", err)
	}
	if err := tx.AddXref(conceptID, domain.Xref{Authority: "inat", ExtID: "160927"}, "wikidata"); err != nil {
		t.Fatalf("AddXref: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// TestExportBundle_RefusesByDefaultWhenXrefSourceNotAllowed is the C1
// regression: before xref provenance existed, the redistribution gate
// queried only backbone_version and trait_vocabulary, so a whole class of
// data — every cross-reference harvested from an external source — was
// copied into bundles unconditionally, no matter what its manifest's
// (schema-REQUIRED) redistribution value said. A restricted xref source
// contributing an xref row to the scope must now refuse the export by
// default, naming the source and its redistribution value.
func TestExportBundle_RefusesByDefaultWhenXrefSourceNotAllowed(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, AUT scope
	addRestrictedXrefSource(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-xref-refused.sqlite")
	_, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err == nil {
		t.Fatal("ExportBundle: want an error when a contributing xref source is not redistribution-allowed, got nil")
	}
	if !strings.Contains(err.Error(), "wikidata (redistribution=restricted)") {
		t.Errorf("ExportBundle error = %q, want it to name the offending xref source and its value", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Errorf("ExportBundle refused, but %q was still created", out)
	}
}

// TestExportBundle_ForceIncludeRestrictedXrefSource_SucceedsAndRecordsSource
// is the opt-out half of the xref gate: --force-include-restricted must
// export, record "wikidata" in bundle_meta.restricted_sources, and carry the
// source's own provenance row plus the xref's attribution into the bundle —
// so the exported file itself still says where those cross-references came
// from and that they were not cleared.
func TestExportBundle_ForceIncludeRestrictedXrefSource_SucceedsAndRecordsSource(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825"
	addRestrictedXrefSource(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-xref-forced.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", AllowRestricted: true,
	}); err != nil {
		t.Fatalf("ExportBundle(AllowRestricted): unexpected error: %v", err)
	}

	meta := readBundleMeta(t, out)
	if meta.RestrictedSources != "wikidata" {
		t.Errorf("bundle_meta.restricted_sources = %q, want %q", meta.RestrictedSources, "wikidata")
	}

	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = raw.Close() }()

	var version, sha, redistribution string
	if err := raw.QueryRow(`SELECT version, manifest_sha, redistribution FROM xref_source WHERE id = 'wikidata'`).
		Scan(&version, &sha, &redistribution); err != nil {
		t.Fatalf("reading xref_source from the bundle: unexpected error: %v", err)
	}
	if version != "2026-08-02" || sha != "cafebabe" || redistribution != "restricted" {
		t.Errorf("bundle xref_source row = (%q, %q, %q), want (%q, %q, %q)",
			version, sha, redistribution, "2026-08-02", "cafebabe", "restricted")
	}

	var source string
	if err := raw.QueryRow(`SELECT source FROM xref WHERE authority = 'inat' AND ext_id = '160927'`).Scan(&source); err != nil {
		t.Fatalf("reading xref.source from the bundle: unexpected error: %v", err)
	}
	if source != "wikidata" {
		t.Errorf("bundle xref.source = %q, want %q", source, "wikidata")
	}
}

// setBackboneRedistribution re-records src's existing "wcvp" backbone_version
// row with redistribution set to value, via the same BeginIngest(INSERT OR
// REPLACE) path a real re-ingest would use — so tests can make the
// BACKBONE itself (not just a trait vocabulary) a non-allowed contributing
// source without re-ingesting the whole fixture.
func setBackboneRedistribution(t *testing.T, src *sqlite.DB, redistribution domain.Redistribution) {
	t.Helper()
	ctx := context.Background()
	bvs, err := src.BackboneVersions(ctx)
	if err != nil || len(bvs) == 0 {
		t.Fatalf("BackboneVersions: unexpected error/empty result: %v / %+v", err, bvs)
	}
	bv := bvs[0]
	bv.Redistribution = redistribution
	tx, err := src.BeginIngest(ctx, bv)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// TestExportBundle_RefusesNamingEverySourceSortedByID proves
// findRestrictedSources' sort actually orders MULTIPLE offending sources —
// both the backbone itself (wcvp, forced to restricted) and the eive trait
// vocabulary (unknown) contribute to the same AUT scope here, so the error
// must name both, alphabetically ("eive" before "wcvp").
func TestExportBundle_RefusesNamingEverySourceSortedByID(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, AUT scope
	setBackboneRedistribution(t, src, domain.RedistributionRestricted)
	addUnknownEIVETraitValue(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-multi-refused.sqlite")
	_, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err == nil {
		t.Fatal("ExportBundle: want an error when multiple contributing sources are not redistribution-allowed, got nil")
	}
	eiveIdx := strings.Index(err.Error(), "eive")
	wcvpIdx := strings.Index(err.Error(), "wcvp")
	if eiveIdx == -1 || wcvpIdx == -1 {
		t.Fatalf("ExportBundle error = %q, want it to name both %q and %q", err, "eive", "wcvp")
	}
	if eiveIdx > wcvpIdx {
		t.Errorf("ExportBundle error = %q, want %q to be named before %q (sorted by id)", err, "eive", "wcvp")
	}

	// With --force-include-restricted, bundle_meta.restricted_sources must
	// list BOTH ids, comma-joined and sorted.
	forcedOut := filepath.Join(t.TempDir(), "bundle-multi-forced.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, forcedOut, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", AllowRestricted: true,
	}); err != nil {
		t.Fatalf("ExportBundle(AllowRestricted): unexpected error: %v", err)
	}
	meta := readBundleMeta(t, forcedOut)
	if meta.RestrictedSources != "eive,wcvp" {
		t.Errorf("bundle_meta.restricted_sources = %q, want %q", meta.RestrictedSources, "eive,wcvp")
	}
}

// TestExportBundle_RefusesByDefaultWhenSourceNotAllowed is the RED/GREEN
// core of the redistribution gate: a bundle scope that includes trait_value
// rows from a vocabulary whose redistribution is "unknown" (not "allowed")
// must be refused by default, naming the offending source and its
// redistribution value in the error — never silently exported.
func TestExportBundle_RefusesByDefaultWhenSourceNotAllowed(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, AUT scope
	addUnknownEIVETraitValue(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-refused.sqlite")
	_, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err == nil {
		t.Fatal("ExportBundle: want an error when a contributing source is not redistribution-allowed, got nil")
	}
	if !strings.Contains(err.Error(), "eive") {
		t.Errorf("ExportBundle error = %q, want it to name the offending source %q", err, "eive")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("ExportBundle error = %q, want it to state the redistribution value %q", err, "unknown")
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Errorf("ExportBundle refused, but %q was still created", out)
	}
}

// TestExportBundle_ForceIncludeRestricted_SucceedsAndRecordsSource proves
// the opt-out: with AllowRestricted, the same scope that
// TestExportBundle_RefusesByDefaultWhenSourceNotAllowed refuses now
// succeeds, AND bundle_meta.restricted_sources names exactly the offending
// source — a bundle can never silently carry unclearable data even when the
// gate is overridden.
func TestExportBundle_ForceIncludeRestricted_SucceedsAndRecordsSource(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, AUT scope
	addUnknownEIVETraitValue(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-forced.sqlite")
	report, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", AllowRestricted: true,
		Now: func() time.Time { return fixedBundleClock },
	})
	if err != nil {
		t.Fatalf("ExportBundle(AllowRestricted): unexpected error: %v", err)
	}
	if report.Concepts == 0 {
		t.Fatal("ExportBundle(AllowRestricted): report.Concepts = 0, want the AUT-scoped concepts copied")
	}

	meta := readBundleMeta(t, out)
	if meta.RestrictedSources != "eive" {
		t.Errorf("bundle_meta.restricted_sources = %q, want %q", meta.RestrictedSources, "eive")
	}
}

// TestExportBundle_AllAllowedSources_RestrictedSourcesEmpty is the
// regression guard for "an all-allowed DB exports exactly as before": the
// existing AUT bundle (no non-allowed source anywhere in scope) must record
// an EMPTY bundle_meta.restricted_sources, proving the gate is a genuine
// no-op when nothing is restricted.
func TestExportBundle_AllAllowedSources_RestrictedSourcesEmpty(t *testing.T) {
	out, _, _ := exportAUTBundle(t)
	meta := readBundleMeta(t, out)
	if meta.RestrictedSources != "" {
		t.Errorf("bundle_meta.restricted_sources = %q, want empty (every contributing source is allowed)", meta.RestrictedSources)
	}
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
// under those 3 taxonids), and — per the Task 4 size reduction
// (copyDistribution's doc comment) — exactly 1 distinct WGSRPD level-3
// area code across their combined distribution: "AUT" itself, not their
// full global range (which the pre-reduction fixture measured at 16
// distinct codes). See TestExportBundle_EmptyArea_IncludesEverything for
// the matching unscoped-side proof: an unscoped export of the SAME
// concept (wcvp:concept:405825) keeps all 9 of its distribution rows,
// which is what makes this scoped-side drop a deliberate scoping decision
// rather than silent data loss. Exact numbers (not just "> 0") are pinned
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
	if report.Areas != 1 {
		t.Errorf("report.Areas = %d, want %d (only the requested area, not the concepts' full global range)", report.Areas, 1)
	}
}

// TestExportBundle_AreaScoped_DistributionExcludesOutOfScopeAreas is the
// Task 4 size-reduction test: an area-scoped bundle's distribution table
// must contain ONLY rows for the requested area(s), not a concept's full
// global range. The fixture's Corynephorus canescens (405825) has
// distribution rows in areas other than AUT (its combined-with-siblings
// global range spans 16 codes, per the report test above) — after an
// AUT-scoped export, none of those other codes may appear for it.
func TestExportBundle_AreaScoped_DistributionExcludesOutOfScopeAreas(t *testing.T) {
	ctx := context.Background()
	_, _, bundle := exportAUTBundle(t)

	concept, synonyms, xrefs, dists, err := bundle.Concept(ctx, "wcvp:concept:405825")
	if err != nil {
		t.Fatalf("bundle.Concept: unexpected error: %v", err)
	}
	_ = concept
	_ = synonyms
	_ = xrefs
	if len(dists) == 0 {
		t.Fatal("distribution = empty, want at least the AUT row")
	}
	for _, d := range dists {
		if d.AreaCode != "AUT" {
			t.Errorf("distribution row = %+v, want only area_code=AUT (out-of-scope areas must be dropped from a scoped bundle)", d)
		}
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
// area-filtered bundle) — AND, per Task 4's copyDistribution doc comment,
// every distribution row of an in-scope concept, not just the ones in a
// requested area (there is no requested area here to begin with). This is
// the deliberate "unscoped still means everything" half of the size
// reduction pair: TestExportBundle_AreaFilter_ReportReflectsWhatWasCopied
// proves an AUT-scoped export of the SAME fixture concept
// (wcvp:concept:405825) drops down to just its AUT row; this test proves
// an unscoped export of that same concept keeps all 9 of its distribution
// rows (internal/adapters/wcvp/testdata/wcvp-sample/wcvp_distribution.csv
// has exactly 9 rows for taxonid 405825) — without this assertion, the
// reduction could have silently become "always scope to something" and no
// test would catch it, since report.Concepts>0 alone says nothing about
// distribution row survival.
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

	concept405825, synonyms405825, xrefs405825, dists, err := bundle.Concept(ctx, "wcvp:concept:405825")
	if err != nil {
		t.Fatalf("bundle.Concept(405825): unexpected error: %v", err)
	}
	_ = concept405825
	_ = synonyms405825
	_ = xrefs405825
	if len(dists) != 9 {
		t.Errorf("bundle.Concept(405825) distribution = %d rows, want 9 (a whole-DB export must copy a concept's FULL distribution, not just an in-scope subset — there is no scope here)", len(dists))
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
	// A second value that reached this concept only through the flagged
	// aggregate-to-nominate-species rule. An offline bundle is the copy a
	// field user actually queries, so the "this was normalised, not matched
	// exactly" signal has to survive the export — dropping it here would
	// re-hide precisely what Hardening Task 5 made visible.
	if err := tx.AddTraitValue(conceptID, domain.TraitValue{
		Vocab: domain.VocabEIVE, VocabVersion: "1.0", Dim: domain.DimN, Value: 4.0,
		Resolution: string(domain.RuleAggregateToNominate),
	}); err != nil {
		t.Fatalf("AddTraitValue(normalised): unexpected error: %v", err)
	}
	if err := tx.UpsertTraitVocabulary(domain.TraitVocabMeta{
		Vocab: domain.VocabEIVE, Version: "1.0", Taxonomy: "euromed-aligned", License: "CC-BY-4.0",
		Redistribution: domain.RedistributionAllowed,
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
	if len(sets[0].Values) != 2 {
		t.Fatalf("bundle.Traits(%q) values = %+v, want the M and N values", conceptID, sets[0].Values)
	}
	// M matched exactly (empty Resolution), N only through the flagged
	// aggregate fallback — both must survive the export unchanged.
	wantResolution := map[domain.TraitDim]string{
		domain.DimM: "",
		domain.DimN: string(domain.RuleAggregateToNominate),
	}
	for _, v := range sets[0].Values {
		want, known := wantResolution[v.Dim]
		if !known {
			t.Errorf("unexpected dim %q in bundle traits", v.Dim)
			continue
		}
		if v.Resolution != want {
			t.Errorf("bundle %s value: Resolution = %q, want %q — the normalisation signal must survive the export",
				v.Dim, v.Resolution, want)
		}
		if v.Dim == domain.DimM && (v.NicheWidth == nil || *v.NicheWidth != 2.5) {
			t.Errorf("bundle M value = %+v, want NicheWidth=2.5", v)
		}
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

// sliceRowSource is a minimal application.RowSource backed by an in-memory
// slice, for tests that need a specific TaxonRow (e.g. an exotic rank) or
// DistributionRow set the shared WCVP fixture doesn't happen to carry.
type sliceRowSource struct {
	taxa  []application.TaxonRow
	dists []application.DistributionRow
}

func (s sliceRowSource) Taxa() []application.TaxonRow                 { return s.taxa }
func (s sliceRowSource) Distributions() []application.DistributionRow { return s.dists }

// ingestMultiAreaFixture ingests two accepted, unrelated concepts into a
// fresh in-memory repo — one with a WGSRPD-L3 distribution row in "AUT"
// only, the other in "SWI" only — so multi-area scoping tests can prove
// "--area AUT,SWI" selects the UNION of both, not just one, without
// depending on the shared WCVP fixture (whose 3 concepts all happen to
// share an AUT row — see TestExportBundle_AreaFilter_ReportReflectsWhatWasCopied
// — making it useless for proving a union across two DISJOINT areas).
func ingestMultiAreaFixture(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp-multiarea", Version: "v1", Redistribution: "allowed"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "aut1", AcceptedTaxonID: "aut1", Accepted: true, Canonical: "Autonia austriaca", Rank: "SPECIES", Status: "Accepted"},
		{TaxonID: "swi1", AcceptedTaxonID: "swi1", Accepted: true, Canonical: "Swissia helvetica", Rank: "SPECIES", Status: "Accepted"},
	}
	dists := []application.DistributionRow{
		{TaxonID: "aut1", AreaCode: "AUT"},
		{TaxonID: "swi1", AreaCode: "SWI"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return sliceRowSource{taxa: taxa, dists: dists}, nil
	}
	if _, err := application.Ingest(context.Background(), ds, readerFor, db); err != nil {
		t.Fatalf("application.Ingest: unexpected error: %v", err)
	}
	return db
}

// TestExportBundle_MultiArea_SelectsUnionOfAreas is the Task 4 RED/GREEN
// core for multi-area scoping: "--area AUT,SWI" must select BOTH
// ingestMultiAreaFixture concepts (one is only in AUT, the other only in
// SWI) — the union, not an intersection or just the first value — while a
// single value keeps selecting only its own concept, proving the existing
// single-area form is unaffected.
func TestExportBundle_MultiArea_SelectsUnionOfAreas(t *testing.T) {
	ctx := context.Background()
	src := ingestMultiAreaFixture(t)

	autOut := filepath.Join(t.TempDir(), "bundle-aut-only.sqlite")
	autReport, err := sqlite.ExportBundle(ctx, src, autOut, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("ExportBundle(AUT): unexpected error: %v", err)
	}
	if autReport.Concepts != 1 {
		t.Errorf("ExportBundle(AUT).Concepts = %d, want %d (single value must keep working as before)", autReport.Concepts, 1)
	}

	swiOut := filepath.Join(t.TempDir(), "bundle-swi-only.sqlite")
	swiReport, err := sqlite.ExportBundle(ctx, src, swiOut, sqlite.BundleOpts{Area: "SWI", SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("ExportBundle(SWI): unexpected error: %v", err)
	}
	if swiReport.Concepts != 1 {
		t.Errorf("ExportBundle(SWI).Concepts = %d, want %d", swiReport.Concepts, 1)
	}

	unionOut := filepath.Join(t.TempDir(), "bundle-aut-swi.sqlite")
	unionReport, err := sqlite.ExportBundle(ctx, src, unionOut, sqlite.BundleOpts{Area: "AUT,SWI", SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("ExportBundle(AUT,SWI): unexpected error: %v", err)
	}
	if unionReport.Concepts != 2 {
		t.Fatalf("ExportBundle(AUT,SWI).Concepts = %d, want %d (the union of both single-area exports)", unionReport.Concepts, 2)
	}

	bundle, err := sqlite.Open(unionOut)
	if err != nil {
		t.Fatalf("sqlite.Open(union bundle): unexpected error: %v", err)
	}
	defer func() { _ = bundle.Close() }()
	if _, err := bundle.Suggest(ctx, "auton", output.SuggestOpts{Limit: 10}); err != nil {
		t.Errorf("bundle.Suggest(AUT concept) on union bundle: unexpected error: %v", err)
	}
	if _, _, _, _, err := bundle.Concept(ctx, "wcvp-multiarea:concept:swi1"); err != nil {
		t.Errorf("bundle.Concept(swi1) on union bundle: unexpected error: %v", err)
	}

	meta := readBundleMeta(t, unionOut)
	if meta.Area != "AUT,SWI" {
		t.Errorf("bundle_meta.area = %q, want the raw requested value %q", meta.Area, "AUT,SWI")
	}
}

// TestExportBundle_ScopeIndependentExport_LargeConceptSetSucceeds is the
// Task 4 RED/GREEN core for the "too many SQL variables" defect
// (docs/research/reality-check.md M5.1): the old code bound one SQL
// placeholder per concept id, so a scope large enough to exceed SQLite's
// SQLITE_MAX_VARIABLE_NUMBER failed outright. This ingests enough synthetic
// concepts to exceed a conservative placeholder budget (well under the
// 440,098 concepts the real WCVP database has — see
// docs/research/reality-check.md M5.1's measured failure and this task's
// report for that full-scale proof) while staying fast as a unit test, and
// asserts an UNSCOPED (Area empty) export of all of them still succeeds.
func TestExportBundle_ScopeIndependentExport_LargeConceptSetSucceeds(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	defer func() { _ = db.Close() }()

	const n = 5000 // exceeds SQLite's historical default SQLITE_MAX_VARIABLE_NUMBER of 999
	taxa := make([]application.TaxonRow, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%d", i)
		taxa = append(taxa, application.TaxonRow{
			TaxonID: id, AcceptedTaxonID: id, Accepted: true,
			Canonical: fmt.Sprintf("Generusx speciesus%d", i), Rank: "SPECIES", Status: "Accepted",
		})
	}
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp-large", Version: "v1", Redistribution: "allowed"}}, ManifestSHA: "x"}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return sliceRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, db); err != nil {
		t.Fatalf("application.Ingest: unexpected error: %v", err)
	}

	out := filepath.Join(t.TempDir(), "bundle-large-unscoped.sqlite")
	report, err := sqlite.ExportBundle(ctx, db, out, sqlite.BundleOpts{SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("ExportBundle (unscoped, %d concepts): unexpected error: %v", n, err)
	}
	if report.Concepts != n {
		t.Errorf("report.Concepts = %d, want %d", report.Concepts, n)
	}
}

// ingestOtherRankFixture ingests one "proles" concept (WCVP's real exotic
// rank — see docs/research/reality-check.md's M1.0) into a fresh in-memory
// repo, so ExportBundle's rank_verbatim carry-through can be tested without
// depending on the shared WCVP fixture containing an exotic rank itself.
func ingestOtherRankFixture(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp-other", Version: "v1", Redistribution: "allowed"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "2", AcceptedTaxonID: "2", Accepted: true, Canonical: "Paeonia corallina proles ovatifolia", Authorship: "Rouy & Foucaud", Rank: "proles", Status: "Synonym"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return sliceRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(context.Background(), ds, readerFor, db); err != nil {
		t.Fatalf("application.Ingest: unexpected error: %v", err)
	}
	return db
}

// TestExportBundle_CarriesRankVerbatimThrough proves Hardening Task 1's
// fix-round-1 requirement that rank_verbatim survives a bundle export, not
// just a live ingest: a "proles" concept's name.rank_verbatim/
// taxon_concept.rank_verbatim must both read back as "proles" from the
// exported bundle file, and repo.Concept against the reopened bundle must
// surface it via domain.Concept.RankVerbatim.
func TestExportBundle_CarriesRankVerbatimThrough(t *testing.T) {
	ctx := context.Background()
	src := ingestOtherRankFixture(t)

	out := filepath.Join(t.TempDir(), "bundle-other-rank.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{SnapshotVersion: "v1"}); err != nil {
		t.Fatalf("ExportBundle: unexpected error: %v", err)
	}

	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = raw.Close() }()

	var nameVerbatim, conceptVerbatim string
	if err := raw.QueryRow(`SELECT rank_verbatim FROM name WHERE id = ?`, "wcvp-other:name:2").Scan(&nameVerbatim); err != nil {
		t.Fatalf("reading name.rank_verbatim: unexpected error: %v", err)
	}
	if nameVerbatim != "proles" {
		t.Errorf("bundle name.rank_verbatim = %q, want %q", nameVerbatim, "proles")
	}
	if err := raw.QueryRow(`SELECT rank_verbatim FROM taxon_concept WHERE id = ?`, "wcvp-other:concept:2").Scan(&conceptVerbatim); err != nil {
		t.Fatalf("reading taxon_concept.rank_verbatim: unexpected error: %v", err)
	}
	if conceptVerbatim != "proles" {
		t.Errorf("bundle taxon_concept.rank_verbatim = %q, want %q", conceptVerbatim, "proles")
	}

	bundle, err := sqlite.Open(out)
	if err != nil {
		t.Fatalf("sqlite.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = bundle.Close() }()

	concept, synonyms, xrefs, dists, err := bundle.Concept(ctx, "wcvp-other:concept:2")
	if err != nil {
		t.Fatalf("bundle.Concept: unexpected error: %v", err)
	}
	_ = synonyms
	_ = xrefs
	_ = dists
	if concept.Rank != domain.RankOther {
		t.Errorf("bundle concept.Rank = %q, want %q", concept.Rank, domain.RankOther)
	}
	if concept.RankVerbatim != "proles" {
		t.Errorf("bundle concept.RankVerbatim = %q, want %q", concept.RankVerbatim, "proles")
	}
}
