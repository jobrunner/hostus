package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// fakeSynonymsRepo is a minimal output.Repository stub, in the same style as
// fakeSuggestRepo: it embeds a nil output.Repository so any method
// application.Synonyms was not expected to call panics rather than
// silently succeeding.
type fakeSynonymsRepo struct {
	output.Repository

	candidates []domain.SynonymCandidate
	err        error

	gotConceptID string
	calls        int
}

func (f *fakeSynonymsRepo) SynonymCandidates(_ context.Context, conceptID string) ([]domain.SynonymCandidate, error) {
	f.calls++
	f.gotConceptID = conceptID
	return f.candidates, f.err
}

func synonymBoolPtr(b bool) *bool { return &b }

// uc5Candidates mirrors the measured shape of Corynephorus canescens
// (wcvp:concept:405825) in miniature: a basionym, two further homotypic
// recombinations, one synonym of unknown typification, three nomenclatural
// defects, one unclassifiable status and two infraspecific ranks. The name
// ids are deliberately NOT in ranked order, so a test that expects the
// basionym first is testing the ranking rather than the input order.
func uc5Candidates() []domain.SynonymCandidate {
	return []domain.SynonymCandidate{
		{NameID: "n-01-aira-breviculmis", Canonical: "aira breviculmis", Rank: domain.RankSpecies},
		{NameID: "n-02-aira-canescens", Canonical: "aira canescens", Authorship: "L.", Rank: domain.RankSpecies, Homotypic: synonymBoolPtr(true), IsBasionym: true},
		{NameID: "n-03-aira-triflora", Canonical: "aira triflora", Rank: domain.RankSpecies, NomStatus: ", pro syn."},
		{NameID: "n-04-avena-canescens", Canonical: "avena canescens", Rank: domain.RankSpecies, Homotypic: synonymBoolPtr(true)},
		{NameID: "n-05-incanescens", Canonical: "corynephorus incanescens", Authorship: "Bubani", Rank: domain.RankSpecies, NomStatus: ", nom. illeg. superfl."},
		{NameID: "n-06-var-andinus", Canonical: "corynephorus canescens var. andinus", Rank: domain.RankVariety, NomStatus: ", nom. nud."},
		{NameID: "n-07-var-montana", Canonical: "corynephorus canescens var. montana", Rank: domain.RankVariety},
		{NameID: "n-08-f-pallidus", Canonical: "corynephorus canescens f. pallidus", Rank: domain.RankForm},
		{NameID: "n-09-sensu-auct", Canonical: "corynephorus fallax", Rank: domain.RankSpecies, NomStatus: ", sensu auct."},
		{NameID: "n-10-weingaertneria", Canonical: "weingaertneria canescens", Rank: domain.RankSpecies, Homotypic: synonymBoolPtr(true)},
	}
}

func nameIDs(rel []domain.SynonymRelevance) []string {
	out := make([]string, len(rel))
	for i, r := range rel {
		out[i] = r.Candidate.NameID
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func runSynonyms(t *testing.T, req application.SynonymsRequest) application.SynonymsResult {
	t.Helper()
	repo := &fakeSynonymsRepo{candidates: uc5Candidates()}
	res, err := application.Synonyms(context.Background(), repo, req)
	if err != nil {
		t.Fatalf("Synonyms(%+v): unexpected error: %v", req, err)
	}
	return res
}

func TestSynonyms_DefaultRelevanceIsAll(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1"})

	if res.Relevance != application.RelevanceAll {
		t.Errorf("Relevance = %q, want %q — the unfiltered list must be the default", res.Relevance, application.RelevanceAll)
	}
	if len(res.Synonyms) != 10 {
		t.Fatalf("got %d synonyms, want all 10", len(res.Synonyms))
	}
	assertSummaryReconciles(t, res)
}

// TestSynonyms_PublicationDiffersFromUnfiltered is the endpoint's reason to
// exist: the same concept, the same request but for one parameter, and a
// materially shorter list.
func TestSynonyms_PublicationDiffersFromUnfiltered(t *testing.T) {
	all := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", PublicationRank: "species"})
	pub := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Relevance: "publication", PublicationRank: "species"})

	if len(all.Synonyms) != 10 {
		t.Fatalf("unfiltered returned %d synonyms, want 10", len(all.Synonyms))
	}
	want := []string{"n-02-aira-canescens", "n-04-avena-canescens", "n-10-weingaertneria", "n-01-aira-breviculmis"}
	if got := nameIDs(pub.Synonyms); !equalIDs(got, want) {
		t.Errorf("publication list = %v, want %v", got, want)
	}
	if pub.Synonyms[0].Candidate.NameID != "n-02-aira-canescens" || !pub.Synonyms[0].Candidate.IsBasionym {
		t.Errorf("publication list does not lead with the basionym: %+v", pub.Synonyms[0].Candidate)
	}
}

