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

func assertCorynephorusSynonyms(t *testing.T, synonyms []domain.Name) {
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

func TestIngest_UnknownRankFails(t *testing.T) {
	ds := &application.Dataset{
		Backbones:   []application.Backbone{{ID: "bad", Version: "v1"}},
		ManifestSHA: "deadbeef",
	}
	repo := openMemoryRepo(t)
	ctx := context.Background()

	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: []application.TaxonRow{{TaxonID: "1", AcceptedTaxonID: "1", Accepted: true, Canonical: "Bogus", Rank: "NOT-A-RANK"}}}, nil
	}

	if _, err := application.Ingest(ctx, ds, readerFor, repo); err == nil {
		t.Fatal("Ingest: expected error for an unparseable rank, got nil")
	}

	versions, err := repo.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	for _, v := range versions {
		if v.ID == "bad" {
			t.Error("BackboneVersions contains the failed backbone; a rolled-back ingest must not leave a partial backbone_version record readable")
		}
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
