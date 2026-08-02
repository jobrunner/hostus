package application_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// wcvpRowSource adapts a *wcvp.Dataset (T4's reader output) into
// application.RowSource — the mapping the real composition root (cmd/app,
// T8) will perform, kept here as the boundary-respecting way to exercise
// Ingest against the real WCVP fixture without application importing the
// wcvp adapter package.
type wcvpRowSource struct{ ds *wcvp.Dataset }

func (s wcvpRowSource) Taxa() []application.TaxonRow {
	out := make([]application.TaxonRow, 0, len(s.ds.Taxa))
	for _, t := range s.ds.Taxa {
		out = append(out, application.TaxonRow{
			TaxonID:         t.TaxonID,
			AcceptedTaxonID: t.AcceptedNameUsageID,
			Accepted:        t.IsAccepted(),
			Canonical:       t.Canonical,
			Authorship:      t.Authorship,
			Rank:            t.Rank,
			Status:          t.Status,
			POWOID:          t.POWOID(),
			ParentTaxonID:   t.ParentNameUsageID,
			BasionymTaxonID: t.OriginalNameUsageID,
		})
	}
	return out
}

func (s wcvpRowSource) Distributions() []application.DistributionRow {
	out := make([]application.DistributionRow, 0, len(s.ds.Distributions))
	for _, d := range s.ds.Distributions {
		out = append(out, application.DistributionRow{TaxonID: d.CoreID, AreaCode: d.AreaCode()})
	}
	return out
}

// loadDataset parses the test manifest (testdata/dataset.yaml, pointing at
// wcvp's real fixture directory) and adapts it into an application.Dataset
// — the mapping the composition root performs so application never imports
// internal/adapters/manifest in production code.
func loadDataset(t *testing.T) *application.Dataset {
	t.Helper()
	ds, err := manifest.Parse("testdata/dataset.yaml")
	if err != nil {
		t.Fatalf("manifest.Parse: unexpected error: %v", err)
	}
	backbones := make([]application.Backbone, 0, len(ds.Backbones))
	for _, b := range ds.Backbones {
		backbones = append(backbones, application.Backbone{
			ID:        b.ID,
			Version:   b.Version,
			License:   b.License,
			SourceURL: b.SourceURL,
			Path:      b.Path,
		})
	}
	return &application.Dataset{Backbones: backbones, ManifestSHA: ds.ManifestSHA}
}

func wcvpReaderFor(b application.Backbone) (application.RowSource, error) {
	ds, err := wcvp.Read(b.Path)
	if err != nil {
		return nil, err
	}
	return wcvpRowSource{ds: ds}, nil
}

func openMemoryRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestIngest_WCVPFixture_RecordsBackboneVersionWithManifestSHA(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	versions, err := repo.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	var wcvpVersion *domain.BackboneVersion
	for i := range versions {
		if versions[i].ID == "wcvp" {
			wcvpVersion = &versions[i]
		}
	}
	if wcvpVersion == nil {
		t.Fatalf("BackboneVersions() = %+v, want an entry for %q", versions, "wcvp")
	}
	if wcvpVersion.ManifestSHA == "" {
		t.Error("BackboneVersion.ManifestSHA = empty, want the manifest's checksum")
	}
	if wcvpVersion.ManifestSHA != ds.ManifestSHA {
		t.Errorf("BackboneVersion.ManifestSHA = %q, want %q (the manifest that was actually ingested)", wcvpVersion.ManifestSHA, ds.ManifestSHA)
	}
	if wcvpVersion.Version != "2026-06-15" {
		t.Errorf("BackboneVersion.Version = %q, want %q", wcvpVersion.Version, "2026-06-15")
	}
}

