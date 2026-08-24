package application_test

import (
	"context"
	"strings"
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
// bearerRole/bearerHomotypic describe the winning link; the losing concept
// always holds the name as a synonym, which is what makes the bearer the
// bearer. One fixture covers both real shapes:
//
//   - "Abies alba": accepted under one concept, synonym under another
//     (decided by the accepted tier).
//   - "Inula hirta": synonym under BOTH, homotypic under only one — L. is
//     homotypic under Pentanema hirtum, Pollich's homonym is not (decided by
//     the homotypic tier).
//
// Returns (bearer, other) concept ids.
func seedHomonymWithOneGenuineBearer(t *testing.T, repo *sqlite.DB, bearerRole string, bearerHomotypic *bool) (bearer, other string) {
	const otherRole = "synonym"
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
	bearer, other := seedHomonymWithOneGenuineBearer(t, repo, "accepted", nil)

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
	bearer, other := seedHomonymWithOneGenuineBearer(t, repo, "synonym", &homotypic)

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
	seedHomonymWithOneGenuineBearer(t, repo, "accepted", nil)
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

// seedSecSpaceHomonym ingests the shape a concept SOURCE (CDM) adds on top of
// a backbone: the same canonical is accepted inside a sec. reference space
// while a backbone concept carries it as a synonym.
//
// Returns (backboneConcept, secConcept).
func seedSecSpaceHomonym(t *testing.T, repo *sqlite.DB) (backboneConcept, secConcept string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-sec-tb", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertSecReference(domain.SecReference{ID: "sec-flora", Title: "Flora sec-flora"}); err != nil {
		t.Fatalf("UpsertSecReference: unexpected error: %v", err)
	}

	const shared = "Secshared testicus"
	bbAccepted := domain.Name{ID: "test-sec-tb:name:bb-acc", Canonical: "Backboneus acceptus", Authorship: "Kunth", Rank: domain.RankSpecies}
	sharedName := domain.Name{ID: "test-sec-tb:name:shared", Canonical: shared, Authorship: "L.", Rank: domain.RankSpecies}
	bb := domain.Concept{ID: "test-sec-tb:concept:backbone", BackboneID: "test-sec-tb", AcceptedName: bbAccepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	sec := domain.Concept{ID: "test-sec-tb:concept:sec", BackboneID: "test-sec-tb", AcceptedName: sharedName, Rank: domain.RankSpecies, Status: domain.StatusAccepted, SecReference: "sec-flora"}

	for _, n := range []domain.Name{bbAccepted, sharedName} {
		if err := tx.UpsertName(n); err != nil {
			t.Fatalf("UpsertName(%s): unexpected error: %v", n.ID, err)
		}
	}
	for _, c := range []domain.Concept{bb, sec} {
		if err := tx.UpsertConcept(c); err != nil {
			t.Fatalf("UpsertConcept(%s): unexpected error: %v", c.ID, err)
		}
	}
	for _, l := range []struct {
		concept, name, role string
		hom                 *bool
	}{
		{bb.ID, bbAccepted.ID, "accepted", nil},
		{sec.ID, sharedName.ID, "accepted", nil},
		{bb.ID, sharedName.ID, "synonym", nil},
	} {
		if err := tx.LinkName(l.concept, l.name, l.role, l.hom); err != nil {
			t.Fatalf("LinkName(%s->%s): unexpected error: %v", l.concept, l.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return bb.ID, sec.ID
}

// TestIngestTraits_NeverAttributesToASecSpaceConcept: a trait value is only
// reachable from the concept id it was written to, so writing it to a concept
// that lives inside a sec. reference space hides it from every consumer
// holding a backbone id — GET /v1/concept/{backbone-id}/traits would return
// nothing while the value sits somewhere else.
//
// This is not hypothetical. On the real index CDM holds "Inula hirta" as
// accepted in seven sec. spaces, and measured against the full name index,
// 194 EIVE / 99 Tichý / 92 Midolo taxa would land on a sec. concept although a
// WCVP concept for the same name exists. What prevents it today is only that
// the manifest happens to list concept_sources AFTER trait_vocabularies — a
// property of one config file, not of the code.
func TestIngestTraits_NeverAttributesToASecSpaceConcept(t *testing.T) {
	repo := seededMatchRepo(t)
	backbone, sec := seedSecSpaceHomonym(t, repo)

	report, err := application.IngestTraits(context.Background(), repo, fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Secshared testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
	}}, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	if got := traitCount(t, repo, sec); got != 0 {
		t.Errorf("traits on the sec.-space concept = %d, want 0: a value there is unreachable from a backbone id", got)
	}
	// The backbone concept holds the name as a synonym and is the only
	// candidate left once sec. concepts are out, so it resolves.
	if got := traitCount(t, repo, backbone); got != 1 {
		t.Errorf("traits on the backbone concept = %d, want 1 (report: matched=%d ambiguous=%d)", got, report.Matched, report.Ambiguous)
	}
}

// TestIngestTraits_ATieBrokenRowNeverDisplacesAnUnambiguousOne is the
// regression for what the tie-break newly makes possible. Two DIFFERENT
// vocabulary taxon names can now resolve to one concept — one of them by a
// clean single-candidate match, the other only because the tie-break picked
// among homonyms. They then contend for the same (concept, dim) slot, and
// ranking them equally would let CSV row order decide whether the vocabulary's
// own unambiguous value or the picked one is stored.
//
// Measured on real data, this is not theoretical: 17 Tichý and 8 Midolo taxa
// (plus EIVE cases) land on a concept another, unambiguous name of the same
// vocabulary already claims. A picked value must never beat a certain one.
func TestIngestTraits_ATieBrokenRowNeverDisplacesAnUnambiguousOne(t *testing.T) {
	// Both orders must give the same answer — if only one does, row order is
	// still deciding.
	for _, tc := range []struct {
		name string
		rows []application.TraitRow
	}{
		{"tie-broken row first", []application.TraitRow{
			{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
			{Taxon: "Bearerus acceptus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 9.0},
		}},
		{"unambiguous row first", []application.TraitRow{
			{Taxon: "Bearerus acceptus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 9.0},
			{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededMatchRepo(t)
			// "Sharedus testicus" is a homonym decided by the homotypic tier;
			// "Bearerus acceptus" is that same concept's accepted name and
			// resolves with no competition at all.
			homotypic := true
			bearer, _ := seedHomonymWithOneGenuineBearer(t, repo, "synonym", &homotypic)

			if _, err := application.IngestTraits(context.Background(), repo, fakeTraitRowSource{rows: tc.rows}, eiveMeta); err != nil {
				t.Fatalf("IngestTraits: unexpected error: %v", err)
			}

			sets, err := repo.Traits(context.Background(), bearer, nil)
			if err != nil {
				t.Fatalf("Traits: unexpected error: %v", err)
			}
			if len(sets) != 1 || len(sets[0].Values) != 1 {
				t.Fatalf("Traits = %v, want exactly one value in one set", sets)
			}
			if got := sets[0].Values[0].Value; got != 9.0 {
				t.Errorf("stored value = %v, want 9 (the unambiguous row's): a picked resolution must not displace a certain one", got)
			}
		})
	}
}

// TestIngestTraits_TieBrokenTaxaAreSampled: every other judgement call this
// report makes is auditable by name — UnmatchedSample says which taxa were
// lost, FlaggedSample which ones had their circumscription equated. The
// tie-break is the one place the crosswalk PICKS a concept, so a bare count
// leaves an operator with 8.638 decisions and no way to check any of them.
func TestIngestTraits_TieBrokenTaxaAreSampled(t *testing.T) {
	repo := seededMatchRepo(t)
	seedHomonymWithOneGenuineBearer(t, repo, "accepted", nil)

	report, err := application.IngestTraits(context.Background(), repo, fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
		{Taxon: "Sharedus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "N", Value: 6.0},
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
	}}, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	// One taxon, two rows: the sample names TAXA, deduplicated, like the
	// other samples in this report.
	if got, want := strings.Join(report.TieBrokenSample, ","), "Sharedus testicus"; got != want {
		t.Errorf("report.TieBrokenSample = %q, want %q", got, want)
	}
	if report.TieBroken != 2 {
		t.Errorf("report.TieBroken = %d, want 2 (rows, not taxa — consistent with the other counters)", report.TieBroken)
	}
}
