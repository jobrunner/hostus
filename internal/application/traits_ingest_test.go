package application_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/traits"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// traitsRowSource adapts a *traits.Dataset (T2's reader output) into
// application.TraitRowSource, the same boundary-respecting bridge
// wcvpRowSource in ingest_test.go uses for backbones — application never
// imports internal/adapters/traits directly (depguard).
type traitsRowSource struct{ ds *traits.Dataset }

func (s traitsRowSource) Rows() []application.TraitRow {
	out := make([]application.TraitRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.TraitRow{
			Taxon:        r.Taxon,
			Vocab:        r.Vocab,
			VocabVersion: r.VocabVersion,
			Dim:          r.Dim,
			Value:        r.Value,
			NicheWidth:   r.NicheWidth,
			NSystems:     r.NSystems,
		})
	}
	return out
}

func loadEIVEFixture(t *testing.T) traitsRowSource {
	t.Helper()
	ds, err := traits.Read("../adapters/traits/testdata/eive-sample.csv")
	if err != nil {
		t.Fatalf("traits.Read(eive-sample.csv): unexpected error: %v", err)
	}
	return traitsRowSource{ds: ds}
}

func loadTichyFixture(t *testing.T) traitsRowSource {
	t.Helper()
	ds, err := traits.Read("../adapters/traits/testdata/tichy-sample.csv")
	if err != nil {
		t.Fatalf("traits.Read(tichy-sample.csv): unexpected error: %v", err)
	}
	return traitsRowSource{ds: ds}
}

var eiveMeta = domain.TraitVocabMeta{
	Vocab:     domain.VocabEIVE,
	Version:   "1.0",
	Taxonomy:  "euromed-via-eurosl",
	License:   "CC-BY-4.0",
	SourceURL: "https://example.org/eive",
}

var tichyMeta = domain.TraitVocabMeta{
	Vocab:     domain.VocabTichy,
	Version:   "2.0",
	Taxonomy:  "floraveg",
	License:   "CC-BY-4.0",
	SourceURL: "https://example.org/tichy",
}

func TestIngestTraits_EIVEFixture_MatchedRowLandsAsTraitValue(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref: unexpected error: %v", err)
	}

	report, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Vocab != string(domain.VocabEIVE) {
		t.Errorf("report.Vocab = %q, want %q", report.Vocab, domain.VocabEIVE)
	}

	sets, err := repo.Traits(ctx, concept.ID, nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("len(Traits) = %d, want 1 (one EIVE TraitSet)", len(sets))
	}
	set := sets[0]
	if set.Vocab != domain.VocabEIVE {
		t.Errorf("set.Vocab = %q, want %q", set.Vocab, domain.VocabEIVE)
	}
	found := false
	for _, v := range set.Values {
		if v.Dim == domain.DimM {
			found = true
			if v.Value != 2.48617710307631 {
				t.Errorf("M value = %v, want %v", v.Value, 2.48617710307631)
			}
			if v.NicheWidth == nil || *v.NicheWidth != 3.42710384303395 {
				t.Errorf("M niche width = %v, want %v", v.NicheWidth, 3.42710384303395)
			}
		}
	}
	if !found {
		t.Error("no M dim value found for Corynephorus canescens; matched EIVE row should have been written")
	}
}

func TestIngestTraits_EIVEFixture_UnmatchedTaxaCountedAndSampled(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	report, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	// Abies alba and Quercus robur are absent from the WCVP fixture
	// (deliberately, per the fixture's design) -- 5 rows each (M/N/R/L/T).
	if report.Unmatched != 10 {
		t.Errorf("report.Unmatched = %d, want %d", report.Unmatched, 10)
	}
	wantSample := []string{"Abies alba", "Quercus robur"}
	sort.Strings(wantSample)
	if !equalStrings(report.UnmatchedSample, wantSample) {
		t.Errorf("report.UnmatchedSample = %v, want %v", report.UnmatchedSample, wantSample)
	}

	// No trait_value must exist for either unmatched taxon: assert via
	// MatchExact that neither name resolves (so there's no concept to even
	// check Traits against), which is exactly why they're unmatched.
	for _, taxon := range wantSample {
		cands, err := repo.MatchExact(ctx, domain.Canonicalize(taxon))
		if err != nil {
			t.Fatalf("MatchExact(%q): unexpected error: %v", taxon, err)
		}
		if len(cands) != 0 {
			t.Fatalf("MatchExact(%q) = %v, want no candidates (fixture design: absent from WCVP)", taxon, cands)
		}
	}
}