// TestSynonyms_UnfilteredCarriesTheSameJudgements pins that `relevance=all`
// is not a dumber answer: every entry still states whether it is
// publishable and why it was withheld.
func TestSynonyms_UnfilteredCarriesTheSameJudgements(t *testing.T) {
	all := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", PublicationRank: "species"})

	byID := map[string]domain.SynonymRelevance{}
	for _, r := range all.Synonyms {
		byID[r.Candidate.NameID] = r
	}
	incanescens := byID["n-05-incanescens"]
	if incanescens.Publishable {
		t.Errorf("incanescens is publishable under relevance=all; it must still be judged")
	}
	if incanescens.Exclusion != domain.ExclusionNomStatus {
		t.Errorf("incanescens Exclusion = %q, want %q", incanescens.Exclusion, domain.ExclusionNomStatus)
	}
	if !strings.Contains(incanescens.Reason, "superfl") {
		t.Errorf("incanescens Reason = %q, want it to name the matched token", incanescens.Reason)
	}
	if got := byID["n-07-var-montana"].Exclusion; got != domain.ExclusionRank {
		t.Errorf("var. montana Exclusion = %q, want %q", got, domain.ExclusionRank)
	}
	if got := byID["n-09-sensu-auct"].Exclusion; got != domain.ExclusionUnclassifiedStatus {
		t.Errorf("sensu auct. Exclusion = %q, want %q", got, domain.ExclusionUnclassifiedStatus)
	}
}

// TestSynonyms_SummaryCountsWhatWasRemoved is the visibility requirement: a
// response that carries two rows must still say that ten existed and which
// rule took each of the other eight.
//
// It runs WITH Max on purpose. Without it the reconciliation below cannot
// observe truncation accounting at all: folding Truncated into Excluded
// (`Excluded["truncated"] = Truncated`) is the mutant that breaks
// publishable + Sum(excluded) == Total on every capped response, and an
// uncapped request leaves it alive. The invariant this endpoint exists to
// guarantee has to be pinned on the path that can violate it.
func TestSynonyms_SummaryCountsWhatWasRemoved(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Relevance: "publication", PublicationRank: "species", Max: 2})

	if res.Truncated != 2 {
		t.Fatalf("Truncated = %d, want 2 — this test is worthless unless the response is actually capped", res.Truncated)
	}
	if len(res.Synonyms) != 2 {
		t.Fatalf("got %d synonyms, want 2", len(res.Synonyms))
	}

	if res.Summary.Total != 10 {
		t.Errorf("Summary.Total = %d, want 10 (every synonym, not just the returned ones)", res.Summary.Total)
	}
	if res.Summary.Publishable != 4 {
		t.Errorf("Summary.Publishable = %d, want 4", res.Summary.Publishable)
	}
	if got := res.Summary.Excluded[domain.ExclusionNomStatus]; got != 3 {
		t.Errorf("Excluded[nom_status] = %d, want 3 (pro syn., nom. illeg. superfl., nom. nud.)", got)
	}
	if got := res.Summary.Excluded[domain.ExclusionRank]; got != 2 {
		t.Errorf("Excluded[rank] = %d, want 2 (var. montana, f. pallidus)", got)
	}
	if got := res.Summary.Excluded[domain.ExclusionUnclassifiedStatus]; got != 1 {
		t.Errorf("Excluded[unclassified_nom_status] = %d, want 1", got)
	}
	if len(res.Summary.UnclassifiedStatuses) != 1 || res.Summary.UnclassifiedStatuses[0] != ", sensu auct." {
		t.Errorf("UnclassifiedStatuses = %v, want [\", sensu auct.\"]", res.Summary.UnclassifiedStatuses)
	}
	// The counters must reconcile: nothing may vanish unaccounted for, and
	// nothing may be counted twice. Truncation is deliberately NOT an
	// exclusion — a capped synonym was not judged irrelevant — so folding it
	// into Excluded would both mis-state the reason and break this sum.
	assertSummaryReconciles(t, res)
}

