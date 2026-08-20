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
