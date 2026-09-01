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

// TestMatchFuzzy_PreferenceAppliesPerSpellingNotAcrossThePool pinnt den
// Whole-Branch-Review-Fund C1: matchFuzzy's Kandidaten-Pool ist HETEROGEN
// (viele verschiedene Schreibweisen aus dem Präfix+Längenfenster-Prefilter).
// Vor dem Fix lief preferGenuineClaimants auf dem GANZEN Pool: sobald
// IRGENDein genuiner (nicht-nativer) Name im Pool lag, flogen ALLE nativen
// Kandidaten ANDERER Namen raus — Fuzzy-Auflösung auf eine native-only
// Gattung war faktisch abgeschaltet, sobald der Prefilter zufällig auch
// einen unverwandten Backbone-Namen mit demselben Präfix fand.
//
// Fixture: eurosl-natives Fall-B-Konzept "Abietinella" (Moos-Gattung, WCVP
// führt sie nicht) + WCVP-Art "Abies alba" — beide teilen sich das
// Präfilter-Präfix "abie" und liegen im Längenfenster der Typo-Query
// "Abietinela", sind aber VÖLLIG unterschiedliche Namen. Die Präferenz darf
// "Abietinella" nur verwerfen, wenn ein genuiner Kandidat MIT DERSELBEN
// SCHREIBWEISE existiert — nicht, weil irgendein anderer Name im selben
// Pool zufällig genuin ist.
func TestMatchFuzzy_PreferenceAppliesPerSpellingNotAcrossThePool(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()

	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "aa1", AcceptedTaxonID: "aa1", Accepted: true, Canonical: "Abies alba", Rank: "SPECIES", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	native := staticNativeRows{
		{Taxon: "Abietinella", SourceID: "e1", Rank: "Genus", Status: "accepted"},
	}
	if _, err := application.IngestNativeSpace(ctx, repo, native, eurosBackboneVersion, domain.RankRoot, noMemberLinks); err != nil {
		t.Fatalf("IngestNativeSpace: %v", err)
	}
	if _, err := application.IngestNameSpace(ctx, repo, sliceRowSource{}, eurosMeta); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: "Abietinela"}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].ConceptID != "eurosl:concept:e1" || results[0].MatchType != domain.MatchFuzzy {
		t.Fatalf("ConceptID/MatchType = %q/%q, want eurosl:concept:e1/%q (das native-only Konzept muss trotz eines unverwandten WCVP-Namens im selben Pool fuzzy auflösen); Note=%q, Candidates=%v",
			results[0].ConceptID, results[0].MatchType, domain.MatchFuzzy, results[0].Note, results[0].Candidates)
	}
}

// TestMatchFuzzy_SecStillDroppedWithinSameSpellingGroup ist die Gegenprobe
// zu C1: innerhalb EINER Schreibweisen-Gruppe muss die Präferenz weiterhin
// greifen — ein sec.-Space-Zwilling derselben Schreibweise darf nicht als
// zweiter Claimant zählen und keine falsche Fuzzy-Ambiguität erzeugen.
func TestMatchFuzzy_SecStillDroppedWithinSameSpellingGroup(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	backboneConceptID, _, _ := seedBackboneAndSecConcept(t, repo)

	// "Pinus sylvestri" (fehlendes Schluss-s) ist kein exaktes Match, liegt
	// aber im Präfix+Längenfenster beider gleichnamigen Kandidaten (Backbone
	// + CDM-sec).
	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: "Pinus sylvestri"}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].MatchType != domain.MatchFuzzy {
		t.Fatalf("MatchType = %q, want %q — Note=%q Candidates=%v", results[0].MatchType, domain.MatchFuzzy, results[0].Note, results[0].Candidates)
	}
	if results[0].ConceptID != backboneConceptID {
		t.Fatalf("ConceptID = %q, want %q (der sec-Zwilling darf innerhalb derselben Schreibweise weiterhin keine falsche Fuzzy-Ambiguität erzeugen); Note=%q",
			results[0].ConceptID, backboneConceptID, results[0].Note)
	}
}