func TestIngest_WCVPFixture_ReportCounts(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	report, err := application.Ingest(ctx, ds, wcvpReaderFor, repo)
	if err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	if got, want := len(report.Backbones), 1; got != want {
		t.Fatalf("len(report.Backbones) = %d, want %d", got, want)
	}
	b := report.Backbones[0]
	if b.ID != "wcvp" {
		t.Errorf("report.Backbones[0].ID = %q, want %q", b.ID, "wcvp")
	}
	// 20 taxon rows in the fixture, every one gets a Name row.
	if b.Names != 20 {
		t.Errorf("report.Backbones[0].Names = %d, want %d", b.Names, 20)
	}
	// 8 rows are self-referentially accepted (see wcvp fixture: two
	// genus-rank accepted rows, one unused genus, and five species/infra-
	// specific accepted rows).
	if b.Concepts != 8 {
		t.Errorf("report.Backbones[0].Concepts = %d, want %d", b.Concepts, 8)
	}
	// 12 non-accepted rows, one of which (taxonid 3082280) points at an
	// accepted taxonid (3082278) absent from the fixture — a dangling
	// reference that must be skipped, not fatal.
	if b.Synonyms != 11 {
		t.Errorf("report.Backbones[0].Synonyms = %d, want %d", b.Synonyms, 11)
	}
	if b.Orphaned != 1 {
		t.Errorf("report.Backbones[0].Orphaned = %d, want %d (the dangling accepted-name reference)", b.Orphaned, 1)
	}
}

func TestIngest_WCVPFixture_GroupsSynonymsUnderAcceptedConcept(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref(powo, 396681-1): unexpected error: %v", err)
	}

	t.Run("concept fields", func(t *testing.T) {
		assertCorynephorusConcept(t, concept)
	})

	gotConcept, synonyms, xrefs, dists, err := repo.Concept(ctx, concept.ID)
	if err != nil {
		t.Fatalf("Concept(%q): unexpected error: %v", concept.ID, err)
	}
	if gotConcept.ID != concept.ID {
		t.Errorf("Concept returned id %q, want %q", gotConcept.ID, concept.ID)
	}

	t.Run("synonyms", func(t *testing.T) {
		assertCorynephorusSynonyms(t, synonyms)
	})
	t.Run("xrefs", func(t *testing.T) {
		if len(xrefs) != 1 || xrefs[0].Authority != "powo" || xrefs[0].ExtID != "396681-1" {
			t.Errorf("xrefs = %+v, want exactly [{powo 396681-1}]", xrefs)
		}
	})
	t.Run("distribution", func(t *testing.T) {
		assertCorynephorusDistribution(t, dists)
	})
}

// mustConcept resolves id via repo.Concept, failing the test on error, for
// callers that only care about the returned *domain.Concept itself (its
// ParentID/AcceptedName.BasionymID) — not the synonyms/xrefs/distribution
// results, which repo.Concept always returns alongside it.
func mustConcept(ctx context.Context, t *testing.T, repo output.Repository, id string) *domain.Concept {
	t.Helper()
	concept, synonyms, xrefs, dists, err := repo.Concept(ctx, id)
	if err != nil {
		t.Fatalf("Concept(%q): unexpected error: %v", id, err)
	}
	_ = synonyms
	_ = xrefs
	_ = dists
	return concept
}

// mustSynonyms resolves id via repo.Concept, failing the test on error, for
// callers that only need the synonym names — not the concept/xrefs/
// distribution results repo.Concept always returns alongside them.
func mustSynonyms(ctx context.Context, t *testing.T, repo output.Repository, id string) []output.SynonymName {
	t.Helper()
	concept, synonyms, xrefs, dists, err := repo.Concept(ctx, id)
	if err != nil {
		t.Fatalf("Concept(%q): unexpected error: %v", id, err)
	}
	_ = concept
	_ = xrefs
	_ = dists
	return synonyms
}

// TestIngest_WCVPFixture_PopulatesParentID proves T7's ingest resolves
// taxon_concept.parent_id from WCVP's parentnameusageid: the fixture's
// Corynephorus canescens (405825) has parentnameusageid=451295, and 451295
// (Corynephorus, GENUS) IS itself an accepted row in the same fixture, so
// it must resolve to that genus concept's id, not stay empty/NULL.
func TestIngest_WCVPFixture_PopulatesParentID(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	const (
		corynephorusCanescensID = "wcvp:concept:405825"
		corynephorusGenusID     = "wcvp:concept:451295"
	)
	concept := mustConcept(ctx, t, repo, corynephorusCanescensID)
	if concept.ParentID != corynephorusGenusID {
		t.Errorf("concept.ParentID = %q, want %q (the fixture's Corynephorus genus concept, ingested from the same taxon file)", concept.ParentID, corynephorusGenusID)
	}
}

