package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// NativeRow is the minimal shape of one native-space row IngestNativeSpace
// needs — a row that describes its OWN taxon concept (Fall B: EuroSL/
// GermanSL ranks above SPECIES, plus aggregates), as opposed to NameRow
// (Fall A: a flat spelling attached to an existing backbone concept).
type NativeRow struct {
	Taxon    string
	SourceID string
	Rank     string // raw, mapped via domain.ParseRankLenient
	Status   string
	ParentID string // source's own IsChildTaxonOfID; "" = no parent
}

// NativeRowSource streams one native space's rows for IngestNativeSpace.
type NativeRowSource interface {
	Rows() []NativeRow
}

// NativeSpaceIngestReport summarizes one Fall-B native-concept ingest run.
type NativeSpaceIngestReport struct {
	Space   string
	Rows    int
	Written int
	// Skipped counts every row that did NOT qualify as its own native
	// concept: either its rank sits at-or-below SPECIES (Fall A's
	// territory, Task 4) or it is more general than minRank (e.g. a GENUS
	// row when minRank is SPECIES_AGGREGATE).
	Skipped int
	// UnknownRank counts rows where domain.ParseRankLenient fell back to
	// RankOther. UnknownRankSample is a deduplicated sample of the raw
	// spellings that triggered it.
	UnknownRank       int
	UnknownRankSample []string
	// MembersLinked counts every aggregate->member edge (concept_aggregate)
	// this run wrote, across every qualifying row's memberLinks entry.
	MembersLinked int
}

// nativeConceptRankOrder is the root-to-leaf ordering of domain.Rank used
// only to decide Fall B's cutoff: a row qualifies as its own native
// concept when its rank sits at-or-more-specific-than minRank but still
// strictly more general than SPECIES (Fall A's territory, Task 4). This
// mirrors the canonical rank set's own declared order (spec §2 / Task 1's
// const block comment), not a general-purpose rank comparator — hence
// local to this file rather than a domain.go export.
var nativeConceptRankOrder = []domain.Rank{
	domain.RankRoot, domain.RankPhylum, domain.RankSubdivision, domain.RankInformalClade,
	domain.RankClass, domain.RankSubclass, domain.RankSuperorder, domain.RankOrder,
	domain.RankFamily, domain.RankSubfamily, domain.RankTribe,
	domain.RankGenus, domain.RankSubgenus, domain.RankSection, domain.RankSubsection, domain.RankSeries,
	domain.RankSpeciesAggregate, domain.RankGenusAggregate,
	domain.RankSpecies, domain.RankCollSpecies, domain.RankSubspecies, domain.RankSubspeciesGroup,
	domain.RankVariety, domain.RankSubvariety, domain.RankForm, domain.RankSubform,
	domain.RankProles, domain.RankRace, domain.RankConvar, domain.RankGrex,
	domain.RankUnrankedInfrageneric, domain.RankUnrankedInfraspecific,
}

func nativeConceptRankOrdinal(r domain.Rank) (int, bool) {
	for i, rr := range nativeConceptRankOrder {
		if rr == r {
			return i, true
		}
	}
	return 0, false
}

// qualifiesAsFallBConcept reports whether rank should be written as its
// own native concept: at-or-more-specific-than minRank (ordinal >=) but
// strictly more general than SPECIES (ordinal <) — the boundary between
// Fall B (this function) and Fall A (Task 4's crosswalk, which owns
// SPECIES and everything more specific). A rank absent from
// nativeConceptRankOrder (RankOther, nothotaxon ranks, ...) never
// qualifies.
func qualifiesAsFallBConcept(rank, minRank domain.Rank) bool {
	ri, ok := nativeConceptRankOrdinal(rank)
	if !ok {
		return false
	}
	mi, ok := nativeConceptRankOrdinal(minRank)
	if !ok {
		return false
	}
	si, _ := nativeConceptRankOrdinal(domain.RankSpecies)
	return ri >= mi && ri < si
}

