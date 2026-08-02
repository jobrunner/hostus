package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// CDMConceptRow is the application-level DTO for one concept of the CDM
// rl_standardliste harvest (SP5, UC6). internal/adapters/cdm's own row type
// is adapted into it by the composition root, the same bridge pattern
// RowSource/TraitRowSource/XrefRowSource use so application never imports an
// adapter (depguard).
//
// Every field is the verbatim source value; the interpretation (rank,
// status, relation vocabulary) happens here and in domain, not in the
// reader.
type CDMConceptRow struct {
	ConceptUUID        string
	ScientificName     string
	Authorship         string
	Rank               string
	Status             string
	SecUUID            string
	SecTitle           string
	ClassificationUUID string
	ParentUUID         string
}

// CDMRelationRow is the application-level DTO for one CDM concept relation.
// IsConceptRelation is tri-state — see internal/adapters/cdm.RelationRow.
type CDMRelationRow struct {
	FromUUID          string
	ToUUID            string
	RelationType      string
	RelationSymbol    string
	IsConceptRelation *bool
	RelationshipUUID  string
}

// CDMIngestReport summarizes one CDM ingest run.
type CDMIngestReport struct {
	Backbone string
	// Concepts/Relations are rows OFFERED; the *Written counters are rows
	// that actually landed. Everything in between is accounted for by one of
	// the loss counters below — no row is ever silently dropped.
	Concepts         int
	ConceptsWritten  int
	SkippedConcepts  int
	Relations        int
	RelationsWritten int
	// SecReferences is how many DISTINCT sec. reference spaces landed — the
	// headline number of SP5, since a reference space is what makes two
	// same-named concepts legitimately different rows.
	SecReferences      int
	ConceptsWithoutSec int
	// EmptyStatus counts concepts whose raw CDM taxonStatus was empty. That
	// is honest absence (the classification tree walk never reached them),
	// stored as domain.StatusUnknown and reported rather than defaulted to
	// "accepted".
	EmptyStatus int
	// OtherRanks/OtherRankSample mirror BackboneReport's: CDM carries 22
	// distinct raw rank spellings and most of the exotic ones ("Species
	// Aggregate", "Section bot.", "Unranked (infraspecific)") have no
	// canonical domain.Rank, so they land as RankOther with the verbatim
	// spelling preserved.
	OtherRanks      int
	OtherRankSample []RankVerbatimCount
	// UnresolvedParents counts concepts whose parent_uuid names a concept
	// this run cannot resolve. Their parent_id is left NULL rather than
	// written — taxon_concept.parent_id is a self-FK, so a dangling value
	// would fail the constraint and take the whole ingest with it.
	UnresolvedParents int
	// PerRelationType counts WRITTEN relations per hostus relation value
	// (domain.Relation), keyed by the canonical string so the report can be
	// printed without re-parsing.
	PerRelationType map[string]int
	// NonConcept counts relation rows that are NOT Berendsohn concept
	// relations — CDM's conceptRelationship=false rows (misapplied names).
	// They are DROPPED, never written to concept_relation; see IngestCDM's
	// doc comment for why.
	NonConcept       int
	NonConceptSample []string
	// UnknownFlag counts rows whose is_concept_relation column was EMPTY
	// (an edge seen only from its to-end). Unknown is not false: such rows
	// ARE written when their relation type is a concept relation, and this
	// counter exists so that decision stays visible.
	UnknownFlag int
	// UnresolvedEnds counts relation rows at least one of whose two ends
	// does not resolve to a taxon_concept. concept_relation FKs both ends,
	// so nothing is written for them — but they are counted and sampled,
	// never silently dropped, and never abort the ingest.
	UnresolvedEnds   int
	UnresolvedSample []string
	// Redistribution is the manifest-pinned value (CDM: "unknown" — the
	// source carries no findable license, see pipelines/cdm/README.md). It
	// gates ExportBundle, never local ingest.
	Redistribution string
	// ReaderErrors is filled in by the composition root: how many CSV rows
	// the reader itself had to skip (short row, malformed quoting, an
	// unparseable is_concept_relation flag). Carried on the report so a
	// damaged artifact is visible in "hostus ingest" output rather than only
	// in the reader's return value.
	ReaderErrors int
}