// TestIngest_WCVPFixture_ParentOutsideIngestedSetStaysEmpty proves the
// conservative half of the parent_id rule: Festuca ovina's own accepted row
// (415853) has parentnameusageid=451511 (Festuca, GENUS) which IS in the
// fixture, so that one resolves — but Jacobaea vulgaris subsp. dunensis
// (3082806, accepted) has originalnameusageid=3082804, a taxonid absent
// from the fixture entirely; its Name.BasionymID must stay empty (NULL)
// rather than dangling. This exercises the "target not ingested -> leave
// NULL" rule from the OTHER linkage column (basionym_id) so both self-
// referencing columns get an explicit not-resolvable case covered.
func TestIngest_WCVPFixture_UnresolvableBasionymStaysEmpty(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	const jacobaeaDunensisID = "wcvp:concept:3082806"
	concept := mustConcept(ctx, t, repo, jacobaeaDunensisID)
	if concept.AcceptedName.BasionymID != "" {
		t.Errorf("concept.AcceptedName.BasionymID = %q, want empty (originalnameusageid 3082804 is absent from the fixture, a dangling reference that must not be invented)", concept.AcceptedName.BasionymID)
	}
}

// TestIngest_WCVPFixture_HomotypicRule proves T7's conservative homotypic
// rule end to end against the real fixture:
//
//   - Bromus ovinus (L.) Scop. (401569), a synonym of Festuca ovina L.
//     (415853), has originalnameusageid=415853 — its basionym IS the
//     accepted name itself, a straight recombination — so it must come
//     back homotypic:true.
//   - Festuca duriuscula L. (415030), also a synonym of Festuca ovina, has
//     no originalnameusageid at all — nothing PROVES its typification, so
//     it must stay NULL (Homotypic == nil), never a guessed false.
func TestIngest_WCVPFixture_HomotypicRule(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	const festucaOvinaID = "wcvp:concept:415853"
	synonyms := mustSynonyms(ctx, t, repo, festucaOvinaID)

	byCanonical := make(map[string]output.SynonymName, len(synonyms))
	for _, s := range synonyms {
		byCanonical[s.Canonical] = s
	}

	bromus, ok := byCanonical["Bromus ovinus"]
	if !ok {
		t.Fatalf("synonyms of %q = %+v, want an entry for %q", festucaOvinaID, synonyms, "Bromus ovinus")
	}
	if bromus.Homotypic == nil || !*bromus.Homotypic {
		t.Errorf("Bromus ovinus.Homotypic = %v, want a pointer to true (recombination of the accepted name Festuca ovina L.)", bromus.Homotypic)
	}
	// Also assert the underlying linkage this rule is derived from:
	// Bromus ovinus's own Name.BasionymID must actually be linked (pass 1's
	// second sub-pass, linkSelfReferences) to Festuca ovina's name id.
	const festucaOvinaNameID = "wcvp:name:415853"
	if bromus.BasionymID != festucaOvinaNameID {
		t.Errorf("Bromus ovinus.BasionymID = %q, want %q", bromus.BasionymID, festucaOvinaNameID)
	}

	duriuscula, ok := byCanonical["Festuca duriuscula"]
	if !ok {
		t.Fatalf("synonyms of %q = %+v, want an entry for %q", festucaOvinaID, synonyms, "Festuca duriuscula")
	}
	if duriuscula.Homotypic != nil {
		t.Errorf("Festuca duriuscula.Homotypic = %v, want nil (no basionym linkage proves it either way)", duriuscula.Homotypic)
	}
}

