package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jobrunner/hostus/internal/adapters/cdm"
	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/namelist"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
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

// xrefSourceRowSource adapts a *xref.Dataset (T1/T2's reader output) into
// application.XrefRowSource, so application never imports
// internal/adapters/xref directly (depguard) — the xref-source counterpart
// of wcvpRowSource above.
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
// counterpart of wcvpRowSource/xrefSourceRowSource above.
type nameSpaceRowSource struct{ ds *namelist.Dataset }

func (s nameSpaceRowSource) Rows() []application.NameRow {
	byID := make(map[string]namelist.Row, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		byID[r.SourceID] = r
	}

	out := make([]application.NameRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		family, order, class := classificationFor(r, byID)
		out = append(out, application.NameRow{
			Taxon:        r.Taxon,
			SourceID:     r.SourceID,
			Status:       r.Status,
			Family:       family,
			OrderName:    order,
			ClassName:    class,
			VernacularDE: r.VernacularDE,
		})
	}
	return out
}

// classificationFor walks r's parent chain (via namelist.Row.ParentID,
// resolved through byID — the full row set of the SAME source, both
// Fall-A species rows and Fall-B higher-rank rows) upward until it has
// found the nearest FAMILY/ORDER/CLASS ancestor for each, or runs out of
// parents. It stops after a bounded number of hops (guards against a
// malformed cyclic parent chain in the raw data — never trust bulk pipeline
// data to be well-formed, same posture as namelist's own row-level error
// handling) rather than looping forever.
func classificationFor(r namelist.Row, byID map[string]namelist.Row) (family, order, class string) {
	const maxHops = 20
	current := r
	for hops := 0; hops < maxHops && current.ParentID != ""; hops++ {
		parent, ok := byID[current.ParentID]
		if !ok {
			break
		}
		// switch true (not `switch rank`) deliberately: a switch typed on
		// domain.Rank would have to exhaust every one of its ~30 rank
		// constants (exhaustive linter) even though every rank besides
		// these three just means "keep walking upward" — a bool switch
		// says that without one line per uninteresting rank.
		rank, _ := domain.ParseRankLenient(parent.Rank)
		// Hoisted for the same coverage reason as internal/domain/synonym.go's
		// switch: a condition written inside a case arm sits in no counted
		// block in Go's coverage model, so `make mutation` reports it as
		// NOT COVERED regardless of how thoroughly the branch is tested. As
		// plain assignments they are covered, mutated and killed.
		isFamily := rank == domain.RankFamily && family == ""
		isOrder := rank == domain.RankOrder && order == ""
		isClass := rank == domain.RankClass && class == ""
		switch {
		case isFamily:
			family = parent.Taxon
		case isOrder:
			order = parent.Taxon
		case isClass:
			class = parent.Taxon
		}
		current = parent
	}
	return family, order, class
}

// ingestNameSpaceDataset runs application.IngestNameSpace against repo
// (SP9/UC4) for the ALREADY-READ ds. manifestSHA is recorded onto the
// space's name_space row exactly as it is onto xref_source.
//
// It takes ds rather than reading ns.Path itself because Ingest()'s
// composition-root loop reads ns's CSV exactly ONCE per name space and
// passes the resulting *namelist.Dataset to both this function (Fall A) and
// ingestNativeSpaceDataset (Fall B, for eurosl/germansl) — see Ingest()'s
// NameSpaces loop.
//
// Reader-level row errors are surfaced on the report rather than aborting,
// like ingestConceptSource: one malformed line out of 16.402 must not cost
// the whole ingest, but it must not vanish either.
func ingestNameSpaceDataset(ctx context.Context, ds *namelist.Dataset, ns manifest.NameSpace, manifestSHA string, repo *sqlite.DB) (application.NameSpaceIngestReport, error) {
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

// nativeRowSource adapts a *namelist.Dataset into
// application.NativeRowSource, the Fall-B counterpart of
// nameSpaceRowSource above — application never imports
// internal/adapters/namelist directly (depguard).
type nativeRowSource struct{ ds *namelist.Dataset }

func (s nativeRowSource) Rows() []application.NativeRow {
	out := make([]application.NativeRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.NativeRow{
			Taxon:    r.Taxon,
			SourceID: r.SourceID,
			Rank:     r.Rank,
			Status:   r.Status,
			ParentID: r.ParentID,
		})
	}
	return out
}