// cdmConceptID / cdmNameID key CDM's rows into hostus' id space. CDM
// concepts are a SECOND BACKBONE, deliberately separate rows from the WCVP
// concepts for the same name: WCVP says "Abies alba Mill." with no sec., CDM
// says "Abies alba Mill. sec. HEGI" and "Abies alba Mill. sec. Wisskirchen &
// Haeupler 1998", and those are three different circumscription claims.
// Merging them on the name would destroy exactly the information UC6 exists
// to serve.
func cdmConceptID(uuid string) string { return "cdm:concept:" + uuid }
func cdmNameID(uuid string) string    { return "cdm:name:" + uuid }

// IngestCDM writes the CDM concept graph into repo as a second backbone.
//
// It is STRICTLY TWO-PHASE, for the same reason IngestTraits/IngestXrefs
// are: the sqlite adapter pins its pool to one connection
// (SetMaxOpenConns(1)), so any repository READ issued while the ingest
// transaction is open deadlocks the process. Phase 1 resolves everything —
// which relation ends already exist in the database, which parent uuids are
// resolvable, which relation types parse — with no transaction open. Phase 2
// opens one transaction and only writes.
//
// Three decisions this function makes, all of them deliberate:
//
//  1. RELATION VOCABULARY — an unmapped relation type ABORTS the run, with
//     the offending value in the error (domain.ParseRelation). This is the
//     opposite of the rank handling two lines below it, and the asymmetry is
//     the point: a rank is descriptive metadata, so an unknown spelling
//     degrades to RankOther and the ingest continues; a relation is a
//     scientific claim about two circumscriptions, so silently coercing an
//     unknown type onto a neighboring value would fabricate one. The
//     schema's original five-value vocabulary was an ASSUMPTION that
//     measurement corrected (see domain.Relation) — the next correction must
//     be loud, not silent.
//
//  2. MISAPPLIED NAMES ARE DROPPED. CDM flags every relation with a
//     conceptRelationship boolean: true = a genuine concept relation,
//     false = a misapplied-name relation. The latter says a NAME was used
//     wrongly, not how two circumscriptions relate, so it is not written to
//     concept_relation at all — counted and sampled instead. Keeping them
//     would make /translate mix two kinds of claim under one column, and
//     dropping them silently would hide 344 rows of the measured harvest.
//     An EMPTY flag is unknown, not false: those rows are written if their
//     TYPE is a concept relation (RelationMisapplied is the only type that
//     is not), and counted under UnknownFlag.
//
//  3. DIRECTIONALITY — one canonical direction, no mirror rows. CDM only
//     ever emits "Includes", never "Included in", and hostus stores each
//     relation exactly as the source states it. Materializing the inverse
//     would double the table, make "how many relations does this source
//     assert?" ambiguous, and require a rule for pro parte/misapplied, which
//     have no meaningful inverse at all. Traversal in the other direction is
//     a query-time concern: domain.Relation.Inverse names the flipped
//     relation for /translate to use.
func IngestCDM(ctx context.Context, repo output.Repository, concepts []CDMConceptRow, relations []CDMRelationRow, meta domain.BackboneVersion) (CDMIngestReport, error) {
	report := CDMIngestReport{
		Backbone:        meta.ID,
		Concepts:        len(concepts),
		Relations:       len(relations),
		Redistribution:  string(meta.Redistribution),
		PerRelationType: map[string]int{},
	}

	plan, err := planCDMIngest(ctx, repo, concepts, relations, &report)
	if err != nil {
		return report, err
	}

	tx, err := repo.BeginIngest(ctx, meta)
	if err != nil {
		return report, fmt.Errorf("application: starting CDM ingest for %q: %w", meta.ID, err)
	}
	if err := writeCDM(tx, plan, meta, &report); err != nil {
		_ = tx.Rollback()
		return report, err
	}
	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing CDM ingest for %q: %w", meta.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing CDM ingest for %q: %w", meta.ID, err)
	}

	report.OtherRankSample = sortedRankCounts(plan.otherRankCounts, otherRankSampleCap)
	report.NonConceptSample = sortedSample(plan.nonConcept)
	report.UnresolvedSample = sortedSample(plan.unresolved)
	return report, nil
}

