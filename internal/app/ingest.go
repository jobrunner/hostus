package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jobrunner/hostus/internal/adapters/cdm"
	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/namelist"
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
			PublishedIn:     t.PublishedIn,
			NomStatus:       t.NomenclaturalStatus,
		})
	}
	return out
}

func (s wcvpRowSource) Distributions() []application.DistributionRow {
	out := make([]application.DistributionRow, 0, len(s.ds.Distributions))
	for _, d := range s.ds.Distributions {
		out = append(out, application.DistributionRow{TaxonID: d.CoreID, AreaCode: d.AreaCode(), AreaName: d.Locality})
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

// nameSpaceRowSource adapts a *namelist.Dataset into
// application.NameRowSource, so application never imports
// internal/adapters/namelist directly (depguard) — the name-space
// counterpart of wcvpRowSource/traitVocabRowSource/xrefSourceRowSource above.
type nameSpaceRowSource struct{ ds *namelist.Dataset }

func (s nameSpaceRowSource) Rows() []application.NameRow {
	out := make([]application.NameRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.NameRow{Taxon: r.Taxon, SourceID: r.SourceID})
	}
	return out
}

// ingestNameSpace opens ns's canonical name-list CSV and runs
// application.IngestNameSpace against repo (SP9/UC4). manifestSHA is recorded
// onto the space's name_space row exactly as it is onto xref_source.
//
// Reader-level row errors are surfaced on the report rather than aborting,
// like ingestConceptSource: one malformed line out of 16.402 must not cost
// the whole ingest, but it must not vanish either.
func ingestNameSpace(ctx context.Context, ns manifest.NameSpace, manifestSHA string, repo *sqlite.DB) (application.NameSpaceIngestReport, error) {
	ds, err := namelist.Read(ns.Path)
	if err != nil {
		return application.NameSpaceIngestReport{}, fmt.Errorf("app: reading name space %q at %q: %w", ns.ID, ns.Path, err)
	}
	// ns.Redistribution is routed through ParseRedistribution for the same
	// reason as adaptBackbones' backbone mapping above — see its doc comment.
	redistribution, err := domain.ParseRedistribution(ns.Redistribution)
	if err != nil {
		return application.NameSpaceIngestReport{}, fmt.Errorf("app: name space %q: %w", ns.ID, err)
	}
	meta := domain.NameSpaceMeta{
		ID:             ns.ID,
		Version:        ns.Version,
		License:        ns.License,
		SourceURL:      ns.SourceURL,
		ManifestSHA:    manifestSHA,
		Redistribution: redistribution,
	}
	report, err := application.IngestNameSpace(ctx, repo, nameSpaceRowSource{ds: ds}, meta)
	report.ReaderErrors = len(ds.Errors)
	return report, err
}

// ingestConceptSource reads cs's two canonical CDM CSVs and runs
// application.IngestCDM against repo. This is the adapter -> application DTO
// bridge for SP5: internal/application must not import
// internal/adapters/cdm (depguard), so the row mapping lives here in the
// composition root, exactly like wcvpRowSource/traitVocabRowSource above.
//
// Reader-level row errors are surfaced on the report rather than aborting:
// the CSVs are the output of a 16–20 h crawl, and one malformed line out of
// 51.466 must not cost the whole ingest. An unmapped RELATION type is the
// opposite case and does abort — see application.IngestCDM.
func ingestConceptSource(ctx context.Context, cs manifest.ConceptSource, manifestSHA string, repo *sqlite.DB) (application.CDMIngestReport, error) {
	conceptsDS, err := cdm.ReadConcepts(cs.Concepts)
	if err != nil {
		return application.CDMIngestReport{}, fmt.Errorf("app: reading concept source %q at %q: %w", cs.ID, cs.Concepts, err)
	}
	relationsDS, err := cdm.ReadRelations(cs.Relations)
	if err != nil {
		return application.CDMIngestReport{}, fmt.Errorf("app: reading concept source %q at %q: %w", cs.ID, cs.Relations, err)
	}
	// cs.Redistribution is routed through ParseRedistribution for the same
	// reason as adaptBackbones' backbone mapping above — see its doc comment.
	redistribution, err := domain.ParseRedistribution(cs.Redistribution)
	if err != nil {
		return application.CDMIngestReport{}, fmt.Errorf("app: concept source %q: %w", cs.ID, err)
	}
	meta := domain.BackboneVersion{
		ID:             cs.ID,
		Version:        cs.Version,
		License:        cs.License,
		SourceURL:      cs.SourceURL,
		IngestedAt:     time.Now().UTC().Format(time.RFC3339),
		ManifestSHA:    manifestSHA,
		Redistribution: redistribution,
	}
	report, err := application.IngestCDM(ctx, repo, adaptCDMConcepts(conceptsDS.Rows), adaptCDMRelations(relationsDS.Rows), meta)
	report.ReaderErrors = len(conceptsDS.Errors) + len(relationsDS.Errors)
	return report, err
}

func adaptCDMConcepts(rows []cdm.ConceptRow) []application.CDMConceptRow {
	out := make([]application.CDMConceptRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.CDMConceptRow{
			ConceptUUID:        r.ConceptUUID,
			ScientificName:     r.ScientificName,
			Authorship:         r.Authorship,
			Rank:               r.Rank,
			Status:             r.Status,
			SecUUID:            r.SecUUID,
			SecTitle:           r.SecTitle,
			ClassificationUUID: r.ClassificationUUID,
			ParentUUID:         r.ParentUUID,
		})
	}
	return out
}

