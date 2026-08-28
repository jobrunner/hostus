package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// seedTestConcept writes a minimal, valid concept for the AggregateConcepts/
// WriteConceptAgreement tests below.
func seedTestConcept(t *testing.T, db *DB, backboneID, conceptID, canonical string, rank domain.Rank) {
	t.Helper()
	tx, err := db.BeginIngest(context.Background(), domain.BackboneVersion{ID: backboneID, Version: "test", Redistribution: domain.RedistributionUnknown})
	if err != nil {
		t.Fatalf("BeginIngest(%q): unexpected error: %v", backboneID, err)
	}
	name := domain.Name{ID: conceptID + ":name", Canonical: canonical, Rank: rank}
	concept := domain.Concept{ID: conceptID, BackboneID: backboneID, AcceptedName: name, Rank: rank, Status: domain.StatusAccepted}
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

func TestAggregateConcepts_FiltersByBackboneAndRank(t *testing.T) {
	db := openTestDB(t)
	seedTestConcept(t, db, "eurosl", "eurosl:concept:agg1", "Salsola kali aggr.", domain.RankSpeciesAggregate)
	seedTestConcept(t, db, "eurosl", "eurosl:concept:sp1", "Salsola kali", domain.RankSpecies)
	seedTestConcept(t, db, "germansl", "germansl:concept:agg2", "Salsola kali s. l.", domain.RankSpeciesAggregate)

	got, err := db.AggregateConcepts(context.Background(), "eurosl", []domain.Rank{domain.RankSpeciesAggregate, domain.RankGenusAggregate})
	if err != nil {
		t.Fatalf("AggregateConcepts: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("AggregateConcepts = %+v, want 1 entry (only the eurosl aggregate, not the eurosl species or the germansl aggregate)", got)
	}
	if got[0].ConceptID != "eurosl:concept:agg1" || got[0].Canonical != "Salsola kali aggr." {
		t.Errorf("AggregateConcepts[0] = %+v, want {eurosl:concept:agg1 Salsola kali aggr.}", got[0])
	}
}

func TestAggregateConcepts_NoMatchingRowsReturnsEmptyNonNilSlice(t *testing.T) {
	db := openTestDB(t)
	got, err := db.AggregateConcepts(context.Background(), "eurosl", []domain.Rank{domain.RankSpeciesAggregate})
	if err != nil {
		t.Fatalf("AggregateConcepts: unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("AggregateConcepts = %+v, want an empty, non-nil slice", got)
	}
}

func TestWriteConceptAgreement_WritesRowWithNullOneSidedID(t *testing.T) {
	db := openTestDB(t)
	seedTestConcept(t, db, "eurosl", "eurosl:concept:agg1", "Rubus fruticosus agg.", domain.RankSpeciesAggregate)

	pairs := []domain.ConceptAgreementPair{
		{
			EuroslConceptID: "eurosl:concept:agg1",
			Agreement:       domain.AgreementOneSided,
			AgreementText:   "nur eurosl kennt dieses Aggregat.",
		},
	}
	if err := db.WriteConceptAgreement(context.Background(), pairs); err != nil {
		t.Fatalf("WriteConceptAgreement: unexpected error: %v", err)
	}

	var (
		euroslID   string
		germanslID sql.NullString
		agreement  string
	)
	row := db.sql.QueryRowContext(context.Background(),
		`SELECT eurosl_concept_id, germansl_concept_id, agreement FROM concept_agreement WHERE eurosl_concept_id = ?`,
		"eurosl:concept:agg1")
	if err := row.Scan(&euroslID, &germanslID, &agreement); err != nil {
		t.Fatalf("querying concept_agreement: unexpected error: %v", err)
	}
	if germanslID.Valid {
		t.Errorf("germansl_concept_id = %q, want SQL NULL", germanslID.String)
	}
	if agreement != string(domain.AgreementOneSided) {
		t.Errorf("agreement = %q, want %q", agreement, domain.AgreementOneSided)
	}
}