// assertSummaryReconciles checks the two arithmetic invariants the exclusion
// summary owes its readers on EVERY response:
//
//	publishable + Sum(excluded) == total        (nothing vanishes, nothing double-counted)
//	returned + truncated        == publishable  (under relevance=publication)
//
// plus the rule that truncation never appears as an exclusion reason.
func assertSummaryReconciles(t *testing.T, res application.SynonymsResult) {
	t.Helper()
	if _, ok := res.Summary.Excluded[domain.SynonymExclusion("truncated")]; ok {
		t.Errorf("Excluded carries a %q key: truncation is not an exclusion reason and must not be counted as one (%v)", "truncated", res.Summary.Excluded)
	}
	sum := res.Summary.Publishable
	for _, n := range res.Summary.Excluded {
		sum += n
	}
	if sum != res.Summary.Total {
		t.Errorf("publishable + excluded = %d, want Total = %d (excluded = %v)", sum, res.Summary.Total, res.Summary.Excluded)
	}
	if res.Relevance == application.RelevancePublication {
		if got := len(res.Synonyms) + res.Truncated; got != res.Summary.Publishable {
			t.Errorf("returned + truncated = %d, want Publishable = %d", got, res.Summary.Publishable)
		}
	}
}

// TestSynonyms_MaxTruncatesAfterRanking is the one that would catch a
// truncate-then-rank implementation: the first three CANDIDATES by name id
// are n-01/n-02/n-03, of which only two are publishable at all, so a
// pre-ranking cap could never produce the three best.
func TestSynonyms_MaxTruncatesAfterRanking(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{
		ConceptID: "c-1", Relevance: "publication", PublicationRank: "species", Max: 3,
	})

	want := []string{"n-02-aira-canescens", "n-04-avena-canescens", "n-10-weingaertneria"}
	if got := nameIDs(res.Synonyms); !equalIDs(got, want) {
		t.Errorf("max=3 list = %v, want %v", got, want)
	}
	if res.Truncated != 1 {
		t.Errorf("Truncated = %d, want 1", res.Truncated)
	}
	if res.Summary.Total != 10 || res.Summary.Publishable != 4 {
		t.Errorf("truncation changed the summary: %+v — it must describe the concept, not the page", res.Summary)
	}
	assertSummaryReconciles(t, res)
}

func TestSynonyms_MaxAboveTheListLengthTruncatesNothing(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Max: 100})

	if len(res.Synonyms) != 10 {
		t.Errorf("got %d synonyms, want 10", len(res.Synonyms))
	}
	if res.Truncated != 0 {
		t.Errorf("Truncated = %d, want 0", res.Truncated)
	}
}

func TestSynonyms_MaxZeroMeansNoTruncation(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Max: 0})

	if len(res.Synonyms) != 10 {
		t.Errorf("max=0 returned %d synonyms, want all 10 (0 means no cap, documented)", len(res.Synonyms))
	}
}

func TestSynonyms_RejectsMaxOutOfRange(t *testing.T) {
	for _, max := range []int{-1, application.MaxSynonymLimit + 1} {
		repo := &fakeSynonymsRepo{candidates: uc5Candidates()}
		_, err := application.Synonyms(context.Background(), repo, application.SynonymsRequest{ConceptID: "c-1", Max: max})
		if !errors.Is(err, application.ErrInvalidMax) {
			t.Errorf("Synonyms(max=%d) error = %v, want ErrInvalidMax", max, err)
		}
		if repo.calls != 0 {
			t.Errorf("Synonyms(max=%d) hit the repository %d times; an absurd cap must be refused before any read", max, repo.calls)
		}
	}
}

func TestSynonyms_MaxAtTheLimitIsAccepted(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Max: application.MaxSynonymLimit})

	if len(res.Synonyms) != 10 {
		t.Errorf("got %d synonyms, want 10", len(res.Synonyms))
	}
}