// TestIngest_HomotypicRule_SharedBasionym is a synthetic (non-WCVP-fixture)
// case for the homotypic rule's SECOND disjunct — a synonym and the
// accepted name are two DIFFERENT recombinations of the same underlying
// basionym (both basionym ids resolve, and are equal to each other, but
// neither equals the other's own name id directly). The real WCVP fixture
// (see TestIngest_WCVPFixture_HomotypicRule) never happens to exercise this
// specific disjunct, so this test builds the minimal rows by hand via
// fakeRowSource: "acc1" (accepted, basionym "bas1"), "bas1" (a separate
// accepted concept, present purely so its taxonID resolves), and
// "syn-shared" (a synonym of acc1, ALSO basionym "bas1").
func TestIngest_HomotypicRule_SharedBasionym(t *testing.T) {
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "test", Version: "v1"}}, ManifestSHA: "x"}
	repo := openMemoryRepo(t)
	ctx := context.Background()

	taxa := []application.TaxonRow{
		{TaxonID: "acc1", AcceptedTaxonID: "acc1", Accepted: true, Canonical: "Accepted One", Rank: "SPECIES", BasionymTaxonID: "bas1"},
		{TaxonID: "bas1", AcceptedTaxonID: "bas1", Accepted: true, Canonical: "Basionym One", Rank: "SPECIES"},
		{TaxonID: "syn-shared", AcceptedTaxonID: "acc1", Accepted: false, Canonical: "Shared Basionym Synonym", Rank: "SPECIES", BasionymTaxonID: "bas1"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	synonyms := mustSynonyms(ctx, t, repo, "test:concept:acc1")
	if len(synonyms) != 1 || synonyms[0].Canonical != "Shared Basionym Synonym" {
		t.Fatalf("synonyms of test:concept:acc1 = %+v, want exactly [Shared Basionym Synonym]", synonyms)
	}
	if synonyms[0].Homotypic == nil || !*synonyms[0].Homotypic {
		t.Errorf("Shared Basionym Synonym.Homotypic = %v, want a pointer to true (shares basionym %q with the accepted name)", synonyms[0].Homotypic, "bas1")
	}
}

// TestIngest_HomotypicRule_SynonymIsAcceptedNamesBasionym is a synthetic
// case for the homotypic rule's THIRD disjunct — the synonym IS ITSELF the
// accepted name's basionym (the accepted name is a recombination of it),
// the mirror image of the WCVP-fixture-covered "synonym recombines the
// accepted name" case. "acc2" (accepted, basionym "syn-basionym") and
// "syn-basionym" (a synonym of acc2, with no basionym of its own).
func TestIngest_HomotypicRule_SynonymIsAcceptedNamesBasionym(t *testing.T) {
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "test", Version: "v1"}}, ManifestSHA: "x"}
	repo := openMemoryRepo(t)
	ctx := context.Background()

	taxa := []application.TaxonRow{
		{TaxonID: "acc2", AcceptedTaxonID: "acc2", Accepted: true, Canonical: "Accepted Two", Rank: "SPECIES", BasionymTaxonID: "syn-basionym"},
		{TaxonID: "syn-basionym", AcceptedTaxonID: "acc2", Accepted: false, Canonical: "Reverse Basionym Synonym", Rank: "SPECIES"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	synonyms := mustSynonyms(ctx, t, repo, "test:concept:acc2")
	if len(synonyms) != 1 || synonyms[0].Canonical != "Reverse Basionym Synonym" {
		t.Fatalf("synonyms of test:concept:acc2 = %+v, want exactly [Reverse Basionym Synonym]", synonyms)
	}
	if synonyms[0].Homotypic == nil || !*synonyms[0].Homotypic {
		t.Errorf("Reverse Basionym Synonym.Homotypic = %v, want a pointer to true (it IS the accepted name's own basionym)", synonyms[0].Homotypic)
	}
}

func assertCorynephorusConcept(t *testing.T, concept *domain.Concept) {
	t.Helper()
	if concept.AcceptedName.Canonical != "Corynephorus canescens" {
		t.Errorf("concept.AcceptedName.Canonical = %q, want %q", concept.AcceptedName.Canonical, "Corynephorus canescens")
	}
	if concept.Rank != domain.RankSpecies {
		t.Errorf("concept.Rank = %q, want %q", concept.Rank, domain.RankSpecies)
	}
	if concept.Status != domain.StatusAccepted {
		t.Errorf("concept.Status = %q, want %q", concept.Status, domain.StatusAccepted)
	}
	// POWOID ("396681-1") IS the IPNI id (spec §A.1's nomenclatural
	// anchor); Ingest must populate Name.IPNIID from it, not leave it
	// empty just because the powo xref already carries the same value.
	if concept.AcceptedName.IPNIID != "396681-1" {
		t.Errorf("concept.AcceptedName.IPNIID = %q, want %q", concept.AcceptedName.IPNIID, "396681-1")
	}
}

func assertCorynephorusSynonyms(t *testing.T, synonyms []output.SynonymName) {
	t.Helper()
	wantSynonyms := []string{
		"Corynephorus canescens f. pallidus",
		"Corynephorus canescens subsp. maritimus",
		"Corynephorus canescens var. montana",
		"Weingaertneria canescens var. pallida",
	}
	gotSynonyms := make([]string, 0, len(synonyms))
	for _, n := range synonyms {
		gotSynonyms = append(gotSynonyms, n.Canonical)
	}
	sort.Strings(gotSynonyms)
	if !equalStrings(gotSynonyms, wantSynonyms) {
		t.Errorf("synonym canonicals = %v, want %v", gotSynonyms, wantSynonyms)
	}
}

func assertCorynephorusDistribution(t *testing.T, dists []domain.Distribution) {
	t.Helper()
	wantAreas := []string{"AUT", "BLT", "BLR", "BGM", "BRC", "RUC", "CNT", "CZE", "DEN"}
	sort.Strings(wantAreas)
	gotAreas := make([]string, 0, len(dists))
	for _, d := range dists {
		if d.AreaScheme != "wgsrpd_l3" {
			t.Errorf("distribution.AreaScheme = %q, want %q", d.AreaScheme, "wgsrpd_l3")
		}
		gotAreas = append(gotAreas, d.AreaCode)
	}
	sort.Strings(gotAreas)
	if !equalStrings(gotAreas, wantAreas) {
		t.Errorf("distribution area codes = %v, want %v", gotAreas, wantAreas)
	}
}

func TestIngest_WCVPFixture_SynonymDoesNotGetItsOwnConcept(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	// taxonid 543929 (Corynephorus canescens var. montana) is a synonym row
	// with its own POWOID (77271170-1); it must NOT have been given its own
	// concept/xref — it must only be reachable as a synonym name grouped
	// under 405825's accepted concept (asserted in the sibling test above).
	_, err := repo.ConceptByXref(ctx, "powo", "77271170-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConceptByXref(powo, 77271170-1) = %v, want %v (synonyms must not get their own concept/xref)", err, domain.ErrNotFound)
	}
}