func TestIngestTraits_AmbiguousNameSkippedNoConceptWritten(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptA, conceptB := seedHomonymPair(t, repo)
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Homonymus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 5.0},
	}}

	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Ambiguous != 1 {
		t.Errorf("report.Ambiguous = %d, want 1", report.Ambiguous)
	}
	if report.Matched != 0 {
		t.Errorf("report.Matched = %d, want 0 (ambiguous row must not be guessed onto either concept)", report.Matched)
	}
	if report.Unmatched != 0 {
		t.Errorf("report.Unmatched = %d, want 0", report.Unmatched)
	}

	for _, cid := range []string{conceptA, conceptB} {
		sets, err := repo.Traits(ctx, cid, nil)
		if err != nil {
			t.Fatalf("Traits(%q): unexpected error: %v", cid, err)
		}
		if len(sets) != 0 {
			t.Errorf("Traits(%q) = %v, want none (ambiguous row must not write to either homonym concept)", cid, sets)
		}
	}
}

func TestIngestTraits_CountsSumToRows(t *testing.T) {
	repo := seededMatchRepo(t)
	seedHomonymPair(t, repo)
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
		{Taxon: "Abies alba", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
		{Taxon: "Homonymus testicus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
	}}

	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Rows != 3 {
		t.Fatalf("report.Rows = %d, want 3", report.Rows)
	}
	if got, want := report.Matched+report.Unmatched+report.Ambiguous, report.Rows; got != want {
		t.Errorf("Matched+Unmatched+Ambiguous = %d, want %d (= Rows)", got, want)
	}
	if report.Matched != 1 || report.Unmatched != 1 || report.Ambiguous != 1 {
		t.Errorf("Matched/Unmatched/Ambiguous = %d/%d/%d, want 1/1/1", report.Matched, report.Unmatched, report.Ambiguous)
	}
}

func TestIngestTraits_SecondVocabularyDoesNotMergeWithFirst(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref: unexpected error: %v", err)
	}

	if _, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), eiveMeta); err != nil {
		t.Fatalf("IngestTraits(eive): unexpected error: %v", err)
	}
	if _, err := application.IngestTraits(ctx, repo, loadTichyFixture(t), tichyMeta); err != nil {
		t.Fatalf("IngestTraits(tichy): unexpected error: %v", err)
	}

	sets, err := repo.Traits(ctx, concept.ID, nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(Traits) = %d, want 2 (EIVE and Tichy kept as separate TraitSets, never merged)", len(sets))
	}
	gotVocabs := map[domain.TraitVocab]bool{}
	for _, s := range sets {
		gotVocabs[s.Vocab] = true
	}
	if !gotVocabs[domain.VocabEIVE] || !gotVocabs[domain.VocabTichy] {
		t.Errorf("Traits vocabs = %v, want both %q and %q present", gotVocabs, domain.VocabEIVE, domain.VocabTichy)
	}

	vocabs, err := repo.TraitVocabularies(ctx)
	if err != nil {
		t.Fatalf("TraitVocabularies: unexpected error: %v", err)
	}
	if len(vocabs) != 2 {
		t.Errorf("len(TraitVocabularies) = %d, want 2", len(vocabs))
	}
}

func TestIngestTraits_MatchExactErrorPropagates(t *testing.T) {
	repo := seededMatchRepo(t)
	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "NOT-A-DIM", Value: 1.0},
	}}
	if _, err := application.IngestTraits(context.Background(), repo, src, eiveMeta); err == nil {
		t.Fatal("IngestTraits: expected error for an unparseable trait dim, got nil")
	}
}

type fakeTraitRowSource struct{ rows []application.TraitRow }

func (f fakeTraitRowSource) Rows() []application.TraitRow { return f.rows }