// TestMatchNames_AggregateSecTwinPrefersNative deckt I1(a) ab: matchAggregate
// wendet dieselbe filter.empty()-Präferenz an wie matchOne — ein natives
// Aggregat-Konzept (kein SecReference) mit einem gleichnamigen CDM-sec-
// Zwilling löst aufs native Konzept auf, nicht auf eine Ambiguität.
func TestMatchNames_AggregateSecTwinPrefersNative(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	const canonical = "Festuca ovina agg."

	mk := func(bb, id, sec string) {
		tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: bb, Version: "v1"})
		if err != nil {
			t.Fatalf("BeginIngest(%s): %v", bb, err)
		}
		if sec != "" {
			if err := tx.UpsertSecReference(domain.SecReference{ID: sec, Title: sec}); err != nil {
				t.Fatalf("UpsertSecReference(%s): %v", sec, err)
			}
		}
		name := domain.Name{ID: id + ":name", Canonical: canonical, Rank: domain.RankSpeciesAggregate}
		concept := domain.Concept{ID: id, BackboneID: bb, AcceptedName: name, Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted, SecReference: sec}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(%s): %v", bb, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept(%s): %v", bb, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(%s): %v", bb, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(%s): %v", bb, err)
		}
	}
	mk("eurosl", "eurosl:concept:fo-agg", "")
	mk("cdm", "cdm:concept:fo-agg", "cdm-sec-fo")

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: canonical}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].ConceptID != "eurosl:concept:fo-agg" || results[0].MatchType != domain.MatchAggregateAlias {
		t.Fatalf("ConceptID/MatchType = %q/%q, want eurosl:concept:fo-agg/%q; Note=%q Candidates=%v",
			results[0].ConceptID, results[0].MatchType, domain.MatchAggregateAlias, results[0].Note, results[0].Candidates)
	}
}

// TestMatchNames_AggregateNominateDemotesNativeFallBAggregate pinnt I4: eine
// Konsequenz von Stufe 2, die GEWOLLT ist, nicht ein weiterer Bug. Ein
// eurosl-natives Fall-B-Aggregat kann als nackter Binomialname gespeichert
// sein (Rang SPECIES_AGGREGATE, aber ohne "agg."-Marker im Namen selbst).
// Fällt eine Aggregat-Query über matchAggregateNominate auf genau diese
// Basis zurück, demotet Stufe 2 das native Aggregat zugunsten einer
// gleichnamigen WCVP-Art — das Ergebnis ist die WCVP-Art als
// MatchAggregateNominate. Vorher (ohne die Präferenz) war die Basis
// ambiguous (zwei Kandidaten gleicher Stärke) und löste GAR NICHT auf; eine
// Rang-Ausnahme für SPECIES_AGGREGATE würde den germansl-Crosswalk-Fix (Spec
// B2) rückgängig machen — siehe Spec-Entscheidung 6.
func TestMatchNames_AggregateNominateDemotesNativeFallBAggregate(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	const bareBinomial = "Hieracium sabaudum"

	// WCVP-Art mit demselben nackten Binomialnamen.
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "hs1", AcceptedTaxonID: "hs1", Accepted: true, Canonical: bareBinomial, Rank: "SPECIES", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// eurosl-natives Fall-B-Aggregat: SPECIES_AGGREGATE-Rang, aber der
	// gespeicherte Name ist der nackte Binomialname (ohne "agg."-Marker) —
	// das reale Muster, das matchAggregateNominate's Basis-Fallback trifft.
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest(eurosl): %v", err)
	}
	aggName := domain.Name{ID: "eurosl:concept:hs-agg:name", Canonical: bareBinomial, Rank: domain.RankSpeciesAggregate}
	aggConcept := domain.Concept{ID: "eurosl:concept:hs-agg", BackboneID: "eurosl", AcceptedName: aggName, Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted}
	if err := tx.UpsertName(aggName); err != nil {
		t.Fatalf("UpsertName(eurosl): %v", err)
	}
	if err := tx.UpsertConcept(aggConcept); err != nil {
		t.Fatalf("UpsertConcept(eurosl): %v", err)
	}
	if err := tx.LinkName(aggConcept.ID, aggName.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(eurosl): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit(eurosl): %v", err)
	}
	if _, err := application.IngestNameSpace(ctx, repo, sliceRowSource{}, eurosMeta); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: bareBinomial + " agg."}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].ConceptID != "wcvp:concept:hs1" || results[0].MatchType != domain.MatchAggregateNominate {
		t.Fatalf("ConceptID/MatchType = %q/%q, want wcvp:concept:hs1/%q (Stufe 2 demotet das native Fall-B-Aggregat zugunsten der WCVP-Art — gewollt, siehe Spec-Entscheidung 6); Note=%q Candidates=%v",
			results[0].ConceptID, results[0].MatchType, domain.MatchAggregateNominate, results[0].Note, results[0].Candidates)
	}
}
