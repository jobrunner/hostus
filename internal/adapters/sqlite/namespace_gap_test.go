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
	// Look up by EITHER side of the OR — ConceptAgreement's own doc comment
	// promises "conceptID on either side", and both branches of that WHERE
	// clause need their own assertion (Copilot review, PR #92).
	for _, lookupID := range []string{want.EuroslConceptID, want.GermanslConceptID} {
		got, err := db.ConceptAgreement(context.Background(), lookupID)
		if err != nil || got == nil || got.EuroslConceptID != want.EuroslConceptID || got.GermanslConceptID != want.GermanslConceptID || got.Agreement != want.Agreement || got.AgreementText != want.AgreementText {
			t.Fatalf("ConceptAgreement(%q) = %+v, %v; want %+v, nil", lookupID, got, err, want)
		}
		if len(got.OnlyInEurosl) != 2 || got.OnlyInEurosl[0] != "e1" || got.OnlyInEurosl[1] != "e2" || len(got.OnlyInGermansl) != 1 || got.OnlyInGermansl[0] != "g1" {
			t.Fatalf("ConceptAgreement(%q) lists = %+v, %+v; want [e1 e2], [g1]", lookupID, got.OnlyInEurosl, got.OnlyInGermansl)
		}
	}
	missing, err := db.ConceptAgreement(context.Background(), "missing")
	if err != nil || missing != nil {
		t.Fatalf("ConceptAgreement(missing) = %+v, %v; want nil, nil", missing, err)
	}
}

// TestConceptAgreementDecodesEmptyCommaListsAsEmptyNonNilSlices pins the
// invariant ConceptAgreement's own doc comment states but
// TestConceptAgreementDecodesBothSidesAndLists never exercised: an empty
// only_in_eurosl/only_in_germansl column must decode to an empty, non-nil
// slice, never [""] (splitCommaList's early-return path — Copilot review,
// PR #92).
func TestConceptAgreementDecodesEmptyCommaListsAsEmptyNonNilSlices(t *testing.T) {
	db := openTestDB(t)
	seedTestConcept(t, db, "eurosl", "eurosl:concept:2", "Eurosl two", domain.RankSpeciesAggregate)
	seedTestConcept(t, db, "germansl", "germansl:concept:2", "Germansl two", domain.RankSpeciesAggregate)

	want := domain.ConceptAgreementPair{
		EuroslConceptID:   "eurosl:concept:2",
		GermanslConceptID: "germansl:concept:2",
		Agreement:         domain.AgreementIdentical,
		AgreementText:     "same",
		OnlyInEurosl:      nil,
		OnlyInGermansl:    nil,
	}
	if err := db.WriteConceptAgreement(context.Background(), []domain.ConceptAgreementPair{want}); err != nil {
		t.Fatalf("WriteConceptAgreement: %v", err)
	}

	got, err := db.ConceptAgreement(context.Background(), want.EuroslConceptID)
	if err != nil {
		t.Fatalf("ConceptAgreement: unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("ConceptAgreement = nil, want a decoded pair")
	}
	if got.OnlyInEurosl == nil || len(got.OnlyInEurosl) != 0 {
		t.Errorf("OnlyInEurosl = %+v (nil=%v), want an empty, non-nil slice", got.OnlyInEurosl, got.OnlyInEurosl == nil)
	}
	if got.OnlyInGermansl == nil || len(got.OnlyInGermansl) != 0 {
		t.Errorf("OnlyInGermansl = %+v (nil=%v), want an empty, non-nil slice", got.OnlyInGermansl, got.OnlyInGermansl == nil)
	}
}
