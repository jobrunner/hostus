package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedHomonymWithOneGenuineBearer ingests the shape a vocabulary name actually
// has in WCVP when it is a HOMONYM: two name rows spelling the same canonical
// with DIFFERENT authorship, each linked to its own concept — one of them the
// concept that genuinely bears the name, the other not.
//
// bearerRole/bearerHomotypic describe the winning link, otherRole the losing
// one, so one fixture covers both real shapes:
//
//   - "Abies alba": accepted under one concept, synonym under another
//     (decided by the accepted tier).
//   - "Inula hirta": synonym under BOTH, homotypic under only one — L. is
//     homotypic under Pentanema hirtum, Pollich's homonym is not (decided by
//     the homotypic tier).
//
// Returns (bearer, other) concept ids.
func seedHomonymWithOneGenuineBearer(t *testing.T, repo *sqlite.DB, bearerRole string, bearerHomotypic *bool, otherRole string) (bearer, other string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-bearer", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	const shared = "Sharedus testicus"
	// Each concept needs an accepted name of its own; when the shared name is
	// the bearer's accepted name, that IS its accepted name.
	bearerAccepted := domain.Name{ID: "test-bearer:name:bearer-acc", Canonical: "Bearerus acceptus", Authorship: "Kunth", Rank: domain.RankSpecies}
	otherAccepted := domain.Name{ID: "test-bearer:name:other-acc", Canonical: "Otherus acceptus", Authorship: "Kunth", Rank: domain.RankSpecies}
	sharedBearer := domain.Name{ID: "test-bearer:name:shared-l", Canonical: shared, Authorship: "L.", Rank: domain.RankSpecies}
	sharedOther := domain.Name{ID: "test-bearer:name:shared-pollich", Canonical: shared, Authorship: "Pollich", Rank: domain.RankSpecies}

	if bearerRole == "accepted" {
		bearerAccepted = sharedBearer
	}
	if otherRole == "accepted" {
		otherAccepted = sharedOther
	}

	bearerConcept := domain.Concept{ID: "test-bearer:concept:bearer", BackboneID: "test-bearer", AcceptedName: bearerAccepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	otherConcept := domain.Concept{ID: "test-bearer:concept:other", BackboneID: "test-bearer", AcceptedName: otherAccepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}

	names := []domain.Name{bearerAccepted, otherAccepted, sharedBearer, sharedOther}
	for i, n := range names {
		if err := tx.UpsertName(n); err != nil {
			t.Fatalf("UpsertName(%d): unexpected error: %v", i, err)
		}
	}
	if err := tx.UpsertConcept(bearerConcept); err != nil {
		t.Fatalf("UpsertConcept(bearer): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(otherConcept); err != nil {
		t.Fatalf("UpsertConcept(other): unexpected error: %v", err)
	}
	// Each concept's own accepted link first, then the shared name's link.
	for _, l := range []struct {
		concept string
		name    string
		role    string
		hom     *bool
	}{
		{bearerConcept.ID, bearerAccepted.ID, "accepted", nil},
		{otherConcept.ID, otherAccepted.ID, "accepted", nil},
		{bearerConcept.ID, sharedBearer.ID, bearerRole, bearerHomotypic},
		{otherConcept.ID, sharedOther.ID, otherRole, nil},
	} {
		if err := tx.LinkName(l.concept, l.name, l.role, l.hom); err != nil {
			t.Fatalf("LinkName(%s): unexpected error: %v", l.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return bearerConcept.ID, otherConcept.ID
}

func traitCount(t *testing.T, repo *sqlite.DB, conceptID string) int {
	t.Helper()
	sets, err := repo.Traits(context.Background(), conceptID, nil)
	if err != nil {
		t.Fatalf("Traits(%q): unexpected error: %v", conceptID, err)
	}
	n := 0
	for _, s := range sets {
		n += len(s.Values)
	}
	return n
}

// TestIngestTraits_HomonymDecidedByTheAcceptedBearer is the "Abies alba" shape.
// EIVE carries indicator values for Abies alba; WCVP holds that canonical under
// two concepts, one as its ACCEPTED name and one as a synonym. The crosswalk
// used to call that ambiguous and drop the row, so a very common tree got no
// traits at all — while /v1/match resolves the same name cleanly, because the
// runtime has a tiered tie-break the ingest did not use.
func TestIngestTraits_HomonymDecidedByTheAcceptedBearer(t *testing.T) {
	repo := seededMatchRepo(t)
	bearer, other := seedHomonymWithOneGenuineBearer(t, repo, "accepted", nil, "synonym")

	report, err := application.IngestTraits(context.Background(), repo, fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
	}}, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	if report.Matched != 1 || report.Ambiguous != 0 {
		t.Errorf("Matched/Ambiguous = %d/%d, want 1/0: the concept holding the name as ACCEPTED is what the name denotes", report.Matched, report.Ambiguous)
	}
	if got := traitCount(t, repo, bearer); got != 1 {
		t.Errorf("traits on the accepted bearer = %d, want 1", got)
	}
	if got := traitCount(t, repo, other); got != 0 {
		t.Errorf("traits on the losing homonym = %d, want 0", got)
	}
}

// TestIngestTraits_HomonymDecidedByTheHomotypicBearer is the "Inula hirta"
// shape, and the case that prompted this: the name is a SYNONYM under both
// concepts, homotypic under only one. "Inula hirta L." is homotypic under
// Pentanema hirtum; "Inula hirta Pollich" is not, under P. britannica. EIVE's
// five dimensions for Inula hirta were dropped entirely.
func TestIngestTraits_HomonymDecidedByTheHomotypicBearer(t *testing.T) {
	repo := seededMatchRepo(t)
	homotypic := true
	bearer, other := seedHomonymWithOneGenuineBearer(t, repo, "synonym", &homotypic, "synonym")

	report, err := application.IngestTraits(context.Background(), repo, fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
	}}, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	if report.Matched != 1 || report.Ambiguous != 0 {
		t.Errorf("Matched/Ambiguous = %d/%d, want 1/0: only one concept bears the name homotypically", report.Matched, report.Ambiguous)
	}
	if got := traitCount(t, repo, bearer); got != 1 {
		t.Errorf("traits on the homotypic bearer = %d, want 1", got)
	}
	if got := traitCount(t, repo, other); got != 0 {
		t.Errorf("traits on the heterotypic homonym = %d, want 0", got)
	}
}

// TestIngestTraits_ReportsHowOftenTheTieBreakDecided keeps the tie-break
// visible in the ingest report. It resolves rows a stricter reading would have
// dropped, and it does so by picking one of several concepts — the kind of
// decision this crosswalk otherwise refuses to make silently. A count is what
// lets an operator see the difference between "resolved cleanly" and "resolved
// because one candidate bore the name and the others did not".
func TestIngestTraits_ReportsHowOftenTheTieBreakDecided(t *testing.T) {
	repo := seededMatchRepo(t)
	seedHomonymWithOneGenuineBearer(t, repo, "accepted", nil, "synonym")
	seedHomonymPair(t, repo) // genuinely ambiguous: accepted under BOTH

	report, err := application.IngestTraits(context.Background(), repo, fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
		{Taxon: "Homonymus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
	}}, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	if report.TieBroken != 1 {
		t.Errorf("report.TieBroken = %d, want 1 (only the homonym with a single genuine bearer)", report.TieBroken)
	}
	// TieBroken counts a SUBSET of Matched, so the documented invariant holds.
	if got, want := report.Matched+report.Unmatched+report.Ambiguous, report.Rows; got != want {
		t.Errorf("Matched+Unmatched+Ambiguous = %d, want %d (= Rows)", got, want)
	}
	if report.Matched != 2 || report.Ambiguous != 1 {
		t.Errorf("Matched/Ambiguous = %d/%d, want 2/1", report.Matched, report.Ambiguous)
	}
}