// TestIngest_UnknownRankDegradesToOther pins the Hardening Task 1 fix: an
// unparseable "taxonrank" value (whether an arbitrary garbage string or one
// of WCVP's real exotic spellings like "proles" — see
// TestIngest_WCVPExoticRanks_CompletesAndReportsThem below for the latter)
// must NOT abort the ingest anymore. This replaces the old
// TestIngest_UnknownRankFails, which pinned the exact opposite (and now
// wrong) behavior.
func TestIngest_UnknownRankDegradesToOther(t *testing.T) {
	ds := &application.Dataset{
		Backbones:   []application.Backbone{{ID: "bad", Version: "v1"}},
		ManifestSHA: "deadbeef",
	}
	repo := openMemoryRepo(t)
	ctx := context.Background()

	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: []application.TaxonRow{{TaxonID: "1", AcceptedTaxonID: "1", Accepted: true, Canonical: "Bogus", Rank: "NOT-A-RANK"}}}, nil
	}

	report, err := application.Ingest(ctx, ds, readerFor, repo)
	if err != nil {
		t.Fatalf("Ingest: unexpected error for an unparseable rank (must degrade to RankOther, not abort): %v", err)
	}
	if len(report.Backbones) != 1 || report.Backbones[0].OtherRanks != 1 {
		t.Fatalf("report.Backbones = %+v, want exactly one backbone with OtherRanks=1", report.Backbones)
	}

	versions, err := repo.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	found := false
	for _, v := range versions {
		if v.ID == "bad" {
			found = true
		}
	}
	if !found {
		t.Error("BackboneVersions does not contain the successfully-ingested backbone \"bad\"")
	}

	concept := mustConcept(ctx, t, repo, "bad:concept:1")
	if concept.Rank != domain.RankOther {
		t.Errorf("concept.Rank = %q, want %q", concept.Rank, domain.RankOther)
	}
}

