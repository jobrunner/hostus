package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// fakeSuggestRepo is a minimal output.Repository stub for exercising
// application.Suggest's own logic (validation, limit default/cap, ranking,
// truncation, BackboneVersions assembly) without a real backing store. It
// embeds a nil output.Repository so it satisfies the interface without
// implementing every method; only Suggest and BackboneVersions are ever
// invoked by application.Suggest, and any accidental call to another method
// panics on the nil embed, which is the point (it would mean Suggest grew a
// dependency this test didn't anticipate).
type fakeSuggestRepo struct {
	output.Repository

	suggestItems []domain.SuggestItem
	suggestErr   error
	versions     []domain.BackboneVersion
	versionsErr  error

	gotQ    string
	gotOpts output.SuggestOpts
	called  bool
}

func (f *fakeSuggestRepo) Suggest(_ context.Context, q string, opts output.SuggestOpts) ([]domain.SuggestItem, error) {
	f.called = true
	f.gotQ = q
	f.gotOpts = opts
	return f.suggestItems, f.suggestErr
}

func (f *fakeSuggestRepo) BackboneVersions(context.Context) ([]domain.BackboneVersion, error) {
	return f.versions, f.versionsErr
}

func (f *fakeSuggestRepo) BuildDistributionClosure(context.Context) error {
	return nil
}

func TestSuggest_EmptyQueryReturnsErrEmptyQuery(t *testing.T) {
	cases := []string{"", "   ", "\t\n"}
	for _, q := range cases {
		repo := &fakeSuggestRepo{}
		_, err := application.Suggest(context.Background(), repo, application.SuggestRequest{Q: q, Limit: 5})
		if !errors.Is(err, application.ErrEmptyQuery) {
			t.Errorf("Suggest(Q=%q) error = %v, want ErrEmptyQuery", q, err)
		}
		if repo.called {
			t.Errorf("Suggest(Q=%q) called repo.Suggest, want no repo access for an invalid query", q)
		}
	}
}

func TestSuggest_ForwardsAreaRanksAndEffectiveLimitToRepo(t *testing.T) {
	cases := []struct {
		name      string
		reqLimit  int
		wantLimit int
	}{
		{"zero defaults to 10", 0, 10},
		{"negative defaults to 10", -3, 10},
		{"in-range limit is forwarded unchanged", 7, 7},
		{"over-max limit is capped at 50", 999, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeSuggestRepo{}
			req := application.SuggestRequest{Q: "coryn", Area: "GER", Ranks: []domain.Rank{domain.RankSpecies}, Limit: c.reqLimit}
			if _, err := application.Suggest(context.Background(), repo, req); err != nil {
				t.Fatalf("Suggest: unexpected error: %v", err)
			}
			if !repo.called {
				t.Fatal("repo.Suggest was not called")
			}
			if repo.gotQ != "coryn" {
				t.Errorf("repo.Suggest q = %q, want %q", repo.gotQ, "coryn")
			}
			if repo.gotOpts.Area != "GER" {
				t.Errorf("repo.Suggest opts.Area = %q, want %q", repo.gotOpts.Area, "GER")
			}
			if len(repo.gotOpts.Ranks) != 1 || repo.gotOpts.Ranks[0] != domain.RankSpecies {
				t.Errorf("repo.Suggest opts.Ranks = %v, want [%v]", repo.gotOpts.Ranks, domain.RankSpecies)
			}
			if repo.gotOpts.Limit != c.wantLimit {
				t.Errorf("repo.Suggest opts.Limit = %d, want %d", repo.gotOpts.Limit, c.wantLimit)
			}
		})
	}
}

// TestSuggest_RanksAndTruncatesRepoResults is the app-layer ranking proof
// the brief calls for: the fake repo returns items deliberately UNRANKED
// (in an order domain.RankSuggestions would never produce), and Suggest
// must still return them in ranked order, truncated to the effective
// limit — proving the ordering is applied by Suggest itself, not merely
// passed through from the repo.
func TestSuggest_RanksAndTruncatesRepoResults(t *testing.T) {
	itemA := domain.SuggestItem{ConceptID: "A", Rank: domain.RankSpecies, Status: domain.StatusSynonym, InArea: false, PrefixHit: true, Score: 5}
	itemB := domain.SuggestItem{ConceptID: "B", Rank: domain.RankSpecies, Status: domain.StatusAccepted, InArea: true, PrefixHit: true, Score: 10}
	itemC := domain.SuggestItem{ConceptID: "C", Rank: domain.RankGenus, Status: domain.StatusAccepted, InArea: true, PrefixHit: true, Score: 1}

	repo := &fakeSuggestRepo{suggestItems: []domain.SuggestItem{itemA, itemB, itemC}}

	resp, err := application.Suggest(context.Background(), repo, application.SuggestRequest{Q: "coryn", Limit: 2})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}

	// Expected rank order per domain.RankSuggestions: InArea true before
	// false puts C and B before A; between C and B (both InArea,
	// Accepted), C's Rank (GENUS, ordinal 1) sorts before B's Rank
	// (SPECIES, ordinal 2). So the full ranked order is C, B, A;
	// truncated to Limit=2 that is [C, B] — A must not appear at all.
	if len(resp.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2 (truncated to Limit)", len(resp.Results))
	}
	if resp.Results[0].ConceptID != "C" || resp.Results[1].ConceptID != "B" {
		t.Errorf("Results = %v, %v, want C then B (ranked, not repo order)", resp.Results[0].ConceptID, resp.Results[1].ConceptID)
	}
}

