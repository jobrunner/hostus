package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedBackboneAndSecConcept ingests one plain backbone (WCVP-shaped, no
// SecReference) concept and one sec.-space (CDM-shaped) concept sharing the
// exact same canonical+author — the multi-backbone shape spec 2026-09-01 B1
// found causing /v1/match to answer "unresolvable" once CDM was loaded
// alongside the taxonomic backbone.
func seedBackboneAndSecConcept(t *testing.T, repo *sqlite.DB) (backboneConceptID, secConceptID, secID string) {
	t.Helper()
	ctx := context.Background()

	// Backbone (WCVP-shaped): no SecReference.
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest(wcvp): %v", err)
	}
	bbName := domain.Name{ID: "wcvp:name:ps1", Canonical: "Pinus sylvestris", Authorship: "L.", Rank: domain.RankSpecies}
	bbConcept := domain.Concept{ID: "wcvp:concept:ps1", BackboneID: "wcvp", AcceptedName: bbName, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(bbName); err != nil {
		t.Fatalf("UpsertName(wcvp): %v", err)
	}
	if err := tx.UpsertConcept(bbConcept); err != nil {
		t.Fatalf("UpsertConcept(wcvp): %v", err)
	}
	if err := tx.LinkName(bbConcept.ID, bbName.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(wcvp): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit(wcvp): %v", err)
	}

	// Sec.-space (CDM-shaped): same canonical+author, SecReference set.
	secID = "cdm-sec-wh98"
	tx, err = repo.BeginIngest(ctx, domain.BackboneVersion{ID: "cdm", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest(cdm): %v", err)
	}
	if err := tx.UpsertSecReference(domain.SecReference{ID: secID, Title: "Wisskirchen & Haeupler 1998"}); err != nil {
		t.Fatalf("UpsertSecReference: %v", err)
	}
	secName := domain.Name{ID: "cdm:name:ps1", Canonical: "Pinus sylvestris", Authorship: "L.", Rank: domain.RankSpecies}
	secConcept := domain.Concept{ID: "cdm:concept:ps1", BackboneID: "cdm", AcceptedName: secName, Rank: domain.RankSpecies, Status: domain.StatusAccepted, SecReference: secID}
	if err := tx.UpsertName(secName); err != nil {
		t.Fatalf("UpsertName(cdm): %v", err)
	}
	if err := tx.UpsertConcept(secConcept); err != nil {
		t.Fatalf("UpsertConcept(cdm): %v", err)
	}
	if err := tx.LinkName(secConcept.ID, secName.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(cdm): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit(cdm): %v", err)
	}

	return bbConcept.ID, secConcept.ID, secID
}

// TestMatchNames_SecSpaceConceptDoesNotCauseAmbiguity pinnt Audit-Befund B1
// (e2e gemessen: POST /v1/match lieferte "unresolvable" für Pinus
// sylvestris, sobald CDM geladen ist): ein sec.-Space-Konzept ist ein
// Attributions-Detail, kein zweiter Claimant — dieselbe Regel, die der
// Ingest-Crosswalk seit PR #94 anwendet.
func TestMatchNames_SecSpaceConceptDoesNotCauseAmbiguity(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	backboneConceptID, _, _ := seedBackboneAndSecConcept(t, repo)

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: "Pinus sylvestris L."}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].ConceptID != backboneConceptID {
		t.Fatalf("ConceptID = %q, want %q (sec-Konzept darf nicht als Claimant zählen); Note=%q", results[0].ConceptID, backboneConceptID, results[0].Note)
	}
}

// TestMatchNames_EntrySecFilterStillReachesSecConcept pinnt Entscheidung 2
// der Spec: mit explizitem entry_sec bleibt das sec-Konzept erreichbar — die
// Präferenz läuft NUR bei leerem Filter.
func TestMatchNames_EntrySecFilterStillReachesSecConcept(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	_, secConceptID, secID := seedBackboneAndSecConcept(t, repo)

	results, err := application.MatchInSpace(ctx, repo,
		[]application.MatchRequest{{ID: "1", Verbatim: "Pinus sylvestris L."}}, "",
		application.MatchFilter{Sec: secID})
	if err != nil {
		t.Fatalf("MatchInSpace(entry_sec=%s): %v", secID, err)
	}
	if results[0].ConceptID != secConceptID {
		t.Fatalf("ConceptID = %q, want %q — entry_sec must still reach the sec concept", results[0].ConceptID, secConceptID)
	}
}

// TestMatchNames_NativeGenusConceptDoesNotCauseAmbiguity pinnt B2 für den
// Serving-Pfad: "Abies" (WCVP-Gattung + eurosl-natives Fall-B-Konzept) muss
// aufs WCVP-Konzept auflösen.
func TestMatchNames_NativeGenusConceptDoesNotCauseAmbiguity(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()

	// 1. WCVP-artiges Backbone mit der Gattung.
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "g1", AcceptedTaxonID: "g1", Accepted: true, Canonical: "Abies", Rank: "GENUS", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// 2. eurosl Fall B: natives GENUS-Konzept gleichen Namens.
	native := staticNativeRows{
		{Taxon: "Abies", SourceID: "e1", Rank: "Genus", Status: "accepted"},
	}
	if _, err := application.IngestNativeSpace(ctx, repo, native, eurosBackboneVersion, domain.RankRoot, noMemberLinks); err != nil {
		t.Fatalf("IngestNativeSpace: %v", err)
	}
	// eurosl muss auch als name_space registriert sein, damit nativeSpaceSet
	// es kennt — im echten Ingest passiert das durch eurosls eigenen
	// Fall-A-Lauf (IngestNameSpace -> UpsertNameSpace).
	if _, err := application.IngestNameSpace(ctx, repo, sliceRowSource{}, eurosMeta); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: "Abies"}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].ConceptID != "wcvp:concept:g1" {
		t.Fatalf("ConceptID = %q, want wcvp:concept:g1 (natives Fall-B-Konzept darf nicht als Claimant zählen); Note=%q", results[0].ConceptID, results[0].Note)
	}
}