// cdmPlan is everything phase 1 resolved: the rows to write, in write order,
// plus the tallies phase 2 does not need to recompute.
type cdmPlan struct {
	secs      []domain.SecReference
	names     []domain.Name
	conceptOf []domain.Concept
	relations []cdmRelation

	otherRankCounts map[string]int
	nonConcept      map[string]bool
	unresolved      map[string]bool
}

type cdmRelation struct {
	from string
	to   string
	rel  domain.Relation
}

// planCDMIngest is phase 1: it turns the raw rows into exactly the writes
// phase 2 will perform, issuing its ONE repository read (ExistingConceptIDs)
// before any transaction exists.
func planCDMIngest(ctx context.Context, repo output.Repository, concepts []CDMConceptRow, relations []CDMRelationRow, report *CDMIngestReport) (*cdmPlan, error) {
	plan := &cdmPlan{
		otherRankCounts: map[string]int{},
		nonConcept:      map[string]bool{},
		unresolved:      map[string]bool{},
	}

	// thisRun is the set of concept ids this run itself writes. A relation
	// end or parent may point at one of them (resolvable without a read) or
	// at a concept an earlier run already wrote (resolved by the read
	// below).
	thisRun := make(map[string]bool, len(concepts))
	for _, row := range concepts {
		if row.ConceptUUID != "" {
			thisRun[cdmConceptID(row.ConceptUUID)] = true
		}
	}

	resolvable, err := resolveCDMEnds(ctx, repo, concepts, relations, thisRun)
	if err != nil {
		return nil, err
	}

	planCDMConcepts(concepts, resolvable, plan, report)
	if err := planCDMRelations(relations, resolvable, plan, report); err != nil {
		return nil, err
	}
	return plan, nil
}

// resolveCDMEnds returns the set of concept ids that will exist by the time
// relations are written: everything this run writes, plus everything the
// database already holds among the ids these rows reference. The repository
// read happens here — before BeginIngest — and nowhere else.
func resolveCDMEnds(ctx context.Context, repo output.Repository, concepts []CDMConceptRow, relations []CDMRelationRow, thisRun map[string]bool) (map[string]bool, error) {
	lookup := map[string]bool{}
	consider := func(uuid string) {
		if uuid == "" {
			return
		}
		if id := cdmConceptID(uuid); !thisRun[id] {
			lookup[id] = true
		}
	}
	for _, row := range concepts {
		consider(row.ParentUUID)
	}
	for _, row := range relations {
		consider(row.FromUUID)
		consider(row.ToUUID)
	}

	// The capacity argument is a sizing hint only: mutating the `+` here is
	// a genuinely equivalent mutant, since a map grows on demand and no
	// observable behavior depends on its initial capacity.
	resolvable := make(map[string]bool, len(thisRun)+len(lookup))
	for id := range thisRun {
		resolvable[id] = true
	}
	if len(lookup) == 0 {
		return resolvable, nil
	}

	ids := make([]string, 0, len(lookup))
	for id := range lookup {
		ids = append(ids, id)
	}
	existing, err := repo.ExistingConceptIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("application: resolving CDM relation ends: %w", err)
	}
	for id, present := range existing {
		if present {
			resolvable[id] = true
		}
	}
	return resolvable, nil
}

// planCDMConcepts turns concept rows into the name/concept/sec_reference
// writes, deduplicating reference spaces and tallying the honest-absence
// cases (empty status, exotic rank, unresolvable parent) rather than
// papering over them.
func planCDMConcepts(concepts []CDMConceptRow, resolvable map[string]bool, plan *cdmPlan, report *CDMIngestReport) {
	seenSec := map[string]bool{}
	for _, row := range concepts {
		if row.ConceptUUID == "" {
			report.SkippedConcepts++
			continue
		}
		if row.SecUUID == "" {
			report.ConceptsWithoutSec++
		} else if !seenSec[row.SecUUID] {
			seenSec[row.SecUUID] = true
			plan.secs = append(plan.secs, domain.SecReference{ID: row.SecUUID, Title: row.SecTitle})
		}

		rank, verbatim := domain.ParseRankLenient(row.Rank)
		if rank == domain.RankOther {
			plan.otherRankCounts[verbatim]++
			report.OtherRanks++
		} else {
			// Only RankOther has thrown information away, so only there is
			// the verbatim spelling worth keeping (same rule as
			// domain.Name.RankVerbatim).
			verbatim = ""
		}
		if row.Status == "" {
			report.EmptyStatus++
		}

		parentID := ""
		if row.ParentUUID != "" {
			if candidate := cdmConceptID(row.ParentUUID); resolvable[candidate] {
				parentID = candidate
			} else {
				report.UnresolvedParents++
			}
		}

		name := domain.Name{
			ID:           cdmNameID(row.ConceptUUID),
			Canonical:    row.ScientificName,
			Authorship:   row.Authorship,
			Rank:         rank,
			RankVerbatim: verbatim,
		}
		plan.names = append(plan.names, name)
		plan.conceptOf = append(plan.conceptOf, domain.Concept{
			ID:           cdmConceptID(row.ConceptUUID),
			BackboneID:   report.Backbone,
			AcceptedName: name,
			Rank:         rank,
			ParentID:     parentID,
			SecReference: row.SecUUID,
			Status:       domain.ParseStatus(row.Status),
			RankVerbatim: verbatim,
		})
	}
	report.SecReferences = len(plan.secs)
}

