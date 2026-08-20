package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestMatchNames_AggregateFallsBackToNominateConcept is issue #67's largest
// class (96 names). Vegetation data sets routinely write collective species as
// "X aggr.", but a backbone that carries no aggregate taxon then answered
// UNRESOLVABLE — while the very same name without the marker resolved exactly.
// Losing the whole row over a marker is worse for a consumer than being handed
// the nominate concept plus the information that it is coarser than asked.
func TestMatchNames_AggregateFallsBackToNominateConcept(t *testing.T) {
	repo := seededMatchRepo(t)
	const corynephorusConceptID = "wcvp:concept:405825"

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens aggr."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]

	if r.ConceptID != corynephorusConceptID {
		t.Errorf("ConceptID = %q, want the nominate concept %q", r.ConceptID, corynephorusConceptID)
	}
	// The whole point of the class: it must NOT look like an exact hit. A
	// consumer that treats "aggregate resolved to one taxon" as exact would
	// carry the narrowing into its own data unmarked.
	if r.MatchType == domain.MatchExact || r.MatchType == domain.MatchExactAuthor {
		t.Errorf("MatchType = %q, want a distinct aggregate type — an aggregate answered with one taxon must stay visible", r.MatchType)
	}
	if r.MatchType != domain.MatchAggregateNominate {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchAggregateNominate)
	}
	if r.MatchType == domain.MatchAggregateAlias {
		t.Error("MatchType must differ from aggregate_alias: there IS no aggregate taxon here, which is exactly what distinguishes the two")
	}
	if r.Note == "" {
		t.Error("Note = empty, want the coarseness stated in words too")
	}
	if r.Confidence >= 0.90 {
		t.Errorf("Confidence = %v, want below the exact tier (0.90): the answer is knowingly coarser than the query", r.Confidence)
	}
}

// TestMatchNames_AggregateAliasStillWinsOverNominate pins the precedence: when
// the index DOES carry an aggregate taxon, that is the better answer and the
// nominate fallback must not take its place.
func TestMatchNames_AggregateAliasStillWinsOverNominate(t *testing.T) {
	repo := seededMatchRepo(t)
	aggConceptID := seedFestucaOvinaAggregate(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovina agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchAggregateAlias {
		t.Errorf("MatchType = %q, want %q — a real aggregate taxon beats the nominate fallback", r.MatchType, domain.MatchAggregateAlias)
	}
	if r.ConceptID != aggConceptID {
		t.Errorf("ConceptID = %q, want the aggregate concept %q", r.ConceptID, aggConceptID)
	}
}

// TestMatchNames_AggregateWithNoNominateEitherStaysUnresolvable pins that the
// fallback invents nothing: a name whose nominate form is absent too is still
// UNRESOLVABLE, as before.
func TestMatchNames_AggregateWithNoNominateEitherStaysUnresolvable(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Nonexistentus bogus agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if r := results[0]; r.ConceptID != "" || !r.RequiresReview {
		t.Errorf("got ConceptID=%q RequiresReview=%v, want unresolvable", r.ConceptID, r.RequiresReview)
	}
}

// TestMatchNames_LayeredAggregateMarkerIsRecognized pins the second half of the
// class. "X aggr. s. l." — an aggregate additionally qualified sensu lato, how
// 60 EIVE taxa are spelled — was not recognized as an aggregate at all: the
// application tested only the LAST token against its own marker list, and
// canonicalization splits "s. l." into two tokens. The name therefore never
// reached the aggregate path and came back unresolvable.
//
// It is now tested with domain.IsAggregateName, the same predicate the rest of
// the codebase uses, and the layers are peeled one at a time so the closest
// form to what was asked wins.
func TestMatchNames_LayeredAggregateMarkerIsRecognized(t *testing.T) {
	repo := seededMatchRepo(t)
	const corynephorusConceptID = "wcvp:concept:405825"

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens aggr. s. l."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != corynephorusConceptID {
		t.Errorf("ConceptID = %q, want the nominate concept %q — a layered marker must still be seen as an aggregate",
			r.ConceptID, corynephorusConceptID)
	}
	if r.MatchType != domain.MatchAggregateNominate {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchAggregateNominate)
	}
}

