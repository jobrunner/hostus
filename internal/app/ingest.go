package app

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/traits"
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// adaptBackbones maps the manifest's backbone entries onto the
// application.Backbone DTO application.Ingest expects. application never
// imports internal/adapters/manifest (depguard), so this mapping lives here
// in the composition root — the one place allowed to know about both.
func adaptBackbones(bs []manifest.Backbone) []application.Backbone {
	out := make([]application.Backbone, 0, len(bs))
	for _, b := range bs {
		out = append(out, application.Backbone{
			ID:             b.ID,
			Version:        b.Version,
			License:        b.License,
			SourceURL:      b.SourceURL,
			Path:           b.Path,
			Redistribution: b.Redistribution,
		})
	}
	return out
}

// wcvpRowSource adapts a *wcvp.Dataset (T4's reader output) into
// application.RowSource, so application never imports internal/adapters/wcvp
// directly (depguard). It mirrors the identical bridge duplicated in
// internal/application/ingest_test.go and internal/adapters/http/taxa_test.go
// for their own boundary-respecting test fixtures; this is the one copy
// production code actually calls.
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

// readerFor opens b's local directory as a WCVP DwC-A bundle and adapts it
// into an application.RowSource. SP1 only ships the WCVP reader (T4); every
// backbone entry in the manifest is read through it regardless of ID, the
// same way T5/T7's own tests exercise Ingest. A per-ID reader dispatch (COL
// XR, Euro+Med, FloraVeg.EU) is future work once those readers land.
func readerFor(b application.Backbone) (application.RowSource, error) {
	ds, err := wcvp.Read(b.Path)
	if err != nil {
		return nil, fmt.Errorf("app: reading backbone %q at %q: %w", b.ID, b.Path, err)
	}
	return wcvpRowSource{ds: ds}, nil
}

// traitVocabRowSource adapts a *traits.Dataset (T2's reader output) into
// application.TraitRowSource, so application never imports
// internal/adapters/traits directly (depguard) — the trait-vocabulary
// counterpart of wcvpRowSource above.
type traitVocabRowSource struct{ ds *traits.Dataset }

func (s traitVocabRowSource) Rows() []application.TraitRow {
	out := make([]application.TraitRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.TraitRow{
			Taxon:        r.Taxon,
			Vocab:        r.Vocab,
			VocabVersion: r.VocabVersion,
			Dim:          r.Dim,
			Value:        r.Value,
			NicheWidth:   r.NicheWidth,
			NSystems:     r.NSystems,
		})
	}
	return out
}

// ingestTraitVocab opens tv's canonical trait CSV and runs
// application.IngestTraits against repo, adapting the manifest's
// TraitVocabulary entry into the domain.TraitVocabMeta the use case records.
func ingestTraitVocab(ctx context.Context, tv manifest.TraitVocabulary, repo *sqlite.DB) (application.TraitIngestReport, error) {
	ds, err := traits.Read(tv.Path)
	if err != nil {
		return application.TraitIngestReport{}, fmt.Errorf("app: reading trait vocabulary %q at %q: %w", tv.ID, tv.Path, err)
	}
	vocab, err := domain.ParseTraitVocab(tv.ID)
	if err != nil {
		return application.TraitIngestReport{}, fmt.Errorf("app: trait vocabulary %q: %w", tv.ID, err)
	}
	meta := domain.TraitVocabMeta{
		Vocab:          vocab,
		Version:        tv.Version,
		Taxonomy:       tv.Taxonomy,
		License:        tv.License,
		SourceURL:      tv.SourceURL,
		Redistribution: domain.Redistribution(tv.Redistribution),
	}
	return application.IngestTraits(ctx, repo, traitVocabRowSource{ds: ds}, meta)
}

// Ingest parses and validates the manifest at manifestPath, opens (or
// creates) the SQLite database at dbPath, and runs application.Ingest
// against every pinned backbone followed by application.IngestTraits
// against every pinned trait vocabulary, returning both reports. It is the
// entry point "hostus ingest" calls.
func Ingest(ctx context.Context, manifestPath, dbPath string) (application.IngestReport, []application.TraitIngestReport, error) {
	manifestDS, err := manifest.Parse(manifestPath)
	if err != nil {
		return application.IngestReport{}, nil, err
	}
	ds := &application.Dataset{Backbones: adaptBackbones(manifestDS.Backbones), ManifestSHA: manifestDS.ManifestSHA}

	repo, err := sqlite.Open(dbPath)
	if err != nil {
		return application.IngestReport{}, nil, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = repo.Close() }()

	backboneReport, err := application.Ingest(ctx, ds, readerFor, repo)
	if err != nil {
		return backboneReport, nil, err
	}

	traitReports := make([]application.TraitIngestReport, 0, len(manifestDS.TraitVocabularies))
	for _, tv := range manifestDS.TraitVocabularies {
		tr, err := ingestTraitVocab(ctx, tv, repo)
		if err != nil {
			return backboneReport, traitReports, err
		}
		traitReports = append(traitReports, tr)
	}

	return backboneReport, traitReports, nil
}
