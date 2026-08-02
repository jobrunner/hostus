package app

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/traits"
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/adapters/xref"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// adaptBackbones maps the manifest's backbone entries onto the
// application.Backbone DTO application.Ingest expects. application never
// imports internal/adapters/manifest (depguard), so this mapping lives here
// in the composition root — the one place allowed to know about both.
//
// b.Redistribution is routed through domain.ParseRedistribution rather than
// cast directly: the JSON-schema enum (dataset.schema.json) and the
// domain.Redistribution constants are two independently maintained sources
// of truth for the same three values, and nothing else in the ingest path
// checks they stay in lockstep. A raw string(...) cast would silently carry
// a schema/constant drift (or a manifest that somehow bypassed schema
// validation) all the way into backbone_version — ParseRedistribution fails
// loudly here instead, at the one composition-root boundary that knows
// about both the manifest and the domain type.
func adaptBackbones(bs []manifest.Backbone) ([]application.Backbone, error) {
	out := make([]application.Backbone, 0, len(bs))
	for _, b := range bs {
		redistribution, err := domain.ParseRedistribution(b.Redistribution)
		if err != nil {
			return nil, fmt.Errorf("app: backbone %q: %w", b.ID, err)
		}
		out = append(out, application.Backbone{
			ID:             b.ID,
			Version:        b.Version,
			License:        b.License,
			SourceURL:      b.SourceURL,
			Path:           b.Path,
			Redistribution: string(redistribution),
		})
	}
	return out, nil
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
	// tv.Redistribution is routed through ParseRedistribution for the same
	// reason as adaptBackbones' backbone mapping above — see its doc
	// comment.
	redistribution, err := domain.ParseRedistribution(tv.Redistribution)
	if err != nil {
		return application.TraitIngestReport{}, fmt.Errorf("app: trait vocabulary %q: %w", tv.ID, err)
	}
	meta := domain.TraitVocabMeta{
		Vocab:          vocab,
		Version:        tv.Version,
		Taxonomy:       tv.Taxonomy,
		License:        tv.License,
		SourceURL:      tv.SourceURL,
		Redistribution: redistribution,
	}
	return application.IngestTraits(ctx, repo, traitVocabRowSource{ds: ds}, meta)
}

// xrefSourceRowSource adapts a *xref.Dataset (T1/T2's reader output) into
// application.XrefRowSource, so application never imports
// internal/adapters/xref directly (depguard) — the xref-source counterpart
// of wcvpRowSource/traitVocabRowSource above.
type xrefSourceRowSource struct{ ds *xref.Dataset }

func (s xrefSourceRowSource) Rows() []application.XrefRow {
	out := make([]application.XrefRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.XrefRow{
			JoinAuthority: r.JoinAuthority,
			JoinID:        r.JoinID,
			Authority:     r.Authority,
			ExtID:         r.ExtID,
		})
	}
	return out
}

// ingestXrefSource opens xs's canonical xref CSV and runs
// application.IngestXrefs against repo, adapting the manifest's XrefSource
// entry into the domain.XrefSourceMeta the use case records. manifestSHA is
// the checksum of the validated manifest xs was pinned by — recorded onto
// the source's xref_source row exactly as it is onto backbone_version, so an
// ingested database can say which harvest its xrefs came from.
func ingestXrefSource(ctx context.Context, xs manifest.XrefSource, manifestSHA string, repo *sqlite.DB) (application.XrefIngestReport, error) {
	ds, err := xref.Read(xs.Path)
	if err != nil {
		return application.XrefIngestReport{}, fmt.Errorf("app: reading xref source %q at %q: %w", xs.ID, xs.Path, err)
	}
	// xs.Redistribution is routed through ParseRedistribution for the same
	// reason as adaptBackbones' backbone mapping above — see its doc
	// comment.
	redistribution, err := domain.ParseRedistribution(xs.Redistribution)
	if err != nil {
		return application.XrefIngestReport{}, fmt.Errorf("app: xref source %q: %w", xs.ID, err)
	}
	meta := domain.XrefSourceMeta{
		ID:             xs.ID,
		Version:        xs.Version,
		License:        xs.License,
		SourceURL:      xs.SourceURL,
		ManifestSHA:    manifestSHA,
		Redistribution: redistribution,
	}
	return application.IngestXrefs(ctx, repo, xrefSourceRowSource{ds: ds}, meta)
}

// Ingest parses and validates the manifest at manifestPath, opens (or
// creates) the SQLite database at dbPath, and runs application.Ingest
// against every pinned backbone, followed by application.IngestTraits
// against every pinned trait vocabulary, followed by application.IngestXrefs
// against every pinned xref source, returning all three reports. It is the
// entry point "hostus ingest" calls.
func Ingest(ctx context.Context, manifestPath, dbPath string) (application.IngestReport, []application.TraitIngestReport, []application.XrefIngestReport, error) {
	manifestDS, err := manifest.Parse(manifestPath)
	if err != nil {
		return application.IngestReport{}, nil, nil, err
	}
	backbones, err := adaptBackbones(manifestDS.Backbones)
	if err != nil {
		return application.IngestReport{}, nil, nil, err
	}
	ds := &application.Dataset{Backbones: backbones, ManifestSHA: manifestDS.ManifestSHA}

	repo, err := sqlite.Open(dbPath)
	if err != nil {
		return application.IngestReport{}, nil, nil, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = repo.Close() }()

	backboneReport, err := application.Ingest(ctx, ds, readerFor, repo)
	if err != nil {
		return backboneReport, nil, nil, err
	}

	traitReports := make([]application.TraitIngestReport, 0, len(manifestDS.TraitVocabularies))
	for _, tv := range manifestDS.TraitVocabularies {
		tr, err := ingestTraitVocab(ctx, tv, repo)
		if err != nil {
			return backboneReport, traitReports, nil, err
		}
		traitReports = append(traitReports, tr)
	}

	xrefReports := make([]application.XrefIngestReport, 0, len(manifestDS.XrefSources))
	for _, xs := range manifestDS.XrefSources {
		xr, err := ingestXrefSource(ctx, xs, manifestDS.ManifestSHA, repo)
		if err != nil {
			return backboneReport, traitReports, xrefReports, err
		}
		xrefReports = append(xrefReports, xr)
	}

	return backboneReport, traitReports, xrefReports, nil
}