// planCDMRelations maps every relation row onto a write, or onto exactly one
// documented loss counter. It is the only place an unmapped relation type
// can stop the run.
func planCDMRelations(relations []CDMRelationRow, resolvable map[string]bool, plan *cdmPlan, report *CDMIngestReport) error {
	for _, row := range relations {
		rel, err := domain.ParseRelation(row.RelationType)
		if err != nil {
			return fmt.Errorf("application: CDM relation %s: %w", cdmRelationKey(row), err)
		}
		// A row is a concept relation unless CDM says otherwise (an
		// explicit false) or its TYPE is not one. An empty flag is unknown,
		// and unknown is not false.
		if !rel.IsConceptRelation() || (row.IsConceptRelation != nil && !*row.IsConceptRelation) {
			report.NonConcept++
			plan.nonConcept[cdmRelationKey(row)] = true
			continue
		}
		if row.IsConceptRelation == nil {
			report.UnknownFlag++
		}

		from, to := cdmConceptID(row.FromUUID), cdmConceptID(row.ToUUID)
		if row.FromUUID == "" || row.ToUUID == "" || !resolvable[from] || !resolvable[to] {
			report.UnresolvedEnds++
			plan.unresolved[cdmRelationKey(row)] = true
			continue
		}
		plan.relations = append(plan.relations, cdmRelation{from: from, to: to, rel: rel})
		report.PerRelationType[string(rel)]++
		report.RelationsWritten++
	}
	return nil
}

// cdmRelationKey formats a relation row for a report sample: its
// relationship uuid when it has one (the stable CDM identity), else the
// endpoint pair, so a sampled row can actually be looked up again.
func cdmRelationKey(row CDMRelationRow) string {
	if row.RelationshipUUID != "" {
		return row.RelationshipUUID
	}
	return fmt.Sprintf("%s->%s", row.FromUUID, row.ToUUID)
}

// writeCDM is phase 2: writes only, no repository reads. sec_reference rows
// go first (a concept's sec_reference id names one), then each name with its
// concept and the accepted link, then the relations — which come last
// because concept_relation FKs both ends to taxon_concept.
func writeCDM(tx output.IngestTx, plan *cdmPlan, meta domain.BackboneVersion, report *CDMIngestReport) error {
	for _, s := range plan.secs {
		if err := tx.UpsertSecReference(s); err != nil {
			return fmt.Errorf("application: writing sec. reference %q: %w", s.ID, err)
		}
	}
	for i, c := range plan.conceptOf {
		if err := tx.UpsertName(plan.names[i]); err != nil {
			return fmt.Errorf("application: writing CDM name %q: %w", plan.names[i].ID, err)
		}
		if err := tx.UpsertConcept(c); err != nil {
			return fmt.Errorf("application: writing CDM concept %q: %w", c.ID, err)
		}
		if err := tx.LinkName(c.ID, plan.names[i].ID, "accepted", nil); err != nil {
			return fmt.Errorf("application: linking CDM name %q to concept %q: %w", plan.names[i].ID, c.ID, err)
		}
		report.ConceptsWritten++
	}
	for _, r := range plan.relations {
		if err := tx.AddConceptRelation(r.from, r.to, r.rel, meta.ID); err != nil {
			return fmt.Errorf("application: writing concept relation %s -> %s (%s): %w", r.from, r.to, r.rel, err)
		}
	}
	return nil
}
