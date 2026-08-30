package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestResolveNameSpaceMemberReturnsMatchAndMissingAsEmpty(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)
	tx, err := db.BeginTraitIngest(context.Background())
	if err != nil {
		t.Fatalf("BeginTraitIngest: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, err := tx.ResolveNameSpaceMember("floraveg", "5647")
	if err != nil || got != corynephorusID {
		t.Fatalf("ResolveNameSpaceMember(found) = %q, %v; want %q, nil", got, err, corynephorusID)
	}
	got, err = tx.ResolveNameSpaceMember("floraveg", "missing")
	if err != nil || got != "" {
		t.Fatalf("ResolveNameSpaceMember(missing) = %q, %v; want empty, nil", got, err)
	}
}

func TestAggregateQueriesReturnBothDirectionsAndEmptySlices(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)
	ctx := context.Background()

	members, err := db.AggregateMembers(ctx, "eurosl:concept:agg1")
	if err != nil || len(members) != 1 || members[0] != corynephorusID {
		t.Fatalf("AggregateMembers = %+v, %v; want [%q], nil", members, err, corynephorusID)
	}
	aggregates, err := db.AggregatesByMember(ctx, corynephorusID)
	if err != nil || len(aggregates) != 1 || aggregates[0] != "eurosl:concept:agg1" {
		t.Fatalf("AggregatesByMember = %+v, %v; want [eurosl:concept:agg1], nil", aggregates, err)
	}
	empty, err := db.AggregateMembers(ctx, "missing-aggregate")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("AggregateMembers(missing) = %+v, %v; want empty non-nil slice, nil", empty, err)
	}
}

func TestConceptAgreementDecodesBothSidesAndLists(t *testing.T) {
	db := openTestDB(t)
	seedTestConcept(t, db, "eurosl", "eurosl:concept:1", "Eurosl one", domain.RankSpeciesAggregate)
	seedTestConcept(t, db, "germansl", "germansl:concept:1", "Germansl one", domain.RankSpeciesAggregate)
	want := domain.ConceptAgreementPair{
		EuroslConceptID:   "eurosl:concept:1",
		GermanslConceptID: "germansl:concept:1",
		Agreement:         domain.AgreementIdentical,
		AgreementText:     "same",
		OnlyInEurosl:      []string{"e1", "e2"},
		OnlyInGermansl:    []string{"g1"},
	}
	if err := db.WriteConceptAgreement(context.Background(), []domain.ConceptAgreementPair{want}); err != nil {
		t.Fatalf("WriteConceptAgreement: %v", err)
	}
	got, err := db.ConceptAgreement(context.Background(), want.EuroslConceptID)
	if err != nil || got == nil || got.EuroslConceptID != want.EuroslConceptID || got.GermanslConceptID != want.GermanslConceptID || got.Agreement != want.Agreement || got.AgreementText != want.AgreementText {
		t.Fatalf("ConceptAgreement = %+v, %v; want %+v, nil", got, err, want)
	}
	if len(got.OnlyInEurosl) != 2 || got.OnlyInEurosl[0] != "e1" || got.OnlyInEurosl[1] != "e2" || len(got.OnlyInGermansl) != 1 || got.OnlyInGermansl[0] != "g1" {
		t.Fatalf("ConceptAgreement lists = %+v, %+v; want [e1 e2], [g1]", got.OnlyInEurosl, got.OnlyInGermansl)
	}
	missing, err := db.ConceptAgreement(context.Background(), "missing")
	if err != nil || missing != nil {
		t.Fatalf("ConceptAgreement(missing) = %+v, %v; want nil, nil", missing, err)
	}
}