// TestIngest_WCVPExoticRanks_CompletesAndReportsThem is the brief's
// required real-shape regression: an ingest whose input contains WCVP's
// "proles" rank (the exact value that made hostus 2.0's full WCVP ingest
// abort after 5.37s on taxon 542377 — see
// docs/research/reality-check.md's M1.0) must complete, not abort, and the
// report must count+surface the exotic ranks rather than silently dropping
// them. The fixture mixes "proles" (count 2, so it must sort first) with
// two exotic ranks tied at count 1 ("grex"/"lusus", which must then break
// the tie alphabetically) and one ordinary "Species" row, so both of
// sortedRankCounts' sort keys (count desc, then verbatim asc) are actually
// exercised, not just the count-desc one.
func TestIngest_WCVPExoticRanks_CompletesAndReportsThem(t *testing.T) {
	ds := &application.Dataset{
		Backbones:   []application.Backbone{{ID: "wcvp-exotic", Version: "v1"}},
		ManifestSHA: "deadbeef",
	}
	repo := openMemoryRepo(t)
	ctx := context.Background()

	taxa := []application.TaxonRow{
		{TaxonID: "1", AcceptedTaxonID: "1", Accepted: true, Canonical: "Ordinary species", Rank: "Species"},
		{TaxonID: "2", AcceptedTaxonID: "2", Accepted: true, Canonical: "Paeonia corallina proles ovatifolia", Rank: "proles"},
		{TaxonID: "3", AcceptedTaxonID: "3", Accepted: true, Canonical: "Some lusus", Rank: "lusus"},
		{TaxonID: "4", AcceptedTaxonID: "4", Accepted: true, Canonical: "Another proles", Rank: "proles"},
		{TaxonID: "5", AcceptedTaxonID: "5", Accepted: true, Canonical: "Some grex", Rank: "grex"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}

	report, err := application.Ingest(ctx, ds, readerFor, repo)
	if err != nil {
		t.Fatalf("Ingest: expected completion despite exotic ranks (proles/lusus/grex), got error: %v", err)
	}
	if len(report.Backbones) != 1 {
		t.Fatalf("len(report.Backbones) = %d, want 1", len(report.Backbones))
	}
	b := report.Backbones[0]
	if b.Names != 5 {
		t.Errorf("b.Names = %d, want 5 (every row still gets a Name, exotic rank or not)", b.Names)
	}
	if b.OtherRanks != 4 {
		t.Errorf("b.OtherRanks = %d, want 4 (two proles + one lusus + one grex)", b.OtherRanks)
	}
	// "grex" sorts before "lusus" alphabetically once their counts tie at 1.
	wantSample := []application.RankVerbatimCount{
		{Verbatim: "proles", Count: 2},
		{Verbatim: "grex", Count: 1},
		{Verbatim: "lusus", Count: 1},
	}
	if len(b.OtherRankSample) != len(wantSample) {
		t.Fatalf("b.OtherRankSample = %+v, want %+v", b.OtherRankSample, wantSample)
	}
	for i, want := range wantSample {
		if b.OtherRankSample[i] != want {
			t.Errorf("b.OtherRankSample[%d] = %+v, want %+v (most frequent first, ties broken alphabetically)", i, b.OtherRankSample[i], want)
		}
	}

	proles := mustConcept(ctx, t, repo, "wcvp-exotic:concept:2")
	if proles.Rank != domain.RankOther {
		t.Errorf("proles concept.Rank = %q, want %q", proles.Rank, domain.RankOther)
	}
	// Fix round 1: rank_verbatim now round-trips through the real sqlite
	// adapter, not just the in-process domain.Name — proving a nomenclature
	// service doesn't forget which exotic rank a concept actually had the
	// moment the ingest process exits (spec §A.1).
	if proles.RankVerbatim != "proles" {
		t.Errorf("proles concept.RankVerbatim = %q, want %q (round-tripped through sqlite)", proles.RankVerbatim, "proles")
	}
	if proles.AcceptedName.RankVerbatim != "proles" {
		t.Errorf("proles concept.AcceptedName.RankVerbatim = %q, want %q (round-tripped through sqlite)", proles.AcceptedName.RankVerbatim, "proles")
	}

	ordinary := mustConcept(ctx, t, repo, "wcvp-exotic:concept:1")
	if ordinary.Rank != domain.RankSpecies {
		t.Errorf("ordinary concept.Rank = %q, want %q", ordinary.Rank, domain.RankSpecies)
	}
	if ordinary.RankVerbatim != "" {
		t.Errorf("ordinary concept.RankVerbatim = %q, want empty (Rank alone already identifies it)", ordinary.RankVerbatim)
	}
}

// TestIngest_OtherRank_PopulatesNameRankVerbatim proves the OTHER half of
// the "verbatim source string must be preserved" requirement: pass 1 sets
// domain.Name.RankVerbatim to the raw source spelling for every row that
// normalizes to domain.RankOther (and leaves it empty otherwise — Rank
// alone already identifies a canonical rank exactly, so there is nothing
// to preserve there). It exercises the fakeCapturingRepo test double
// (below) — a minimal in-memory output.Repository/IngestTx, rather than
// the real sqlite adapter — so this test stays a narrow, fast check of
// exactly what Ingest hands to UpsertName, independent of the sqlite
// adapter's own read/write plumbing (which
// TestIngest_WCVPExoticRanks_CompletesAndReportsThem above, and
// internal/adapters/sqlite's own tests, cover separately): the
// information must survive from TaxonRow through to the domain.Name
// Ingest hands the repository, which is the boundary this task owns.
func TestIngest_OtherRank_PopulatesNameRankVerbatim(t *testing.T) {
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp-exotic", Version: "v1"}}, ManifestSHA: "x"}
	repo := &fakeCapturingRepo{}
	ctx := context.Background()

	taxa := []application.TaxonRow{
		{TaxonID: "1", AcceptedTaxonID: "1", Accepted: true, Canonical: "Ordinary species", Rank: "Species"},
		{TaxonID: "2", AcceptedTaxonID: "2", Accepted: true, Canonical: "Paeonia corallina proles ovatifolia", Rank: "proles"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}

	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	names := repo.tx.namesByID()
	proles, ok := names["wcvp-exotic:name:2"]
	if !ok {
		t.Fatalf("no Name captured for taxon 2")
	}
	if proles.Rank != domain.RankOther {
		t.Errorf("proles Name.Rank = %q, want %q", proles.Rank, domain.RankOther)
	}
	if proles.RankVerbatim != "proles" {
		t.Errorf("proles Name.RankVerbatim = %q, want %q (preserved, readable)", proles.RankVerbatim, "proles")
	}

	ordinary, ok := names["wcvp-exotic:name:1"]
	if !ok {
		t.Fatalf("no Name captured for taxon 1")
	}
	if ordinary.Rank != domain.RankSpecies {
		t.Errorf("ordinary Name.Rank = %q, want %q", ordinary.Rank, domain.RankSpecies)
	}
	if ordinary.RankVerbatim != "" {
		t.Errorf("ordinary Name.RankVerbatim = %q, want empty (Rank alone already identifies it)", ordinary.RankVerbatim)
	}
}

func TestIngest_ReaderErrorPropagates(t *testing.T) {
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}}
	repo := openMemoryRepo(t)
	wantErr := errors.New("boom")
	readerFor := func(application.Backbone) (application.RowSource, error) { return nil, wantErr }

	_, err := application.Ingest(context.Background(), ds, readerFor, repo)
	if !errors.Is(err, wantErr) {
		t.Errorf("Ingest error = %v, want it to wrap %v", err, wantErr)
	}
}