func TestSynonyms_RejectsUnknownRelevanceNamingIt(t *testing.T) {
	repo := &fakeSynonymsRepo{candidates: uc5Candidates()}
	_, err := application.Synonyms(context.Background(), repo, application.SynonymsRequest{ConceptID: "c-1", Relevance: "wichtig"})

	if !errors.Is(err, application.ErrInvalidRelevance) {
		t.Fatalf("error = %v, want ErrInvalidRelevance", err)
	}
	if !strings.Contains(err.Error(), `"wichtig"`) {
		t.Errorf("error %q does not name the offending value", err)
	}
	if repo.calls != 0 {
		t.Errorf("repository was read %d times despite an invalid request", repo.calls)
	}
}

// TestSynonyms_RejectsValidButUnsupportedRank is the deliberate refusal
// documented on PublicationRankSpecies: GENUS parses as a taxon rank, but
// UC5 defines no exclusion set for it, and silently returning an
// unfiltered-by-rank list to a caller who asked for a filter is the failure
// mode this rejects.
func TestSynonyms_RejectsValidButUnsupportedRank(t *testing.T) {
	for _, rank := range []string{"genus", "variety", "quatsch"} {
		repo := &fakeSynonymsRepo{candidates: uc5Candidates()}
		_, err := application.Synonyms(context.Background(), repo, application.SynonymsRequest{ConceptID: "c-1", PublicationRank: rank})
		if !errors.Is(err, application.ErrInvalidPublicationRank) {
			t.Errorf("Synonyms(rank=%q) error = %v, want ErrInvalidPublicationRank", rank, err)
		}
		if !strings.Contains(err.Error(), rank) {
			t.Errorf("error %q does not name the offending value %q", err, rank)
		}
	}
}

func TestSynonyms_AcceptsRelevanceAndRankCaseInsensitively(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Relevance: "PUBLICATION", PublicationRank: " Species "})

	if res.Relevance != application.RelevancePublication {
		t.Errorf("Relevance = %q, want %q", res.Relevance, application.RelevancePublication)
	}
	if res.PublicationRank != application.PublicationRankSpecies {
		t.Errorf("PublicationRank = %q, want %q", res.PublicationRank, application.PublicationRankSpecies)
	}
	if len(res.Synonyms) != 4 {
		t.Errorf("got %d synonyms, want 4", len(res.Synonyms))
	}
}

// TestSynonyms_WithoutRankNothingIsExcludedByRank covers the caller
// publishing a full infraspecific treatment: no `rank`, no rank exclusions.
func TestSynonyms_WithoutRankNothingIsExcludedByRank(t *testing.T) {
	res := runSynonyms(t, application.SynonymsRequest{ConceptID: "c-1", Relevance: "publication"})

	if got := res.Summary.Excluded[domain.ExclusionRank]; got != 0 {
		t.Errorf("Excluded[rank] = %d without a rank parameter, want 0", got)
	}
	if res.Summary.Publishable != 6 {
		t.Errorf("Summary.Publishable = %d, want 6 (the two infraspecific names are back)", res.Summary.Publishable)
	}
	if res.PublicationRank != "" {
		t.Errorf("PublicationRank = %q, want empty", res.PublicationRank)
	}
}

func TestSynonyms_ConceptWithoutSynonymsIsEmptyNotError(t *testing.T) {
	repo := &fakeSynonymsRepo{candidates: nil}

	res, err := application.Synonyms(context.Background(), repo, application.SynonymsRequest{ConceptID: "c-empty", Relevance: "publication"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Synonyms) != 0 {
		t.Errorf("got %d synonyms, want 0", len(res.Synonyms))
	}
	if res.Summary.Total != 0 || res.Summary.Publishable != 0 {
		t.Errorf("Summary = %+v, want a zeroed summary", res.Summary)
	}
	if res.Summary.Excluded == nil {
		t.Errorf("Summary.Excluded is nil; SummarizeSynonyms guarantees a non-nil map")
	}
}

func TestSynonyms_PropagatesRepositoryError(t *testing.T) {
	repo := &fakeSynonymsRepo{err: domain.ErrNotFound}

	_, err := application.Synonyms(context.Background(), repo, application.SynonymsRequest{ConceptID: "c-nope"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want domain.ErrNotFound", err)
	}
	if repo.gotConceptID != "c-nope" {
		t.Errorf("repository was asked for %q, want %q", repo.gotConceptID, "c-nope")
	}
}