// rankVerbatimFor decides what to store in domain.Name.RankVerbatim /
// domain.Concept.RankVerbatim for one row: verbatim ONLY when rank is
// domain.RankOther, empty otherwise. This mirrors the documented invariant
// on both fields (see their doc comments in internal/domain/taxon.go: Rank
// alone already identifies the canonical spelling for every rank besides
// RankOther, so setting RankVerbatim unconditionally would defeat
// "RankVerbatim populated" as a signal for "this rank is exotic/RankOther")
// and the same gating internal/application/ingest.go's WCVP ingest already
// applies. Extracted as its own function (rather than an inline
// `if rank == domain.RankOther` at each of the two call sites) so the
// invariant itself — not just IngestNativeSpace's end-to-end behavior — is
// directly unit-testable.
func rankVerbatimFor(rank domain.Rank, verbatim string) string {
	if rank == domain.RankOther {
		return verbatim
	}
	return ""
}

// IngestNativeSpace writes every row of src whose rank qualifies (see
// qualifiesAsFallBConcept — at-or-more-specific-than minRank, but strictly
// more general than SPECIES) as its own taxon_concept with
// backbone_id = bv.ID. Species-/infraspecific rows are skipped: those are
// Fall A (IngestNameSpace, Task 4), not Fall B.
//
// Rank mapping is LENIENT (domain.ParseRankLenient), not strict: an
// unrecognized raw value must not abort the ingest (see spec section 11,
// correctness test 1) — it is counted as RankOther and reported with a
// sample (report.UnknownRankSample), never guessed or silently dropped.
//
// A qualifying row's ParentID is only carried onto concept.ParentID when
// the PARENT row also qualifies (see qualifyingSourceIDs below) — writing
// a parent_id that points at a concept this ingest never creates would
// violate taxon_concept's own FK constraint, and does happen in practice:
// an aggregate's ParentID commonly names its GENUS row, which sits below
// minRank and is therefore skipped.
func IngestNativeSpace(ctx context.Context, repo output.Repository, src NativeRowSource, bv domain.BackboneVersion, minRank domain.Rank, memberLinks map[string][]string) (NativeSpaceIngestReport, error) {
	report := NativeSpaceIngestReport{Space: bv.ID}
	rows := src.Rows()
	report.Rows = len(rows)

	// qualifyingSourceIDs is computed BEFORE the write loop, in a pass over
	// the whole row set, so parent linkage is order-independent: a row's
	// ParentID is only ever written when the parent row ITSELF qualifies as
	// a Fall-B concept (see below) — a parent_id pointing at a concept this
	// ingest never writes would violate taxon_concept's own FK constraint,
	// and CSV row order is not guaranteed to put a parent ahead of its
	// child.
	qualifyingSourceIDs := map[string]bool{}
	for _, row := range rows {
		rank, _ := domain.ParseRankLenient(row.Rank)
		if qualifiesAsFallBConcept(rank, minRank) {
			qualifyingSourceIDs[row.SourceID] = true
		}
	}

	tx, err := repo.BeginIngest(ctx, bv)
	if err != nil {
		return report, fmt.Errorf("application: starting native space ingest for %q: %w", bv.ID, err)
	}

	unknownSeen := map[string]bool{}
	for _, row := range rows {
		written, err := writeNativeRow(tx, row, bv, minRank, qualifyingSourceIDs, memberLinks, &report, unknownSeen)
		if err != nil {
			_ = tx.Rollback()
			return report, err
		}
		if written {
			report.Written++
		} else {
			report.Skipped++
		}
	}

	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing native space ingest for %q: %w", bv.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing native space ingest for %q: %w", bv.ID, err)
	}
	return report, nil
}

