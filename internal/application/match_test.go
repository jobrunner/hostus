package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedFestucaOvinaAggregate adds a synthetic aggregate/collective-species
// concept ("Festuca ovina agg.") directly through the Repository port,
// bypassing the WCVP reader — WCVP backbones don't carry aggregate
// concepts, so a real ingest run never produces one; this is data the
// (future) aggregate-vocabulary source would supply. Kept minimal: just
// enough of a Concept for MatchExact to find and MatchNames to resolve to.
func seedFestucaOvinaAggregate(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-agg", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "test-agg:name:festuca-ovina-agg", Canonical: "Festuca ovina agg.", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-agg:concept:festuca-ovina-agg", BackboneID: "test-agg", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

// seedHomonymPair ingests TWO distinct accepted concepts that share the
// exact same canonical name and authorship — a real homonym-like
// situation WCVP itself does not model, but one MatchNames must still
// handle correctly: a query for that canonical+author now classifies
// exact_author against BOTH concepts, and picking either one silently
// would hide a genuine ambiguity from the caller.
func seedHomonymPair(t *testing.T, repo *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-homonym", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	nameA := domain.Name{ID: "test-homonym:name:a", Canonical: "Homonymus testicus", Authorship: "L.", Rank: domain.RankSpecies}
	nameB := domain.Name{ID: "test-homonym:name:b", Canonical: "Homonymus testicus", Authorship: "L.", Rank: domain.RankSpecies}
	conceptAObj := domain.Concept{ID: "test-homonym:concept:a", BackboneID: "test-homonym", AcceptedName: nameA, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	conceptBObj := domain.Concept{ID: "test-homonym:concept:b", BackboneID: "test-homonym", AcceptedName: nameB, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(nameA); err != nil {
		t.Fatalf("UpsertName(a): unexpected error: %v", err)
	}
	if err := tx.UpsertName(nameB); err != nil {
		t.Fatalf("UpsertName(b): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(conceptAObj); err != nil {
		t.Fatalf("UpsertConcept(a): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(conceptBObj); err != nil {
		t.Fatalf("UpsertConcept(b): unexpected error: %v", err)
	}
	if err := tx.LinkName(conceptAObj.ID, nameA.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(a): unexpected error: %v", err)
	}
	if err := tx.LinkName(conceptBObj.ID, nameB.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(b): unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// seedSynonymAndAcceptedSameNameOneConcept ingests ONE concept whose
// accepted name and one synonym share the exact same canonical+author —
// e.g. a re-published homotypic synonym citing an identical author string.
// MatchExact then returns two candidates for that canonical+author, but
// both resolve to the SAME concept, so this must NOT be flagged as
// ambiguous (contrast with seedHomonymPair, which spans two DIFFERENT
// concepts). Returns the one concept ID.
func seedSynonymAndAcceptedSameNameOneConcept(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-dupe-role", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	accepted := domain.Name{ID: "test-dupe-role:name:accepted", Canonical: "Duplicatus samestringus", Authorship: "L.", Rank: domain.RankSpecies}
	synonym := domain.Name{ID: "test-dupe-role:name:synonym", Canonical: "Duplicatus samestringus", Authorship: "L.", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-dupe-role:concept:1", BackboneID: "test-dupe-role", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(accepted); err != nil {
		t.Fatalf("UpsertName(accepted): unexpected error: %v", err)
	}
	if err := tx.UpsertName(synonym); err != nil {
		t.Fatalf("UpsertName(synonym): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, accepted.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(accepted): unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, synonym.ID, "synonym", nil); err != nil {
		t.Fatalf("LinkName(synonym): unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

// seedSileneOtites ingests a single accepted concept "Silene otites" — a
// correctly-spelled name a query for the commonly-mistyped "Silene otitis"
// should fuzzy-resolve to (a single epithet-letter typo, well past
// domain.FuzzyThreshold; see domain.Similarity's doc comment). The real
// WCVP test fixture carries no Silene at all, so this is seeded directly
// via the Repository port, same pattern as seedFestucaOvinaAggregate. It
// also carries a classification (Family "Caryophyllaceae", Task 10's
// TestMatchNames_AlwaysIncludesClassification).
func seedSileneOtites(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-fuzzy", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "test-fuzzy:name:silene-otites", Canonical: "Silene otites", Authorship: "(L.) Wibel", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-fuzzy:concept:silene-otites", BackboneID: "test-fuzzy", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.UpsertClassification(concept.ID, "Caryophyllaceae", "", ""); err != nil {
		t.Fatalf("UpsertClassification: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

// seedSileneFuzzyTie ingests TWO distinct accepted concepts, "Silene
// otites" and "Silene otitas", both exactly one substitution away from a
// query of "Silene otitis" (same rune length, one differing letter each) —
// so both tie for the best domain.Similarity score. Returns both concept
// IDs in ingestion order.
func seedSileneFuzzyTie(t *testing.T, repo *sqlite.DB) (conceptA, conceptB string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-fuzzy-tie", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	nameA := domain.Name{ID: "test-fuzzy-tie:name:a", Canonical: "Silene otites", Rank: domain.RankSpecies}
	nameB := domain.Name{ID: "test-fuzzy-tie:name:b", Canonical: "Silene otitas", Rank: domain.RankSpecies}
	conceptAObj := domain.Concept{ID: "test-fuzzy-tie:concept:a", BackboneID: "test-fuzzy-tie", AcceptedName: nameA, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	conceptBObj := domain.Concept{ID: "test-fuzzy-tie:concept:b", BackboneID: "test-fuzzy-tie", AcceptedName: nameB, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(nameA); err != nil {
		t.Fatalf("UpsertName(a): unexpected error: %v", err)
	}
	if err := tx.UpsertName(nameB); err != nil {
		t.Fatalf("UpsertName(b): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(conceptAObj); err != nil {
		t.Fatalf("UpsertConcept(a): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(conceptBObj); err != nil {
		t.Fatalf("UpsertConcept(b): unexpected error: %v", err)
	}
	if err := tx.LinkName(conceptAObj.ID, nameA.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(a): unexpected error: %v", err)
	}
	if err := tx.LinkName(conceptBObj.ID, nameB.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(b): unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return conceptAObj.ID, conceptBObj.ID
}

func TestMatchNames_FuzzyResolvesTypoToSingleConcept(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedSileneOtites(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Silene otitis"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchFuzzy {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchFuzzy)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true (mandatory on every fuzzy hit, per spec §B.2)")
	}
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if r.Confidence < domain.FuzzyThreshold || r.Confidence >= 1.0 {
		t.Errorf("Confidence = %v, want in [FuzzyThreshold, 1.0)", r.Confidence)
	}
	if len(r.Candidates) != 1 || r.Candidates[0] != "Silene otites" {
		t.Errorf("Candidates = %v, want [%q]", r.Candidates, "Silene otites")
	}
}

func TestMatchNames_FuzzyTieAcrossDistinctConceptsIsAmbiguous(t *testing.T) {
	repo := seededMatchRepo(t)
	seedSileneFuzzyTie(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Silene otitis"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (ambiguous — no type should be assigned)", r.MatchType)
	}
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty (ambiguous — no concept should be picked arbitrarily)", r.ConceptID)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("Candidates = %v, want 2 tied candidate names", r.Candidates)
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

func TestMatchNames_ExactBeatsFuzzy(t *testing.T) {
	repo := seededMatchRepo(t)
	// Also seed a near-miss for Corynephorus canescens (the real fixture's
	// exact accepted name) so a fuzzy candidate genuinely exists — proving
	// the exact hit wins because exact/exact_author is tried FIRST, not
	// merely because nothing fuzzy was available.
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-near-miss", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "test-near-miss:name:corynephorus-canascens", Canonical: "Corynephorus canascens", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-near-miss:concept:corynephorus-canascens", BackboneID: "test-near-miss", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchExact {
		t.Errorf("MatchType = %q, want %q (exact must win over an available fuzzy candidate)", r.MatchType, domain.MatchExact)
	}
	if r.RequiresReview {
		t.Error("RequiresReview = true, want false (an exact match is not a fuzzy match)")
	}
}

// seedThresholdBoundaryPair ingests one accepted concept whose canonical is
// engineered to sit EXACTLY at domain.FuzzyThreshold's similarity to
// queryCanonThresholdBoundary: both are 20 runes long, distinct-lettered
// (a-t, plus u/v/w substitutes) so no cheaper insert/delete realignment is
// possible, differing in exactly 3 aligned positions -> Levenshtein distance
// 3, similarity 1 - 3/20 = 0.85 exactly. This exercises the "is the
// threshold check itself inclusive of the boundary" case that no
// same/very-different length pair in TestSimilarity's table (which never
// lands exactly on 0.85) can reach.
const (
	queryCanonThresholdBoundary     = "Abcdefghijklmnopqrst"
	candidateCanonThresholdBoundary = "Abcdeughijkvmnopqwst"
)

func seedThresholdBoundaryPair(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-threshold", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "test-threshold:name:1", Canonical: candidateCanonThresholdBoundary, Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-threshold:concept:1", BackboneID: "test-threshold", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

func TestMatchNames_FuzzyThresholdIsInclusiveAtExactBoundary(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedThresholdBoundaryPair(t, repo)

	if got := domain.Similarity(domain.Canonicalize(queryCanonThresholdBoundary), domain.Canonicalize(candidateCanonThresholdBoundary)); got != domain.FuzzyThreshold {
		t.Fatalf("test fixture invalid: Similarity = %v, want exactly domain.FuzzyThreshold (%v)", got, domain.FuzzyThreshold)
	}

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: queryCanonThresholdBoundary},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchFuzzy {
		t.Errorf("MatchType = %q, want %q (a similarity exactly AT FuzzyThreshold must still resolve, not be rejected as below it)", r.MatchType, domain.MatchFuzzy)
	}
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if r.Confidence != domain.FuzzyThreshold {
		t.Errorf("Confidence = %v, want exactly domain.FuzzyThreshold (%v)", r.Confidence, domain.FuzzyThreshold)
	}
}

func TestMatchNames_HomonymAtSameStrengthDifferentConceptsFlagsAmbiguous(t *testing.T) {
	repo := seededMatchRepo(t)
	seedHomonymPair(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Homonymus testicus L."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true (two distinct concepts tied at exact_author)")
	}
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty (ambiguous — no concept should be picked arbitrarily)", r.ConceptID)
	}
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty", r.MatchType)
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("Candidates = %v, want 2 tied candidate names", r.Candidates)
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

func TestMatchNames_SameConceptViaMultipleRolesIsNotAmbiguous(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedSynonymAndAcceptedSameNameOneConcept(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Duplicatus samestringus L."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	assertMatchResult(t, r, wantMatch{
		matchType:  domain.MatchExactAuthor,
		confidence: 0.99,
		conceptID:  conceptID,
	})
}

// seededMatchRepo ingests the real WCVP fixture (T5's Ingest use case,
// exercised the same way ingest_test.go does) into a fresh in-memory repo,
// giving MatchNames real candidates to classify rather than a mock.
func seededMatchRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	if _, err := application.Ingest(context.Background(), ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}
	return repo
}

// wantMatch is the subset of a MatchResult one spec-batch case cares about.
type wantMatch struct {
	matchType      domain.MatchType
	confidence     float64
	conceptID      string
	note           string // checked verbatim if non-empty, else just "non-empty" if noteNonEmpty is set
	noteNonEmpty   bool
	requiresReview bool
}

func assertMatchResult(t *testing.T, r application.MatchResult, want wantMatch) {
	t.Helper()
	if r.MatchType != want.matchType {
		t.Errorf("MatchType = %q, want %q", r.MatchType, want.matchType)
	}
	if r.Confidence != want.confidence {
		t.Errorf("Confidence = %v, want %v", r.Confidence, want.confidence)
	}
	if r.ConceptID != want.conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, want.conceptID)
	}
	if r.RequiresReview != want.requiresReview {
		t.Errorf("RequiresReview = %v, want %v", r.RequiresReview, want.requiresReview)
	}
	if want.note != "" && r.Note != want.note {
		t.Errorf("Note = %q, want %q", r.Note, want.note)
	}
	if want.noteNonEmpty && r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

func TestMatchNames_SpecBatch(t *testing.T) {
	repo := seededMatchRepo(t)
	jacobaeaVulgarisConceptID := "wcvp:concept:3082777"
	corynephorusConceptID := "wcvp:concept:405825"
	aggConceptID := seedFestucaOvinaAggregate(t, repo)

	cases := []struct {
		name     string
		verbatim string
		want     wantMatch
	}{
		{
			name:     "synonym query resolves to accepted concept via exact_author",
			verbatim: "Senecio jacobaea L.",
			want:     wantMatch{matchType: domain.MatchExactAuthor, confidence: 0.99, conceptID: jacobaeaVulgarisConceptID},
		},
		{
			name:     "aggregate resolves to the seeded aggregate concept",
			verbatim: "Festuca ovina agg.",
			want:     wantMatch{matchType: domain.MatchAggregateAlias, confidence: 0.95, conceptID: aggConceptID, note: "Aggregat, keine Kleinartauflösung"},
		},
		{
			name:     "typo (no fuzzy) is UNRESOLVABLE",
			verbatim: "Silene otitis",
			want:     wantMatch{requiresReview: true, noteNonEmpty: true},
		},
		{
			name:     "bare name with no author is exact",
			verbatim: "Corynephorus canescens",
			want:     wantMatch{matchType: domain.MatchExact, confidence: 0.90, conceptID: corynephorusConceptID},
		},
		{
			name:     "full name with matching author is exact_author",
			verbatim: "Corynephorus canescens (L.) P.Beauv.",
			want:     wantMatch{matchType: domain.MatchExactAuthor, confidence: 0.99, conceptID: corynephorusConceptID},
		},
	}

	reqs := make([]application.MatchRequest, len(cases))
	for i, c := range cases {
		reqs[i] = application.MatchRequest{ID: c.name, Verbatim: c.verbatim}
	}
	results, err := application.MatchNames(context.Background(), repo, reqs)
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if len(results) != len(reqs) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(reqs))
	}
	byID := make(map[string]application.MatchResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertMatchResult(t, byID[c.name], c.want)
		})
	}
}

func TestMatchNames_AuthorMismatchIsUnresolvableAndListsCandidate(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens (Mill.) Someone"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (UNRESOLVABLE on author mismatch)", r.MatchType)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if len(r.Candidates) != 1 || r.Candidates[0] != "Corynephorus canescens" {
		t.Errorf("Candidates = %v, want [%q] (the canonical-matching candidate whose author didn't classify)", r.Candidates, "Corynephorus canescens")
	}
}

func TestMatchNames_AggregateWithoutKnownConceptIsUnresolvable(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Nonexistentus bogus agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (UNRESOLVABLE)", r.MatchType)
	}
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty", r.ConceptID)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

// TestMatchNames_AggregateTypoFuzzyResolves pins the fix-round-1 gap: a
// typo'd aggregate query ("Festuca ovinaa agg.", one inserted letter) must
// get the same fuzzy chance as a typo'd plain species name once
// repo.MatchExact finds nothing for its exact (marker-included) canonical —
// per spec §B.2, fuzzy is the catch-all once exact/exact_author/AGGREGATE
// all come up empty, not just the first two. The result must still convey
// its aggregate origin (via the Note) even though MatchType becomes
// domain.MatchFuzzy rather than domain.MatchAggregateAlias, and
// RequiresReview must be true regardless.
func TestMatchNames_AggregateTypoFuzzyResolves(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedFestucaOvinaAggregate(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovinaa agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchFuzzy {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchFuzzy)
	}
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true (mandatory on every fuzzy hit, per spec §B.2)")
	}
	if r.Confidence < domain.FuzzyThreshold {
		t.Errorf("Confidence = %v, want >= FuzzyThreshold (%v)", r.Confidence, domain.FuzzyThreshold)
	}
	if !strings.Contains(r.Note, "Aggregat") {
		t.Errorf("Note = %q, want it to still signal the aggregate origin (MatchType alone no longer does, since it's now %q)", r.Note, r.MatchType)
	}
}

// TestMatchNames_AggregateTypoWithNoNearMatchIsUnresolvable pins the other
// direction of the same fix: an aggregate query with no near match at all
// (not just no exact one) must still end up UNRESOLVABLE, unchanged —
// falling through to fuzzy must never manufacture a match out of nothing.
func TestMatchNames_AggregateTypoWithNoNearMatchIsUnresolvable(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Zzzznonexistent zzzzznope agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (UNRESOLVABLE)", r.MatchType)
	}
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty", r.ConceptID)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

func TestMatchNames_EmptyRequestsReturnsEmptyResults(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

// seedAggregateHomonymPair seeds TWO distinct concepts sharing the exact
// same aggregate canonical ("Ambigua aggregata agg.") — the aggregate
// counterpart of seedHomonymPair. Two independent sources (e.g. two
// backbones' collective-species concepts) can legitimately carry the same
// aggregate name, and hostus has no basis for preferring either.
func seedAggregateHomonymPair(t *testing.T, repo *sqlite.DB) (conceptA, conceptB string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-agg-homonym", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	for _, suffix := range []string{"a", "b"} {
		name := domain.Name{ID: "test-agg-homonym:name:" + suffix, Canonical: "Ambigua aggregata agg.", Rank: domain.RankSpecies}
		concept := domain.Concept{ID: "test-agg-homonym:concept:" + suffix, BackboneID: "test-agg-homonym", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(%s): unexpected error: %v", suffix, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept(%s): unexpected error: %v", suffix, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(%s): unexpected error: %v", suffix, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return "test-agg-homonym:concept:a", "test-agg-homonym:concept:b"
}

// TestMatchNames_AggregateResolvingToSeveralConceptsRequiresReview pins the
// aggregate path against the same guard classify and matchFuzzy already
// apply: when the aggregate's canonical resolves to more than one DISTINCT
// concept, hostus must not answer with candidates[0]. Picking the first row
// SQLite happened to return would present a coin flip as a 0.95-confidence
// answer and hide a real ambiguity from the caller.
func TestMatchNames_AggregateResolvingToSeveralConceptsRequiresReview(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptA, conceptB := seedAggregateHomonymPair(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Ambigua aggregata agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty (must not pick one of %q/%q)", r.ConceptID, conceptA, conceptB)
	}
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (nothing was resolved)", r.MatchType)
	}
	if r.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0 (no resolution, no confidence)", r.Confidence)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true (an ambiguous aggregate needs a human)")
	}
	if len(r.Candidates) != 2 {
		t.Errorf("Candidates = %v, want both tied names listed", r.Candidates)
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

// TestMatchNames_AggregateWithSeveralNamesOfOneConceptStillResolves pins the
// other direction: several MatchExact rows that all resolve to the SAME
// concept (here an aggregate's accepted name plus a synonym sharing its
// canonical) are NOT ambiguous — the aggregate_alias result must be
// unchanged, RequiresReview stays false. The guard keys on distinct
// concepts, not on candidate count.
func TestMatchNames_AggregateWithSeveralNamesOfOneConceptStillResolves(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedFestucaOvinaAggregate(t, repo)

	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-agg", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	syn := domain.Name{ID: "test-agg:name:festuca-ovina-agg-syn", Canonical: "Festuca ovina agg.", Rank: domain.RankSpecies}
	if err := tx.UpsertName(syn); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.LinkName(conceptID, syn.ID, "synonym", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovina agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchAggregateAlias {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchAggregateAlias)
	}
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if r.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", r.Confidence)
	}
	if r.RequiresReview {
		t.Error("RequiresReview = true, want false (two names, one concept, is not an ambiguity)")
	}
}

// seededAggregateRepo seeds a fresh in-memory repo with native SPECIES_
// AGGREGATE concepts for "Salsola kali" in BOTH eurosl (spelled "Salsola
// kali agg.") and germansl (spelled "Salsola kali s.l." — a different
// marker spelling, so the two never collide on MatchExact's exact-canonical
// lookup, only on Task 10's name-match key
// domain.Canonicalize(domain.StripAggregateMarkers(...))), each linked to
// the SAME WCVP member, then precomputes+writes their concept_agreement
// pair via application.ComputeConceptAgreement — which is Agreement
// "identical" for identical member sets. A verbatim query for "Salsola kali
// agg." then resolves (via MatchExact) uniquely to the eurosl concept.
func seededAggregateRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:salsola-kali-agg", "Salsola kali agg.", []string{"wcvp:concept:salsola-kali-1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:salsola-kali-agg", "Salsola kali s.l.", []string{"wcvp:concept:salsola-kali-1"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if err := repo.WriteConceptAgreement(context.Background(), report.Pairs); err != nil {
		t.Fatalf("WriteConceptAgreement: unexpected error: %v", err)
	}
	return repo
}

func TestMatchNames_AlwaysIncludesClassification(t *testing.T) {
	repo := seededMatchRepo(t)
	seedSileneOtites(t, repo)
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Silene otites"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if results[0].Classification.Family != "Caryophyllaceae" {
		t.Errorf("Classification.Family = %q, want %q", results[0].Classification.Family, "Caryophyllaceae")
	}
}

func TestMatchNames_AggregateHitCarriesResolutionAcrossSpaces(t *testing.T) {
	repo := seededAggregateRepo(t)
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Salsola kali agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	res := results[0].AggregateResolution
	if res == nil {
		t.Fatal("AggregateResolution = nil, want non-nil for an aggregate match")
	}
	if len(res.Options) != 3 {
		t.Fatalf("len(Options) = %d, want 3 (eurosl, germansl, wcvp)", len(res.Options))
	}
	if res.Agreement != domain.AgreementIdentical {
		t.Errorf("Agreement = %q, want %q", res.Agreement, domain.AgreementIdentical)
	}
}

// TestMatchNames_AggregateResolutionMixedCaseMarkerStillMatches is the
// end-to-end regression for the Task 10 fix-round-1 finding: a query whose
// aggregate marker carries a non-lowercase letter ("agG." rather than the
// stored "agg.") must still resolve the SAME AggregateResolution a
// lowercase query would. The marker's FIRST letter is deliberately kept
// lowercase ("agG.", not "AGG.") — splitVerbatim's own heuristic treats a
// token STARTING with an uppercase letter as the beginning of the author
// citation, not the canonical, so an all-caps marker never reaches
// isAggregate/buildAggregateResolution as part of the canonical at all
// (that is splitVerbatim's existing, unrelated rule — not this fix's
// concern). A marker whose first letter is lowercase but some OTHER letter
// is not (a real possibility: inconsistent casing in a pasted/OCR'd name
// list) DOES reach buildAggregateResolution's raw `canonical`, unaltered by
// any earlier canonicalization — that is the case this test exercises.
//
// Before the fix, this was accidentally fine only because both call sites
// happened to canonicalize before stripping; the fix makes that guaranteed
// (via the single aggregateMatchKey function) rather than accidental.
func TestMatchNames_AggregateResolutionMixedCaseMarkerStillMatches(t *testing.T) {
	repo := seededAggregateRepo(t)
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Salsola kali agG."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	res := results[0].AggregateResolution
	if res == nil {
		t.Fatal("AggregateResolution = nil, want non-nil for an aggregate match with a mixed-case marker")
	}
	if res.RequestedNameSpace != "eurosl" {
		t.Errorf("RequestedNameSpace = %q, want %q", res.RequestedNameSpace, "eurosl")
	}
	if res.Status != domain.AggregatePolicyKnown {
		t.Errorf("Status = %q, want %q", res.Status, domain.AggregatePolicyKnown)
	}
	if res.Agreement != domain.AgreementIdentical {
		t.Errorf("Agreement = %q, want %q (mixed-case marker must still name-match the germansl aggregate)", res.Agreement, domain.AgreementIdentical)
	}
}