// ingestNativeSpace opens ns's canonical name-list CSV and runs
// application.IngestNativeSpace against repo (Fall B — EuroSL/GermanSL
// ranks above SPECIES, plus aggregates). It reuses the same CSV reader as
// ingestNameSpace (Fall A): the two use cases differ only in what they DO
// with each row, not in what they read.
//
// Unlike ingestNameSpace, this builds a domain.BackboneVersion, not a
// domain.NameSpaceMeta: Fall B writes its own taxon_concept rows via
// repo.BeginIngest, the backbone path, not the name-space path — see the
// plan's "Architecture" section head.
func ingestNativeSpace(ctx context.Context, ns manifest.NameSpace, manifestSHA string, repo *sqlite.DB, minRank domain.Rank) (application.NativeSpaceIngestReport, error) {
	ds, err := namelist.Read(ns.Path)
	if err != nil {
		return application.NativeSpaceIngestReport{}, fmt.Errorf("app: reading native space %q at %q: %w", ns.ID, ns.Path, err)
	}
	return ingestNativeSpaceDataset(ctx, ds, ns, manifestSHA, repo, minRank)
}

// ingestNativeSpaceDataset is ingestNativeSpace's logic split out from the
// CSV read, mirroring ingestNameSpaceDataset above — Ingest()'s composition-
// root loop reads ns's CSV once and passes the same *namelist.Dataset to
// both the Fall-A and Fall-B bridge. memberLinks is derived from ds via
// nativeMemberLinks below, rather than passed in, so this function's only
// two inputs are "the row set" and "which space/minRank" — matching
// ingestNameSpaceDataset's shape.
func ingestNativeSpaceDataset(ctx context.Context, ds *namelist.Dataset, ns manifest.NameSpace, manifestSHA string, repo *sqlite.DB, minRank domain.Rank) (application.NativeSpaceIngestReport, error) {
	// ns.Redistribution is routed through ParseRedistribution for the same
	// reason as adaptBackbones' backbone mapping above — see its doc comment.
	redistribution, err := domain.ParseRedistribution(ns.Redistribution)
	if err != nil {
		return application.NativeSpaceIngestReport{}, fmt.Errorf("app: native space %q: %w", ns.ID, err)
	}
	bv := domain.BackboneVersion{
		ID:             ns.ID,
		Version:        ns.Version,
		License:        ns.License,
		SourceURL:      ns.SourceURL,
		ManifestSHA:    manifestSHA,
		Redistribution: redistribution,
	}
	return application.IngestNativeSpace(ctx, repo, nativeRowSource{ds: ds}, bv, minRank, nativeMemberLinks(ds))
}

// nativeMemberLinks derives Task 6's aggregate->member wiring from ds's
// full row set: every row with a ParentID becomes a member entry under its
// parent's SourceID, regardless of either row's own rank.
// IngestNativeSpace's own write loop only consults memberLinks for a row it
// has ALREADY confirmed qualifies as a Fall-B concept (see
// qualifiesAsFallBConcept), and ResolveNameSpaceMember safely returns ""
// (not an error) for a memberSourceID that Fall A never crosswalked — so no
// rank-filtering is needed here; the downstream code already handles both
// cases correctly.
func nativeMemberLinks(ds *namelist.Dataset) map[string][]string {
	links := map[string][]string{}
	for _, r := range ds.Rows {
		if r.ParentID == "" {
			continue
		}
		links[r.ParentID] = append(links[r.ParentID], r.SourceID)
	}
	return links
}

// ingestConceptSource reads cs's two canonical CDM CSVs and runs
// application.IngestCDM against repo. This is the adapter -> application DTO
// bridge for SP5: internal/application must not import
// internal/adapters/cdm (depguard), so the row mapping lives here in the
// composition root, exactly like wcvpRowSource above.
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
	Xrefs          []application.XrefIngestReport
	ConceptSources []application.CDMIngestReport
	NameSpaces     []application.NameSpaceIngestReport
	// NativeSpaces holds Fall B's own-concept ingest report for every name
	// space that also runs Fall B (currently eurosl/germansl — see
	// nativeSpaceBackboneIDs) — see Ingest()'s NameSpaces loop.
	NativeSpaces []application.NativeSpaceIngestReport
	// ConceptAgreement is the eurosl<->germansl aggregate-member comparison
	// (Task 7), computed once after every name space's Fall A+B ingest has
	// run — see Ingest()'s step 4.
	ConceptAgreement application.ConceptAgreementReport
}

// nameSpaceIDEurosl/nameSpaceIDGermansl are the two name-space ids pinned
// as named constants (rather than repeated string literals) so
// nativeSpaceBackboneIDs below and its callers/tests share one spelling.
const (
	nameSpaceIDEurosl   = "eurosl"
	nameSpaceIDGermansl = "germansl"
)