// TestSuggest_FewerResultsThanLimitAreNotPadded checks the truncate step
// tolerates a repo returning fewer items than the effective limit (no
// out-of-range slice, no panic).
func TestSuggest_FewerResultsThanLimitAreNotPadded(t *testing.T) {
	repo := &fakeSuggestRepo{suggestItems: []domain.SuggestItem{
		{ConceptID: "only", Rank: domain.RankSpecies, Status: domain.StatusAccepted, PrefixHit: true},
	}}

	resp, err := application.Suggest(context.Background(), repo, application.SuggestRequest{Q: "coryn", Limit: 10})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(resp.Results))
	}
}

func TestSuggest_BackboneVersionsPopulatedFromRepo(t *testing.T) {
	repo := &fakeSuggestRepo{versions: []domain.BackboneVersion{
		{ID: "wcvp", Version: "2026-06-15"},
		{ID: "other", Version: "1.2.3"},
	}}

	resp, err := application.Suggest(context.Background(), repo, application.SuggestRequest{Q: "coryn"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	want := map[string]string{"wcvp": "2026-06-15", "other": "1.2.3"}
	if len(resp.BackboneVersions) != len(want) {
		t.Fatalf("BackboneVersions = %v, want %v", resp.BackboneVersions, want)
	}
	for id, version := range want {
		if resp.BackboneVersions[id] != version {
			t.Errorf("BackboneVersions[%q] = %q, want %q", id, resp.BackboneVersions[id], version)
		}
	}
}

func TestSuggest_RepoSuggestErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeSuggestRepo{suggestErr: wantErr}

	_, err := application.Suggest(context.Background(), repo, application.SuggestRequest{Q: "coryn"})
	if !errors.Is(err, wantErr) {
		t.Errorf("Suggest error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestSuggest_RepoBackboneVersionsErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeSuggestRepo{versionsErr: wantErr}

	_, err := application.Suggest(context.Background(), repo, application.SuggestRequest{Q: "coryn"})
	if !errors.Is(err, wantErr) {
		t.Errorf("Suggest error = %v, want it to wrap %v", err, wantErr)
	}
}

// TestSuggest_WCVPFixture_AreaRankedAndTruncated is the end-to-end DoD
// scenario against a real, ingested repo (T2's sqlite adapter + T1's
// domain.RankSuggestions via application.Suggest): "coryn" should surface
// the Corynephorus genus and species concepts, the species one InArea for
// "AUT" (present in the fixture's distribution rows), and the response
// must carry the wcvp BackboneVersion.
func TestSuggest_WCVPFixture_AreaRankedAndTruncated(t *testing.T) {
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	ctx := context.Background()
	if _, err := application.Ingest(ctx, ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}
	// application.Ingest does not build distribution_effective itself (the
	// production CLI wiring in internal/app/ingest.go does, once, after all
	// backbones are ingested); Suggest's in_area now reads that closure
	// table, so this direct-Ingest fixture must build it explicitly.
	if err := repo.BuildDistributionClosure(ctx); err != nil {
		t.Fatalf("BuildDistributionClosure: unexpected error: %v", err)
	}

	resp, err := application.Suggest(ctx, repo, application.SuggestRequest{Q: "coryn", Area: "AUT", Limit: 5})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("Results = empty, want at least the Corynephorus concepts")
	}
	if len(resp.Results) > 5 {
		t.Fatalf("len(Results) = %d, want <= 5 (Limit)", len(resp.Results))
	}

	if resp.BackboneVersions["wcvp"] != "2026-06-15" {
		t.Errorf("BackboneVersions[wcvp] = %q, want %q", resp.BackboneVersions["wcvp"], "2026-06-15")
	}

	// Corynephorus canescens (405825) has an AUT distribution row in the
	// fixture, so with Area="AUT" it must be ranked InArea. The genus
	// concept Corynephorus (451295) has no distribution row at all, so it
	// must not be InArea. RankSuggestions puts InArea results first, so
	// the species concept must precede the genus concept in Results.
	var speciesIdx, genusIdx = -1, -1
	for i, r := range resp.Results {
		switch r.Canonical {
		case "Corynephorus canescens":
			speciesIdx = i
			if !r.InArea {
				t.Error("Corynephorus canescens InArea = false, want true for Area=AUT")
			}
		case "Corynephorus":
			genusIdx = i
			if r.InArea {
				t.Error("Corynephorus (genus) InArea = true, want false (no distribution row in the fixture)")
			}
		}
	}
	if speciesIdx == -1 || genusIdx == -1 {
		t.Fatalf("Results = %+v, want both the Corynephorus genus and species concepts", resp.Results)
	}
	if speciesIdx > genusIdx {
		t.Errorf("Corynephorus canescens (InArea) at index %d, genus (not InArea) at %d; want species ranked first", speciesIdx, genusIdx)
	}
}
