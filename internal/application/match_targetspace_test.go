package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// addFloraVeg upserts a "floraveg" name space (redistribution unknown, exactly
// as the real ingest records it) and attaches the given per-concept entries,
// via the same BeginTraitIngest write path application.IngestNameSpace uses.
// It is the minimal seam a MatchInSpace test needs: a target space that
// repo.NameSpaces reports as ingested, plus the per-concept spellings
// ResolveTargetSpace reads.
func addFloraVeg(t *testing.T, repo *sqlite.DB, entries map[string][]domain.NameSpaceEntry) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", ManifestSHA: "x",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: %v", err)
	}
	for conceptID, es := range entries {
		for _, e := range es {
			if err := tx.AddNameSpaceEntry(conceptID, e); err != nil {
				t.Fatalf("AddNameSpaceEntry(%s): %v", conceptID, err)
			}
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// TestMatchInSpace_AggregateKnown resolves the source document's own example
// — "Festuca ovina agg." — against a floraveg space that carries the
// aggregate as a taxon of its own, and asserts the buildable half of UC4:
// aggregate_policy=known and the ESy-compatible aggregate spelling handed back.
func TestMatchInSpace_AggregateKnown(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedFestucaOvinaAggregate(t, repo)
	addFloraVeg(t, repo, map[string][]domain.NameSpaceEntry{
		conceptID: {
			{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina", Aggregate: false},
			{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true},
		},
	})

	results, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovina agg."},
	}, "floraveg")
	if err != nil {
		t.Fatalf("MatchInSpace: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if r.AggregatePolicy != domain.AggregatePolicyKnown {
		t.Errorf("AggregatePolicy = %q, want %q", r.AggregatePolicy, domain.AggregatePolicyKnown)
	}
	if r.TargetSpaceName != "Festuca ovina aggr." {
		t.Errorf("TargetSpaceName = %q, want %q", r.TargetSpaceName, "Festuca ovina aggr.")
	}
}

// TestMatchInSpace_AggregateUnresolvable resolves an aggregate query onto a
// concept the target space knows ONLY as a microspecies: no aggregate taxon of
// its own. Per the source document this is "not decidable", not "not relevant"
// — coverage must not be distributed onto the microspecies — so the policy is
// unresolvable and NO ESy name is handed back that could be mistaken for the
// aggregate.
func TestMatchInSpace_AggregateUnresolvable(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedFestucaOvinaAggregate(t, repo)
	addFloraVeg(t, repo, map[string][]domain.NameSpaceEntry{
		conceptID: {
			{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina", Aggregate: false},
		},
	})

	results, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovina agg."},
	}, "floraveg")
	if err != nil {
		t.Fatalf("MatchInSpace: unexpected error: %v", err)
	}
	r := results[0]
	if r.AggregatePolicy != domain.AggregatePolicyUnresolvable {
		t.Errorf("AggregatePolicy = %q, want %q", r.AggregatePolicy, domain.AggregatePolicyUnresolvable)
	}
	if r.TargetSpaceName != "" {
		t.Errorf("TargetSpaceName = %q, want empty (must not offer the microspecies as the aggregate)", r.TargetSpaceName)
	}
}

// TestMatchInSpace_PlainSpeciesCarriesNoPolicy pins the third state: a plain
// species is not an aggregate at all, so aggregate_policy is ABSENT (empty),
// never "known" — emitting "known" for every ordinary species would drain the
// field of meaning. The ESy spelling is still handed back.
func TestMatchInSpace_PlainSpeciesCarriesNoPolicy(t *testing.T) {
	repo := seededMatchRepo(t)
	const corynephorusConceptID = "wcvp:concept:405825"
	addFloraVeg(t, repo, map[string][]domain.NameSpaceEntry{
		corynephorusConceptID: {
			{Space: "floraveg", ExtID: "9001", Name: "Corynephorus canescens", Aggregate: false},
		},
	})

	results, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens"},
	}, "floraveg")
	if err != nil {
		t.Fatalf("MatchInSpace: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != corynephorusConceptID {
		t.Fatalf("ConceptID = %q, want %q", r.ConceptID, corynephorusConceptID)
	}
	if r.AggregatePolicy != "" {
		t.Errorf("AggregatePolicy = %q, want empty (plain species carries no policy)", r.AggregatePolicy)
	}
	if r.TargetSpaceName != "Corynephorus canescens" {
		t.Errorf("TargetSpaceName = %q, want %q", r.TargetSpaceName, "Corynephorus canescens")
	}
}

// TestMatchInSpace_UnknownTargetSpaceIsRejected pins that an un-ingested target
// space is rejected by name (the HTTP layer renders this as INVALID_QUERY), not
// silently ignored.
func TestMatchInSpace_UnknownTargetSpaceIsRejected(t *testing.T) {
	repo := seededMatchRepo(t)

	_, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens"},
	}, "germansl")
	if !errors.Is(err, application.ErrUnknownTargetSpace) {
		t.Fatalf("err = %v, want ErrUnknownTargetSpace", err)
	}
}

// TestMatchInSpace_WithoutSpaceMatchesUnchanged pins that MatchInSpace with an
// empty space behaves exactly as MatchNames — the field is opt-in and UC3/UC6,
// which share this endpoint, must see no change.
func TestMatchInSpace_WithoutSpaceMatchesUnchanged(t *testing.T) {
	repo := seededMatchRepo(t)
	reqs := []application.MatchRequest{{ID: "1", Verbatim: "Corynephorus canescens"}}

	withEmpty, err := application.MatchInSpace(context.Background(), repo, reqs, "")
	if err != nil {
		t.Fatalf("MatchInSpace(\"\"): unexpected error: %v", err)
	}
	plain, err := application.MatchNames(context.Background(), repo, reqs)
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(withEmpty[0], plain[0]) {
		t.Errorf("MatchInSpace(\"\") = %+v, want identical to MatchNames = %+v", withEmpty[0], plain[0])
	}
}
