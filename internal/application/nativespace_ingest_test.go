package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// staticNativeRows is a fixed application.NativeRowSource for tests — the
// Fall-B counterpart of any static NameRowSource test double.
type staticNativeRows []application.NativeRow

func (s staticNativeRows) Rows() []application.NativeRow { return s }

// noMemberLinks is the empty memberLinks map passed by every test that does
// not exercise the Task 6 aggregate-member relation.
var noMemberLinks = map[string][]string{}

func TestIngestNativeSpace_WritesAggregateAsOwnConcept(t *testing.T) {
	repo := openMemoryRepo(t)
	src := staticNativeRows{
		{Taxon: "Salsola", SourceID: "genus1", Rank: "Genus", Status: "accepted"},
		// "SPECIES_AGGREGATE" (canonical spelling, domain.ParseRank), not the
		// brief's literal "Species Aggregate": domain.ParseRankLenient (Task
		// 1, already committed, out of this task's scope) is a strict
		// lookup table keyed on exact canonical spellings — "Species
		// Aggregate" (space instead of underscore) does not match any
		// canonicalRanks/germanSLRankCodes/nothotaxonRanks entry and
		// degrades to RankOther, which never qualifies as a Fall-B concept.
		// Using the canonical spelling here is what the brief's own
		// intent — "an aggregate row becomes its own concept" — requires.
		{Taxon: "Salsola kali aggr.", SourceID: "agg1", Rank: "SPECIES_AGGREGATE", Status: "accepted", ParentID: "genus1"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankSpeciesAggregate, noMemberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.Written != 1 {
		t.Fatalf("report.Written = %d, want 1 (Genus soll uebersprungen werden, da unterhalb minRank in der Hierarchie ausserhalb des Aggregat-Kontexts steht)", report.Written)
	}
	if report.Skipped != 1 {
		t.Errorf("report.Skipped = %d, want 1", report.Skipped)
	}

	// dogsled forbids a plain `:=` with 3+ blank identifiers; the
	// if/else-assign form below (matching namespace_ingest_test.go's
	// existing workaround) is exempt.
	var concept *domain.Concept
	if c, _, _, _, err := repo.Concept(context.Background(), "eurosl:concept:agg1"); err != nil {
		t.Fatalf("Concept: unexpected error: %v", err)
	} else {
		concept = c
	}
	if concept.Rank != domain.RankSpeciesAggregate {
		t.Errorf("concept.Rank = %q, want %q", concept.Rank, domain.RankSpeciesAggregate)
	}
	if concept.BackboneID != "eurosl" {
		t.Errorf("concept.BackboneID = %q, want %q", concept.BackboneID, "eurosl")
	}
}

func TestIngestNativeSpace_SkipsSpeciesAndInfraspecificRows(t *testing.T) {
	repo := openMemoryRepo(t)
	src := staticNativeRows{
		{Taxon: "Salsola kali", SourceID: "sp1", Rank: "Species", Status: "accepted"},
		{Taxon: "Salsola kali subsp. kali", SourceID: "ssp1", Rank: "Subspecies", Status: "accepted"},
		{Taxon: "Salsola", SourceID: "genus1", Rank: "Genus", Status: "accepted"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankGenus, noMemberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.Written != 1 {
		t.Fatalf("report.Written = %d, want 1 (nur Genus qualifiziert; Species/Subspecies gehoeren zu Fall A)", report.Written)
	}
	if report.Skipped != 2 {
		t.Errorf("report.Skipped = %d, want 2", report.Skipped)
	}
}

// TestIngestNativeSpace_OrdinaryRankGetsEmptyRankVerbatim is the fix-round-1
// regression test: domain.Name.RankVerbatim / domain.Concept.RankVerbatim
// are documented (internal/domain/taxon.go) as populated ONLY for
// domain.RankOther — Rank itself already identifies the canonical spelling
// for every other rank. A Fall-B concept with an ORDINARY rank (Family)
// must therefore come back with RankVerbatim == "", not the raw source
// spelling.
func TestIngestNativeSpace_OrdinaryRankGetsEmptyRankVerbatim(t *testing.T) {
	repo := openMemoryRepo(t)
	src := staticNativeRows{
		{Taxon: "Chenopodiaceae", SourceID: "fam1", Rank: "Family", Status: "accepted"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankFamily, noMemberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.Written != 1 {
		t.Fatalf("report.Written = %d, want 1", report.Written)
	}

	var concept *domain.Concept
	if c, _, _, _, err := repo.Concept(context.Background(), "eurosl:concept:fam1"); err != nil {
		t.Fatalf("Concept: unexpected error: %v", err)
	} else {
		concept = c
	}
	if concept.RankVerbatim != "" {
		t.Errorf("concept.RankVerbatim = %q, want empty (Family is not RankOther)", concept.RankVerbatim)
	}
}

func TestIngestNativeSpace_UnknownRankFallsBackToOtherAndIsReported(t *testing.T) {
	repo := openMemoryRepo(t)
	src := staticNativeRows{
		{Taxon: "Mystery taxon", SourceID: "x1", Rank: "totally-unknown-rank", Status: "accepted"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankSpeciesAggregate, noMemberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.UnknownRank != 1 {
		t.Errorf("report.UnknownRank = %d, want 1", report.UnknownRank)
	}
	if len(report.UnknownRankSample) != 1 || report.UnknownRankSample[0] != "totally-unknown-rank" {
		t.Errorf("report.UnknownRankSample = %v, want [totally-unknown-rank]", report.UnknownRankSample)
	}
	if report.Written != 0 {
		t.Errorf("report.Written = %d, want 0 (RankOther nie oberhalb SPECIES qualifiziert)", report.Written)
	}
	if report.Skipped != 1 {
		t.Errorf("report.Skipped = %d, want 1", report.Skipped)
	}
}

// TestIngestNativeSpace_LinksAggregateMembers pins Task 6: a Fall-B
// aggregate row's memberLinks entry resolves each listed member source id
// against name_space_entry (the Fall-A crosswalk Task 4 already wrote for
// this same space) and records the aggregate->member edge in
// concept_aggregate. Deviates from the brief's literal fixture (an invented
// "Salsola kali" WCVP concept via an unspecified seededNamespaceRepo
// helper): reuses seededMatchRepo's real WCVP fixture and its already-known
// festucaOvinaConceptID instead of inventing a second concept-seeding
// helper, matching namespace_ingest_test.go's own established precedent
// (TestIngestNameSpace_WritesClassificationOntoMatchedConcept's doc
// comment).
func TestIngestNativeSpace_LinksAggregateMembers(t *testing.T) {
	repo := seededMatchRepo(t) // WCVP concept "Festuca ovina" already exists
	// Fall A (Task 4) has already linked "Festuca ovina" under source id
	// "fo1" in the eurosl name space:
	seedNameSpaceEntry(t, repo, "eurosl", "fo1", festucaOvinaConceptID)

	src := staticNativeRows{
		{Taxon: "Festuca ovina aggr.", SourceID: "agg1", Rank: "SPECIES_AGGREGATE", Status: "accepted"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}
	memberLinks := map[string][]string{"agg1": {"fo1"}} // aggregat-source-id -> [mitglied-source-ids]

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankSpeciesAggregate, memberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.MembersLinked != 1 {
		t.Errorf("report.MembersLinked = %d, want 1", report.MembersLinked)
	}

	members, err := repo.AggregateMembers(context.Background(), "eurosl:concept:agg1")
	if err != nil {
		t.Fatalf("AggregateMembers: unexpected error: %v", err)
	}
	if len(members) != 1 || members[0] != festucaOvinaConceptID {
		t.Errorf("AggregateMembers = %v, want [%s]", members, festucaOvinaConceptID)
	}
}

// TestIngestNativeSpace_UnresolvedMemberIsSkippedNotError pins the brief's
// explicit non-error case: a memberLinks entry naming a member source id
// that the Fall-A crosswalk never resolved (no name_space_entry row) is
// silently skipped, not an ingest failure.
func TestIngestNativeSpace_UnresolvedMemberIsSkippedNotError(t *testing.T) {
	repo := openMemoryRepo(t)
	src := staticNativeRows{
		{Taxon: "Festuca ovina aggr.", SourceID: "agg1", Rank: "SPECIES_AGGREGATE", Status: "accepted"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}
	memberLinks := map[string][]string{"agg1": {"does-not-exist"}}

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankSpeciesAggregate, memberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.MembersLinked != 0 {
		t.Errorf("report.MembersLinked = %d, want 0", report.MembersLinked)
	}

	members, err := repo.AggregateMembers(context.Background(), "eurosl:concept:agg1")
	if err != nil {
		t.Fatalf("AggregateMembers: unexpected error: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("AggregateMembers = %v, want empty", members)
	}
}

// seedNameSpaceEntry writes one name_space_entry row directly (bypassing
// IngestNameSpace's own name-matching, which this task's fixture does not
// need to exercise) so a test can pin what Task 6 consumes: a Fall-A
// crosswalk result already sitting in name_space_entry for (space,
// sourceID) -> conceptID.
func seedNameSpaceEntry(t *testing.T, repo *sqlite.DB, space, sourceID, conceptID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: space, Version: "test", Redistribution: domain.RedistributionUnknown})
	if err != nil {
		t.Fatalf("BeginIngest(%s): unexpected error: %v", space, err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{ID: space, Version: "test", Redistribution: domain.RedistributionUnknown}); err != nil {
		t.Fatalf("UpsertNameSpace(%s): unexpected error: %v", space, err)
	}
	if err := tx.AddNameSpaceEntry(conceptID, domain.NameSpaceEntry{Space: space, ExtID: sourceID, Name: conceptID, Status: "accepted"}); err != nil {
		t.Fatalf("AddNameSpaceEntry(%s:%s): unexpected error: %v", space, sourceID, err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}