// TestMatchNames_AmbiguousNominateIsNotStampedAsResolved pins the worst answer
// this function could give, which its first version did give: classify reports
// an ambiguous TIE as a normal result with an empty ConceptID (its second
// return value means "no candidates at all", not "unresolved"), so the code
// stamped MatchType=aggregate_nominate and a confidence onto a result with no
// concept behind them. Measured live before the fix:
//
//	"Abies alba aggr." -> match_type=aggregate_nominate, confidence=0.75,
//	                      concept_id="", 10 candidates
//
// A consumer treating "match_type is not unresolvable" as "concept_id is safe
// to use" would break or, worse, silently misattribute data.
func TestMatchNames_AmbiguousNominateIsNotStampedAsResolved(t *testing.T) {
	repo := seededMatchRepo(t)
	// Two distinct accepted concepts share "Homonymus testicus", so its
	// nominate form is genuinely ambiguous.
	seedHomonymPair(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Homonymus testicus aggr."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID == "" && r.MatchType != "" {
		t.Errorf("MatchType = %q with an empty ConceptID: a match type must never claim more than the result delivers", r.MatchType)
	}
	if r.ConceptID == "" && r.Confidence != 0 {
		t.Errorf("Confidence = %v with an empty ConceptID, want 0", r.Confidence)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false for an ambiguous nominate, want true")
	}
}

// TestMatchNames_LayeredMarkerFindingAnAggregateReportsAlias pins the second
// lie the first version told. When only the OUTER layer is stripped and the
// remaining spelling is itself an aggregate the index carries, that is an
// aggregate_alias hit — nothing was narrowed. Reporting aggregate_nominate
// there both mislabels it and states the opposite of the truth ("keine
// Sammelart im Index"), and made one and the same concept come back with two
// different types and confidences depending on how many marker layers the
// caller happened to type.
func TestMatchNames_LayeredMarkerFindingAnAggregateReportsAlias(t *testing.T) {
	repo := seededMatchRepo(t)
	aggConceptID := seedFestucaOvinaAggregate(t, repo)

	// "Festuca ovina agg. s. l.": the outer sensu-lato layer is stripped, and
	// the remaining "Festuca ovina agg." IS the seeded aggregate taxon.
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovina agg. s. l."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != aggConceptID {
		t.Fatalf("ConceptID = %q, want the aggregate concept %q", r.ConceptID, aggConceptID)
	}
	if r.MatchType != domain.MatchAggregateAlias {
		t.Errorf("MatchType = %q, want %q — the index DOES carry this aggregate, so nothing was narrowed",
			r.MatchType, domain.MatchAggregateAlias)
	}
	if r.Note == noteAggregateNominateForTest {
		t.Error("the note claims there is no aggregate in the index, but one was found")
	}
}

// noteAggregateNominateForTest mirrors the production note so the assertion
// above fails loudly if the wrong one is attached.
const noteAggregateNominateForTest = "Aggregat: keine Sammelart im Index, aufgelöst auf das Nominal-Konzept — deckt weniger ab als die Anfrage"

// TestAggregateNominateConfidenceOutranksFuzzy pins the ordering the numbers
// exist to express: this type is a CERTAIN concept, a fuzzy result is an
// unreviewed guess whose confidence is its similarity score and therefore
// always >= domain.FuzzyThreshold. Ranking the certainty below the guess would
// make any uniform auto-accept threshold take the guesses and reject the
// certainties — which the first draft (0.75) did.
func TestAggregateNominateConfidenceOutranksFuzzy(t *testing.T) {
	repo := seededMatchRepo(t)
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens aggr."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	got := results[0].Confidence
	if got <= domain.FuzzyThreshold {
		t.Errorf("confidence %v <= FuzzyThreshold %v: a certain concept must outrank an unreviewed fuzzy guess",
			got, domain.FuzzyThreshold)
	}
	if got >= 0.90 {
		t.Errorf("confidence %v >= the exact tier (0.90): the answer is still coarser than the query", got)
	}
}