func adaptCDMRelations(rows []cdm.RelationRow) []application.CDMRelationRow {
	out := make([]application.CDMRelationRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.CDMRelationRow{
			FromUUID:          r.FromUUID,
			ToUUID:            r.ToUUID,
			RelationType:      r.RelationType,
			RelationSymbol:    r.RelationSymbol,
			IsConceptRelation: r.IsConceptRelation,
			RelationshipUUID:  r.RelationshipUUID,
		})
	}
	return out
}

// Reports bundles everything one "hostus ingest" run produced. It replaced a
// four-value return when SP5 added a fourth kind of source: past four
// results a positional tuple stops documenting itself, and every caller was
// already discarding most of it with blank identifiers.
type Reports struct {
	Backbone       application.IngestReport
	Traits         []application.TraitIngestReport
	Xrefs          []application.XrefIngestReport
	ConceptSources []application.CDMIngestReport
	NameSpaces     []application.NameSpaceIngestReport
}

// Ingest parses and validates the manifest at manifestPath, opens (or
// creates) the SQLite database at dbPath, and runs application.Ingest
// against every pinned backbone, then application.IngestTraits against every
// pinned trait vocabulary, then application.IngestXrefs against every pinned
// xref source, then application.IngestCDM against every pinned concept
// source, then application.IngestNameSpace against every pinned name space.
// It is the entry point "hostus ingest" calls.
//
// Concept sources run LATE on purpose: their relation ends resolve against
// taxon_concept, so anything an earlier phase wrote is already available to
// them (see application.IngestCDM's phase 1). Name spaces run LAST for the
// same reason — their crosswalk resolves against the name index, so every
// concept any earlier phase contributed is a possible target.
func Ingest(ctx context.Context, manifestPath, dbPath string) (Reports, error) {
	var reports Reports

	manifestDS, err := manifest.Parse(manifestPath)
	if err != nil {
		return reports, err
	}
	backbones, err := adaptBackbones(manifestDS.Backbones)
	if err != nil {
		return reports, err
	}
	ds := &application.Dataset{Backbones: backbones, ManifestSHA: manifestDS.ManifestSHA}

	repo, err := sqlite.Open(dbPath)
	if err != nil {
		return reports, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = repo.Close() }()

	reports.Backbone, err = application.Ingest(ctx, ds, readerFor, repo)
	if err != nil {
		return reports, err
	}

	reports.Traits = make([]application.TraitIngestReport, 0, len(manifestDS.TraitVocabularies))
	for _, tv := range manifestDS.TraitVocabularies {
		tr, err := ingestTraitVocab(ctx, tv, repo)
		if err != nil {
			return reports, err
		}
		reports.Traits = append(reports.Traits, tr)
	}

	reports.Xrefs = make([]application.XrefIngestReport, 0, len(manifestDS.XrefSources))
	for _, xs := range manifestDS.XrefSources {
		xr, err := ingestXrefSource(ctx, xs, manifestDS.ManifestSHA, repo)
		if err != nil {
			return reports, err
		}
		reports.Xrefs = append(reports.Xrefs, xr)
	}

	reports.ConceptSources = make([]application.CDMIngestReport, 0, len(manifestDS.ConceptSources))
	for _, cs := range manifestDS.ConceptSources {
		cr, err := ingestConceptSource(ctx, cs, manifestDS.ManifestSHA, repo)
		if err != nil {
			return reports, err
		}
		reports.ConceptSources = append(reports.ConceptSources, cr)
	}

	reports.NameSpaces = make([]application.NameSpaceIngestReport, 0, len(manifestDS.NameSpaces))
	for _, ns := range manifestDS.NameSpaces {
		nr, err := ingestNameSpace(ctx, ns, manifestDS.ManifestSHA, repo)
		if err != nil {
			return reports, err
		}
		reports.NameSpaces = append(reports.NameSpaces, nr)
	}

	// BuildDistributionClosure runs once ALL backbones (incl. CDM) are
	// ingested — it resolves CDM concepts' in_area name fallback against WCVP
	// twins, which must already be present by this point.
	if err := repo.BuildDistributionClosure(ctx); err != nil {
		return reports, fmt.Errorf("app: building distribution closure: %w", err)
	}

	return reports, nil
}