// writeNativeRow classifies and (if it qualifies) writes ONE row, updating
// report's UnknownRank tracking as a side effect. Split out of
// IngestNativeSpace's loop to keep that function's cognitive complexity
// within the linter's bound (gocognit) — mirrors writeNameSpaceRow's role
// in namespace_ingest.go (Task 4).
//
// The returned bool reports whether the row was written (true) or skipped
// (false); the caller updates report.Written/report.Skipped from it so
// those two counters stay the caller's single source of truth.
func writeNativeRow(
	tx output.IngestTx,
	row NativeRow,
	bv domain.BackboneVersion,
	minRank domain.Rank,
	qualifyingSourceIDs map[string]bool,
	memberLinks map[string][]string,
	report *NativeSpaceIngestReport,
	unknownSeen map[string]bool,
) (bool, error) {
	rank, verbatim := domain.ParseRankLenient(row.Rank)
	if rank == domain.RankOther {
		report.UnknownRank++
		if !unknownSeen[verbatim] {
			unknownSeen[verbatim] = true
			report.UnknownRankSample = append(report.UnknownRankSample, verbatim)
		}
	}
	if !qualifiesAsFallBConcept(rank, minRank) {
		return false, nil
	}

	// rankVerbatimFor gates RankVerbatim exactly like
	// internal/application/ingest.go's WCVP ingest: populated ONLY for
	// RankOther. See its own doc comment for why.
	rv := rankVerbatimFor(rank, verbatim)
	name := domain.Name{
		ID:           bv.ID + ":name:" + row.SourceID,
		Canonical:    row.Taxon,
		Rank:         rank,
		RankVerbatim: rv,
	}
	concept := domain.Concept{
		ID:           bv.ID + ":concept:" + row.SourceID,
		BackboneID:   bv.ID,
		AcceptedName: name,
		Rank:         rank,
		Status:       domain.StatusAccepted,
		// RankVerbatim mirrors name.RankVerbatim: both were derived from the
		// same row by the same domain.ParseRankLenient call, so they're
		// always identical (see domain.Concept.RankVerbatim's doc comment
		// for why it's still its own field). Matches
		// internal/application/ingest.go's WCVP ingest.
		RankVerbatim: rv,
	}
	if row.ParentID != "" && qualifyingSourceIDs[row.ParentID] {
		concept.ParentID = bv.ID + ":concept:" + row.ParentID
	}
	if err := tx.UpsertName(name); err != nil {
		return false, fmt.Errorf("application: writing native name %q: %w", row.Taxon, err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		return false, fmt.Errorf("application: writing native concept %q: %w", row.Taxon, err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		return false, fmt.Errorf("application: linking native name %q: %w", row.Taxon, err)
	}

	if err := linkAggregateMembers(tx, row, bv, concept.ID, rank, memberLinks, report); err != nil {
		return false, err
	}
	return true, nil
}

// linkAggregateMembers resolves and writes every concept_aggregate edge
// row's memberLinks entry names (Task 6), updating report.MembersLinked as
// a side effect. Split out of writeNativeRow to keep that function's
// cognitive complexity within the linter's bound (gocognit) — mirrors
// writeNativeRow's own extraction out of IngestNativeSpace's loop.
//
// rank is the row's own Rank (domain.ParseRankLenient(row.Rank) — already
// computed once by the caller, not recomputed here). A concept_aggregate
// edge's aggregating side must ALWAYS be a genuine collective/aggregate rank
// (isCollectiveRank, match.go Task 10) — never a bare Fall-B concept like
// GENUS or ORDER that merely happens to be the ParentID of a Fall-A-
// crosswalked member. Without this guard, a full production ingest
// (minRank=domain.RankRoot, which qualifies every rank as its own Fall-B
// concept) would contaminate concept_aggregate with GENUS->species edges,
// and internal/adapters/http/taxa.go's aggregateMembershipsFor — which picks
// its match by prefix with no ORDER BY — could then non-deterministically
// surface the GENUS concept instead of the real aggregate/section concept
// as aggregate_concept_id in /v1/concept/{id} responses (final-review
// residual finding).
func linkAggregateMembers(
	tx output.IngestTx,
	row NativeRow,
	bv domain.BackboneVersion,
	aggregateConceptID string,
	rank domain.Rank,
	memberLinks map[string][]string,
	report *NativeSpaceIngestReport,
) error {
	if !isCollectiveRank(rank) {
		return nil // GENUS/ORDER/etc. sind Fall-B-Konzepte, aber niemals die
		// aggregierende Seite einer concept_aggregate-Kante — nur echte
		// Sammel-Ränge dürfen Mitglieder tragen.
	}
	memberSourceIDs, ok := memberLinks[row.SourceID]
	if !ok {
		return nil
	}
	for _, memberSourceID := range memberSourceIDs {
		memberConceptID, err := tx.ResolveNameSpaceMember(bv.ID, memberSourceID)
		if err != nil {
			return fmt.Errorf("application: resolving aggregate member %q for %q: %w", memberSourceID, row.Taxon, err)
		}
		if memberConceptID == "" {
			continue // Fall-A-Crosswalk hat diese Zeile nicht aufgelöst — kein Fehler
		}
		if err := tx.AddAggregateMember(aggregateConceptID, memberConceptID); err != nil {
			return fmt.Errorf("application: linking aggregate member %q -> %q: %w", aggregateConceptID, memberConceptID, err)
		}
		report.MembersLinked++
	}
	return nil
}