type fakeRowSource struct {
	taxa  []application.TaxonRow
	dists []application.DistributionRow
}

func (f fakeRowSource) Taxa() []application.TaxonRow                 { return f.taxa }
func (f fakeRowSource) Distributions() []application.DistributionRow { return f.dists }

var _ output.Repository = (*sqlite.DB)(nil)

// fakeCapturingRepo is a minimal output.Repository test double whose only
// meaningfully-implemented method is BeginIngest — the only Repository
// method application.Ingest itself calls (everything downstream goes
// through the returned IngestTx). It exists so
// TestIngest_OtherRank_PopulatesNameRankVerbatim can inspect the exact
// domain.Name Ingest hands to UpsertName without needing the real sqlite
// adapter's schema to carry a field (RankVerbatim) this task deliberately
// does not persist there.
type fakeCapturingRepo struct {
	tx fakeCapturingTx
}

func (f *fakeCapturingRepo) BeginIngest(context.Context, domain.BackboneVersion) (output.IngestTx, error) {
	return &f.tx, nil
}

func (f *fakeCapturingRepo) Concept(context.Context, string) (*domain.Concept, []output.SynonymName, []domain.Xref, []domain.Distribution, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) Classification(context.Context, string) ([]domain.ClassificationEntry, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) ConceptByXref(context.Context, string, string) (*domain.Concept, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) ConceptIDsByXref(context.Context, string, []string) (map[string]string, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) MatchExact(context.Context, string) ([]output.MatchCandidate, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) MatchFuzzyCandidates(context.Context, string, int) ([]output.MatchCandidate, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) BackboneVersions(context.Context) ([]domain.BackboneVersion, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) Traits(context.Context, string, []domain.TraitVocab) ([]domain.TraitSet, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) TraitVocabularies(context.Context) ([]domain.TraitVocabMeta, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) Suggest(context.Context, string, output.SuggestOpts) ([]domain.SuggestItem, error) {
	panic("not needed by Ingest")
}
func (f *fakeCapturingRepo) BeginTraitIngest(context.Context) (output.IngestTx, error) {
	panic("not needed by Ingest")
}

