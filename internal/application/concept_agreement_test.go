package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedMemberConcept writes memberConceptID as a minimal SPECIES-rank
// concept under its own backbone (derived from the id's "<backbone>:
// concept:<sourceID>" shape), idempotently — seedAggregateWithMembers calls
// it once per member id, and two aggregates in the same test commonly share
// a member.
func seedMemberConcept(t *testing.T, repo *sqlite.DB, memberConceptID string) {
	t.Helper()
	backbone, sourceID, ok := strings.Cut(memberConceptID, ":concept:")
	if !ok {
		t.Fatalf("seedMemberConcept: %q is not a <backbone>:concept:<id> shape", memberConceptID)
	}
	bv := domain.BackboneVersion{ID: backbone, Version: "test", Redistribution: domain.RedistributionUnknown}
	tx, err := repo.BeginIngest(context.Background(), bv)
	if err != nil {
		t.Fatalf("BeginIngest(%q): unexpected error: %v", backbone, err)
	}
	name := domain.Name{ID: memberConceptID + ":name", Canonical: "Member " + sourceID, Rank: domain.RankSpecies}
	concept := domain.Concept{ID: memberConceptID, BackboneID: backbone, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// seedAggregateWithMembers writes aggregateConceptID as a native
// SPECIES_AGGREGATE concept (canonical name) under its own backbone
// (derived from the id's "<backbone>:concept:<id>" shape) and links it to
// memberConceptIDs via concept_aggregate, creating each member as a
// minimal SPECIES concept first if it does not already exist.
func seedAggregateWithMembers(t *testing.T, repo *sqlite.DB, aggregateConceptID, canonical string, memberConceptIDs []string) {
	t.Helper()
	for _, m := range memberConceptIDs {
		seedMemberConcept(t, repo, m)
	}

	backbone, sourceID, ok := strings.Cut(aggregateConceptID, ":concept:")
	if !ok {
		t.Fatalf("seedAggregateWithMembers: %q is not a <backbone>:concept:<id> shape", aggregateConceptID)
	}
	bv := domain.BackboneVersion{ID: backbone, Version: "test", Redistribution: domain.RedistributionUnknown}
	tx, err := repo.BeginIngest(context.Background(), bv)
	if err != nil {
		t.Fatalf("BeginIngest(%q): unexpected error: %v", backbone, err)
	}
	name := domain.Name{ID: aggregateConceptID + ":name:" + sourceID, Canonical: canonical, Rank: domain.RankSpeciesAggregate}
	concept := domain.Concept{ID: aggregateConceptID, BackboneID: backbone, AcceptedName: name, Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	for _, m := range memberConceptIDs {
		if err := tx.AddAggregateMember(aggregateConceptID, m); err != nil {
			t.Fatalf("AddAggregateMember(%q, %q): unexpected error: %v", aggregateConceptID, m, err)
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

func TestComputeConceptAgreement_IdenticalMembersYieldsIdentical(t *testing.T) {
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:agg1", "Salsola kali aggr.", []string{"wcvp:concept:1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:agg2", "Salsola kali s. l.", []string{"wcvp:concept:1"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if len(report.Pairs) != 1 {
		t.Fatalf("report.Pairs = %d, want 1", len(report.Pairs))
	}
	if report.Pairs[0].Agreement != domain.AgreementIdentical {
		t.Errorf("Agreement = %q, want %q", report.Pairs[0].Agreement, domain.AgreementIdentical)
	}
	if report.Pairs[0].EuroslConceptID != "eurosl:concept:agg1" || report.Pairs[0].GermanslConceptID != "germansl:concept:agg2" {
		t.Errorf("EuroslConceptID/GermanslConceptID = %q/%q, want eurosl:concept:agg1/germansl:concept:agg2",
			report.Pairs[0].EuroslConceptID, report.Pairs[0].GermanslConceptID)
	}
	if report.Pairs[0].AgreementText == "" {
		t.Error("AgreementText is empty, want a generated sentence")
	}
}

func TestComputeConceptAgreement_DifferingMembersYieldsSuperset(t *testing.T) {
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:agg1", "Salsola kali aggr.", []string{"wcvp:concept:1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:agg2", "Salsola kali s. l.", []string{"wcvp:concept:1", "wcvp:concept:2"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if len(report.Pairs) != 1 {
		t.Fatalf("report.Pairs = %d, want 1", len(report.Pairs))
	}
	if report.Pairs[0].Agreement != domain.AgreementSubset {
		t.Errorf("Agreement = %q, want %q (eurosl ist Teilmenge von germansl)", report.Pairs[0].Agreement, domain.AgreementSubset)
	}
	if len(report.Pairs[0].OnlyInGermansl) != 1 || report.Pairs[0].OnlyInGermansl[0] != "wcvp:concept:2" {
		t.Errorf("OnlyInGermansl = %v, want [wcvp:concept:2]", report.Pairs[0].OnlyInGermansl)
	}
}

func TestComputeConceptAgreement_OneSidedAggregateInEuroslOnly(t *testing.T) {
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:agg1", "Rubus fruticosus agg.", []string{"wcvp:concept:1"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if len(report.Pairs) != 1 {
		t.Fatalf("report.Pairs = %d, want 1", len(report.Pairs))
	}
	pair := report.Pairs[0]
	if pair.Agreement != domain.AgreementOneSided {
		t.Errorf("Agreement = %q, want %q", pair.Agreement, domain.AgreementOneSided)
	}
	if pair.EuroslConceptID != "eurosl:concept:agg1" {
		t.Errorf("EuroslConceptID = %q, want eurosl:concept:agg1", pair.EuroslConceptID)
	}
	if pair.GermanslConceptID != "" {
		t.Errorf("GermanslConceptID = %q, want empty", pair.GermanslConceptID)
	}
}

func TestComputeConceptAgreement_WriteConceptAgreementAcceptsResult(t *testing.T) {
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:agg1", "Salsola kali aggr.", []string{"wcvp:concept:1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:agg2", "Salsola kali s. l.", []string{"wcvp:concept:1"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if err := repo.WriteConceptAgreement(context.Background(), report.Pairs); err != nil {
		t.Fatalf("WriteConceptAgreement: unexpected error: %v", err)
	}
}