// nativeSpaceBackboneIDs are the name-space ids that also run Fall B
// (application.IngestNativeSpace) in Ingest()'s NameSpaces loop, immediately
// after their own Fall A (application.IngestNameSpace) run: eurosl and
// germansl are the only two canonical CSVs in the manifest that carry a
// parent_id chain above SPECIES (Task 3's deliberate scope) — floraveg/
// euromed's 5-column CSV has none, so running Fall B against them would be a
// silent no-op at best.
var nativeSpaceBackboneIDs = map[string]bool{nameSpaceIDEurosl: true, nameSpaceIDGermansl: true}

// ingestNameSpaceAndNative reads ns's canonical CSV exactly ONCE and runs
// Fall A (application.IngestNameSpace) against it, then — for eurosl/
// germansl only, see nativeSpaceBackboneIDs — Fall B
// (application.IngestNativeSpace) against the SAME already-read dataset.
// Split out of Ingest()'s NameSpaces loop to keep that function's cognitive
// complexity within the linter's bound (gocognit), mirroring
// writeNativeRow's own extraction pattern in nativespace_ingest.go.
//
// The returned *application.NativeSpaceIngestReport is nil when ns did not
// qualify for Fall B (ns.ID not in nativeSpaceBackboneIDs) — the caller
// appends it to reports.NativeSpaces only when non-nil.
func ingestNameSpaceAndNative(ctx context.Context, ns manifest.NameSpace, manifestSHA string, repo *sqlite.DB) (application.NameSpaceIngestReport, *application.NativeSpaceIngestReport, error) {
	nsDS, err := namelist.Read(ns.Path)
	if err != nil {
		return application.NameSpaceIngestReport{}, nil, fmt.Errorf("app: reading name space %q at %q: %w", ns.ID, ns.Path, err)
	}

	nr, err := ingestNameSpaceDataset(ctx, nsDS, ns, manifestSHA, repo)
	if err != nil {
		return nr, nil, err
	}
	if !nativeSpaceBackboneIDs[ns.ID] {
		return nr, nil, nil
	}

	// Fall B runs AFTER Fall A for the SAME name space, never before:
	// IngestNativeSpace's aggregate-member resolution
	// (ResolveNameSpaceMember) reads name_space_entry rows, which only
	// Fall A (just above) writes. minRank=domain.RankRoot writes every rank
	// above SPECIES as its own concept, not just a narrow band — see
	// nativeConceptRankOrder's own doc comment for the full root-to-leaf
	// ordering this cutoff is measured against.
	nsr, err := ingestNativeSpaceDataset(ctx, nsDS, ns, manifestSHA, repo, domain.RankRoot)
	if err != nil {
		return nr, nil, err
	}
	return nr, &nsr, nil
}

// Ingest parses and validates the manifest at manifestPath, opens (or
// creates) the SQLite database at dbPath, and runs application.Ingest
// against every pinned backbone, then application.IngestXrefs against every
// pinned xref source, then application.IngestCDM against every pinned concept
// source, then application.IngestNameSpace against every pinned name space.
// It is the entry point "hostus ingest" calls.
//
// A manifest's trait_vocabularies entries (manifest.Dataset.TraitVocabularies)
// are deliberately IGNORED here — the traits subsystem was removed and
// transferred to situs (Teilprojekt 2); a manifest still pinning one is not
// an error, its entries are simply never read.
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
	reports.NativeSpaces = make([]application.NativeSpaceIngestReport, 0, len(manifestDS.NameSpaces))
	for _, ns := range manifestDS.NameSpaces {
		nr, nsr, err := ingestNameSpaceAndNative(ctx, ns, manifestDS.ManifestSHA, repo)
		if err != nil {
			return reports, err
		}
		reports.NameSpaces = append(reports.NameSpaces, nr)
		if nsr != nil {
			reports.NativeSpaces = append(reports.NativeSpaces, *nsr)
		}
	}

	// ComputeConceptAgreement/WriteConceptAgreement (Task 7) run once, AFTER
	// every name space's Fall A+B ingest is done: the comparison pairs up
	// eurosl and germansl aggregate concepts by name and needs BOTH spaces'
	// concept_aggregate member lists fully written first. This runs BEFORE
	// BuildDistributionClosure below — it is itself part of "the whole
	// index is now built", not a distribution concern.
	agreementReport, err := application.ComputeConceptAgreement(ctx, repo)
	if err != nil {
		return reports, err
	}
	if err := repo.WriteConceptAgreement(ctx, agreementReport.Pairs); err != nil {
		return reports, err
	}
	reports.ConceptAgreement = agreementReport

	// BuildDistributionClosure runs once ALL backbones (incl. CDM) are
	// ingested — it resolves CDM concepts' in_area name fallback against WCVP
	// twins, which must already be present by this point.
	if err := repo.BuildDistributionClosure(ctx); err != nil {
		return reports, fmt.Errorf("app: building distribution closure: %w", err)
	}

	return reports, nil
}