// fakeCapturingTx is the IngestTx fakeCapturingRepo hands out: it records
// every Name passed to UpsertName (keyed by id, last write wins — matching
// pass 1's "write once, sub-pass 1b may re-write with linkage filled in"
// pattern) and no-ops everything else Ingest calls along the way.
type fakeCapturingTx struct {
	names map[string]domain.Name
}

func (t *fakeCapturingTx) namesByID() map[string]domain.Name { return t.names }

func (t *fakeCapturingTx) UpsertName(n domain.Name) error {
	if t.names == nil {
		t.names = make(map[string]domain.Name)
	}
	t.names[n.ID] = n
	return nil
}
func (t *fakeCapturingTx) UpsertConcept(domain.Concept) error                { return nil }
func (t *fakeCapturingTx) LinkName(string, string, string, *bool) error      { return nil }
func (t *fakeCapturingTx) AddXref(string, domain.Xref) error                 { return nil }
func (t *fakeCapturingTx) AddDistribution(string, domain.Distribution) error { return nil }
func (t *fakeCapturingTx) AddTraitValue(string, domain.TraitValue) error     { return nil }
func (t *fakeCapturingTx) UpsertTraitVocabulary(domain.TraitVocabMeta) error { return nil }
func (t *fakeCapturingTx) Finalize() error                                   { return nil }
func (t *fakeCapturingTx) Commit() error                                     { return nil }
func (t *fakeCapturingTx) Rollback() error                                   { return nil }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