// txGuardRepo decorates a real output.Repository and makes the two-phase
// contract of IngestTraits (resolve everything FIRST, then open ONE ingest
// transaction and only write) a hard, deterministic assertion instead of a
// timing observation: every MatchExact issued while an ingest transaction
// is open fails loudly.
//
// This pins a real deadlock, not a style rule. The sqlite adapter runs with
// SetMaxOpenConns(1) (per-connection foreign_keys pragma + shared
// ":memory:" database), so an open IngestTx holds the only connection and a
// read from inside the transaction blocks FOREVER — in "hostus ingest" just
// as much as in a test. A timing/timeout test would only observe the hang;
// this decorator names its cause.
type txGuardRepo struct {
	output.Repository
	txOpen     bool
	matchCalls map[string]int
}

func newTxGuardRepo(inner output.Repository) *txGuardRepo {
	return &txGuardRepo{Repository: inner, matchCalls: map[string]int{}}
}

func (r *txGuardRepo) MatchExact(ctx context.Context, canon string) ([]output.MatchCandidate, error) {
	if r.txOpen {
		return nil, fmt.Errorf("repository read MatchExact(%q) issued while an ingest transaction is open: this deadlocks against SetMaxOpenConns(1)", canon)
	}
	r.matchCalls[canon]++
	return r.Repository.MatchExact(ctx, canon)
}

func (r *txGuardRepo) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	tx, err := r.Repository.BeginIngest(ctx, bv)
	if err != nil {
		return nil, err
	}
	r.txOpen = true
	return &txGuardTx{IngestTx: tx, repo: r}, nil
}

type txGuardTx struct {
	output.IngestTx
	repo *txGuardRepo
}

func (t *txGuardTx) Commit() error {
	t.repo.txOpen = false
	return t.IngestTx.Commit()
}

func (t *txGuardTx) Rollback() error {
	t.repo.txOpen = false
	return t.IngestTx.Rollback()
}

func TestIngestTraits_DoesNoRepositoryReadWhileIngestTxOpen(t *testing.T) {
	repo := newTxGuardRepo(seededMatchRepo(t))

	report, err := application.IngestTraits(context.Background(), repo, loadEIVEFixture(t), eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Matched == 0 {
		t.Fatal("report.Matched = 0; the guard test must exercise the write path, not a no-op ingest")
	}
	if report.Unmatched == 0 {
		t.Fatal("report.Unmatched = 0; the guard test must exercise the unmatched path too")
	}
	if repo.txOpen {
		t.Error("ingest transaction still open after IngestTraits returned")
	}
}

func TestIngestTraits_ResolvesEachDistinctTaxonExactlyOnce(t *testing.T) {
	repo := newTxGuardRepo(seededMatchRepo(t))

	// Three rows, two distinct taxa: the repeated taxon must be resolved
	// from the phase-1 cache, not re-queried per (taxon, dim) row.
	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "N", Value: 2.0},
		{Taxon: "Abies alba", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 3.0},
	}}

	if _, err := application.IngestTraits(context.Background(), repo, src, eiveMeta); err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	want := map[string]int{
		domain.Canonicalize("Corynephorus canescens"): 1,
		domain.Canonicalize("Abies alba"):             1,
	}
	if len(repo.matchCalls) != len(want) {
		t.Fatalf("MatchExact called for %v, want exactly the distinct names %v", repo.matchCalls, want)
	}
	for canon, n := range want {
		if got := repo.matchCalls[canon]; got != n {
			t.Errorf("MatchExact(%q) called %d time(s), want %d", canon, got, n)
		}
	}
}

func TestIngestTraits_ResolveErrorPropagatesWithoutOpeningTx(t *testing.T) {
	repo := &failingMatchRepo{Repository: seededMatchRepo(t)}

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Corynephorus canescens", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
	}}

	if _, err := application.IngestTraits(context.Background(), repo, src, eiveMeta); err == nil {
		t.Fatal("IngestTraits: expected the MatchExact error to propagate, got nil")
	}
	if repo.beganIngest {
		t.Error("BeginIngest was called although resolution had already failed; phase 1 must fully precede phase 2")
	}
}

type failingMatchRepo struct {
	output.Repository
	beganIngest bool
}

func (r *failingMatchRepo) MatchExact(context.Context, string) ([]output.MatchCandidate, error) {
	return nil, errors.New("boom")
}

func (r *failingMatchRepo) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	r.beganIngest = true
	return r.Repository.BeginIngest(ctx, bv)
}
