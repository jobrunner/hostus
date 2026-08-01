package application_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
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

// TestIngestTraits_LeavesNoBackboneVersionRow pins the boundary between a
// trait vocabulary and a taxonomic backbone. IngestTraits used to open its
// write transaction via BeginIngest with a synthetic
// "trait:<vocab>" BackboneVersion, which unconditionally INSERTs into
// backbone_version — those rows are then served verbatim as the
// backbone_versions provenance block of every /v1/suggest and /v1/match
// response, telling clients a trait vocabulary is a backbone.
func TestIngestTraits_LeavesNoBackboneVersionRow(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()

	if _, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), eiveMeta); err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}

	versions, err := repo.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("BackboneVersions = %v, want none: a trait-only ingest must not record ANY backbone", versions)
	}

	// The vocabulary itself must still be recorded — the fix removes the
	// backbone row, not the provenance.
	vocabs, err := repo.TraitVocabularies(ctx)
	if err != nil {
		t.Fatalf("TraitVocabularies: unexpected error: %v", err)
	}
	if len(vocabs) != 1 {
		t.Fatalf("len(TraitVocabularies) = %d, want 1", len(vocabs))
	}
}

// TestIngestTraits_BackboneVersionsHoldOnlyRealBackbones is the same
// guarantee on a realistically populated database: after a real WCVP ingest
// PLUS two trait vocabularies, BackboneVersions must list exactly the
// backbones and nothing whose id looks like "trait:...".
func TestIngestTraits_BackboneVersionsHoldOnlyRealBackbones(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	if _, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), eiveMeta); err != nil {
		t.Fatalf("IngestTraits(eive): unexpected error: %v", err)
	}
	if _, err := application.IngestTraits(ctx, repo, loadTichyFixture(t), tichyMeta); err != nil {
		t.Fatalf("IngestTraits(tichy): unexpected error: %v", err)
	}

	versions, err := repo.BackboneVersions(ctx)
	if err != nil {
		t.Fatalf("BackboneVersions: unexpected error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(BackboneVersions) = %d (%v), want 1 (the WCVP backbone alone)", len(versions), versions)
	}
	if versions[0].ID != "wcvp" {
		t.Errorf("BackboneVersions[0].ID = %q, want %q", versions[0].ID, "wcvp")
	}
	for _, bv := range versions {
		if strings.HasPrefix(bv.ID, "trait:") {
			t.Errorf("BackboneVersions contains synthetic trait entry %q; trait vocabularies are not backbones", bv.ID)
		}
	}
}

// TestIngestTraits_VocabMismatchFailsAndWritesNothing pins the worst
// misconfiguration the (vocab, version) reconciliation exists to stop:
// a manifest entry pinned to one vocabulary pointed at another
// vocabulary's canonical CSV. Without the check, Tichý's 1..12 values would
// be stored under vocab=eive and rendered on EIVE's normalized 0..10 scale
// — an invented scale, and the cross-vocabulary merge PoC P10 forbids.
func TestIngestTraits_VocabMismatchFailsAndWritesNothing(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	// eiveMeta pins vocab "eive", the fixture rows declare "tichy2023".
	_, err := application.IngestTraits(ctx, repo, loadTichyFixture(t), eiveMeta)
	if err == nil {
		t.Fatal("IngestTraits: expected an error for a CSV/manifest vocab mismatch, got nil")
	}
	for _, want := range []string{"tichy2023", "eive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q; it must name BOTH sides of the mismatch", err, want)
		}
	}
	assertNothingIngested(t, repo)
}

// TestIngestTraits_VersionMismatchFailsAndWritesNothing pins the subtler
// half: the vocab agrees but the manifest pins a version the pipeline does
// not emit. Silently accepting it makes Repository.Traits' LEFT JOIN onto
// trait_vocabulary miss, degrading taxonomy to "" on the wire instead of
// failing — this was live in the shipped dataset.example.yaml.
func TestIngestTraits_VersionMismatchFailsAndWritesNothing(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	wrongVersion := eiveMeta
	wrongVersion.Version = "9.9"

	_, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), wrongVersion)
	if err == nil {
		t.Fatal("IngestTraits: expected an error for a CSV/manifest version mismatch, got nil")
	}
	for _, want := range []string{"9.9", "1.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q; it must name BOTH sides of the mismatch", err, want)
		}
	}
	assertNothingIngested(t, repo)
}

// assertNothingIngested asserts a refused trait ingest left the database
// completely untouched — no vocabulary metadata row and no trait values on
// the one concept the fixtures do match.
func assertNothingIngested(t *testing.T, repo *sqlite.DB) {
	t.Helper()
	ctx := context.Background()

	vocabs, err := repo.TraitVocabularies(ctx)
	if err != nil {
		t.Fatalf("TraitVocabularies: unexpected error: %v", err)
	}
	if len(vocabs) != 0 {
		t.Errorf("TraitVocabularies = %v, want none: a refused ingest must write nothing", vocabs)
	}

	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref: unexpected error: %v", err)
	}
	sets, err := repo.Traits(ctx, concept.ID, nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("Traits(%q) = %v, want none: a refused ingest must write nothing", concept.ID, sets)
	}
}

// TestIngestTraits_WritesManifestPinnedVersionOnEveryValue pins that the
// version persisted on a trait_value is the manifest's pinned one — the
// single source of truth reconciled against the CSV — so a value can never
// carry a version trait_vocabulary has no row for.
func TestIngestTraits_WritesManifestPinnedVersionOnEveryValue(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	if _, err := application.IngestTraits(ctx, repo, loadEIVEFixture(t), eiveMeta); err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref: unexpected error: %v", err)
	}
	sets, err := repo.Traits(ctx, concept.ID, nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("len(Traits) = %d, want 1", len(sets))
	}
	if sets[0].VocabVersion != eiveMeta.Version {
		t.Errorf("VocabVersion = %q, want %q (the manifest-pinned version)", sets[0].VocabVersion, eiveMeta.Version)
	}
	// The join onto trait_vocabulary only resolves when the value's version
	// matches the recorded vocabulary's — a non-empty Taxonomy proves it did.
	if sets[0].Taxonomy != eiveMeta.Taxonomy {
		t.Errorf("Taxonomy = %q, want %q (empty means the trait_vocabulary join missed)", sets[0].Taxonomy, eiveMeta.Taxonomy)
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

func (r *txGuardRepo) BeginTraitIngest(ctx context.Context) (output.IngestTx, error) {
	tx, err := r.Repository.BeginTraitIngest(ctx)
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

	// Since Hardening Task 5 a distinct taxon costs one query per key of
	// its domain.NameCandidates ladder — but still exactly ONE walk of that
	// ladder, however many rows carry the name. "Corynephorus canescens"
	// resolves on its exact key and therefore costs exactly one query;
	// "Abies alba" resolves on none, so its full ladder is walked, and each
	// of its keys is queried exactly once.
	for canon, n := range map[string]int{
		domain.Canonicalize("Corynephorus canescens"): 1,
		domain.Canonicalize("Abies alba"):             1,
	} {
		if got := repo.matchCalls[canon]; got != n {
			t.Errorf("MatchExact(%q) called %d time(s), want %d", canon, got, n)
		}
	}
	for _, cand := range domain.NameCandidates("Abies alba") {
		if got := repo.matchCalls[cand.Key]; got != 1 {
			t.Errorf("MatchExact(%q) called %d time(s), want 1 (one ladder walk per distinct taxon)", cand.Key, got)
		}
	}
	// No key beyond the two ladders may have been queried at all: the
	// matched taxon must stop at its exact key rather than walking on.
	wantKeys := map[string]bool{domain.Canonicalize("Corynephorus canescens"): true}
	for _, cand := range domain.NameCandidates("Abies alba") {
		wantKeys[cand.Key] = true
	}
	for got := range repo.matchCalls {
		if !wantKeys[got] {
			t.Errorf("MatchExact(%q) was called, want no query beyond the two taxa's ladders", got)
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

func (r *failingMatchRepo) BeginTraitIngest(ctx context.Context) (output.IngestTx, error) {
	r.beganIngest = true
	return r.Repository.BeginTraitIngest(ctx)
}

// --- Hardening Task 5: deterministic name normalisation -------------------
//
// Every trait-side taxon name below is a REAL name the full-data crosswalk
// failed to resolve against the complete WCVP (poc/measure/out/unmatched-*.txt,
// see docs/research/reality-check.md M2'); every backbone-side canonical is
// the spelling WCVP actually holds for it.

// seedBackboneNames ingests one accepted concept per canonical name under a
// throwaway backbone, and returns canonical -> concept id.
func seedBackboneNames(t *testing.T, repo *sqlite.DB, backboneID string, canonicals ...string) map[string]string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: backboneID, Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	out := make(map[string]string, len(canonicals))
	for i, canonical := range canonicals {
		name := domain.Name{ID: fmt.Sprintf("%s:name:%d", backboneID, i), Canonical: canonical, Rank: domain.RankSpecies}
		concept := domain.Concept{ID: fmt.Sprintf("%s:concept:%d", backboneID, i), BackboneID: backboneID, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(%q): unexpected error: %v", canonical, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept(%q): unexpected error: %v", canonical, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(%q): unexpected error: %v", canonical, err)
		}
		out[canonical] = concept.ID
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return out
}

// normalisationCase is one (trait name -> backbone name, expected rule) pair.
type normalisationCase struct {
	name        string
	traitTaxon  string
	backbone    string
	wantRule    domain.NormalizationRule
	wantFlagged bool
}

func TestIngestTraits_NormalisationRulesResolveRealUnmatchedNames(t *testing.T) {
	cases := []normalisationCase{
		{
			name:       "hybrid spacing (EIVE miss Acer ×coriaceum)",
			traitTaxon: "Acer ×coriaceum", backbone: "Acer × coriaceum",
			wantRule: domain.RuleHybridSpacing,
		},
		{
			name:       "hybrid spacing, ASCII x (Midolo miss Crocosmia x crocosmiiflora)",
			traitTaxon: "Crocosmia x crocosmiiflora", backbone: "Crocosmia × crocosmiiflora",
			wantRule: domain.RuleHybridSpacing,
		},
		{
			name:       "hybrid marker dropped (EIVE miss Anacamptis ×albertii)",
			traitTaxon: "Anacamptis ×albertii", backbone: "Anacamptis albertii",
			wantRule: domain.RuleHybridMarkerDropped,
		},
		{
			name:       "hybrid marker added (Tichy/Midolo miss Abies borisii-regis)",
			traitTaxon: "Abies borisii-regis", backbone: "Abies × borisii-regis",
			wantRule: domain.RuleHybridMarkerAdded,
		},
		{
			name:       "aggregate to nominate species (EIVE miss Acer opalus aggr.)",
			traitTaxon: "Acer opalus aggr.", backbone: "Acer opalus",
			wantRule: domain.RuleAggregateToNominate, wantFlagged: true,
		},
		{
			name:       "stacked aggregate markers (EIVE miss Agrostis capillaris aggr. s. l.)",
			traitTaxon: "Agrostis capillaris aggr. s. l.", backbone: "Agrostis capillaris",
			wantRule: domain.RuleAggregateToNominate, wantFlagged: true,
		},
		{
			// "Festuca ovina s. l." is the other real Tichý miss of this
			// shape; "pallens" is used here because the shared WCVP test
			// fixture already carries a "Festuca ovina" concept, which would
			// make the fallback key ambiguous for reasons unrelated to the
			// rule under test.
			name:       "sensu lato (Tichy miss Festuca pallens s. l.)",
			traitTaxon: "Festuca pallens s. l.", backbone: "Festuca pallens",
			wantRule: domain.RuleAggregateToNominate, wantFlagged: true,
		},
		{
			name:       "autonym (EIVE miss Acer obtusatum subsp. obtusatum)",
			traitTaxon: "Acer obtusatum subsp. obtusatum", backbone: "Acer obtusatum",
			wantRule: domain.RuleAutonym, wantFlagged: true,
		},
		{
			name:       "autonym (EIVE miss Aconitum lycoctonum subsp. lycoctonum)",
			traitTaxon: "Aconitum lycoctonum subsp. lycoctonum", backbone: "Aconitum lycoctonum",
			wantRule: domain.RuleAutonym, wantFlagged: true,
		},
		{
			name:       "genitive orthography (all three vocabularies miss Cardamine plumierii)",
			traitTaxon: "Cardamine plumierii", backbone: "Cardamine plumieri",
			wantRule: domain.RuleOrthographyGenitive,
		},
		{
			name:       "genitive orthography, other direction (EIVE/Tichy miss Polygala edmundi)",
			traitTaxon: "Polygala edmundi", backbone: "Polygala edmundii",
			wantRule: domain.RuleOrthographyGenitive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertNormalisationCase(t, tc) })
	}
}

// assertNormalisationCase drives one trait name through a repository seeded
// with exactly the backbone spelling WCVP holds for it, and checks that it
// resolves, resolves via the expected rule, is flagged iff the rule is a
// judgement call, and that the value lands on the backbone concept.
func assertNormalisationCase(t *testing.T, tc normalisationCase) {
	t.Helper()
	repo := seededMatchRepo(t)
	ids := seedBackboneNames(t, repo, "test-norm", tc.backbone)
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: tc.traitTaxon, Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Matched != 1 || report.Unmatched != 0 || report.Ambiguous != 0 {
		t.Fatalf("Matched/Unmatched/Ambiguous = %d/%d/%d, want 1/0/0 — %q must resolve to %q",
			report.Matched, report.Unmatched, report.Ambiguous, tc.traitTaxon, tc.backbone)
	}
	if len(report.Normalized) != 1 {
		t.Fatalf("report.Normalized = %+v, want exactly one rule (%q)", report.Normalized, tc.wantRule)
	}
	want := application.RuleCount{Rule: tc.wantRule, Rows: 1, Taxa: 1, Flagged: tc.wantFlagged}
	if report.Normalized[0] != want {
		t.Errorf("report.Normalized[0] = %+v, want %+v", report.Normalized[0], want)
	}
	wantSample := []string(nil)
	if tc.wantFlagged {
		wantSample = []string{tc.traitTaxon}
	}
	if !equalStrings(report.FlaggedSample, wantSample) {
		t.Errorf("report.FlaggedSample = %v, want %v", report.FlaggedSample, wantSample)
	}

	// The value must land on the backbone concept, not nowhere.
	sets, err := repo.Traits(ctx, ids[tc.backbone], nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 1 || sets[0].Values[0].Value != 4.0 {
		t.Fatalf("Traits(%q) = %+v, want the single M=4.0 value", tc.backbone, sets)
	}
	// The rule must survive into the DATA, not just the ingest report: a
	// consumer reading this value back has to be able to tell that it
	// reached this concept through a rewrite.
	if got := sets[0].Values[0].Resolution; got != string(tc.wantRule) {
		t.Errorf("persisted TraitValue.Resolution = %q, want %q", got, tc.wantRule)
	}
}

func TestIngestTraits_AggregateConceptWinsOverNominateSpeciesFallback(t *testing.T) {
	// The fallback is a last resort: where the backbone DOES carry an
	// aggregate concept, the aggregate must win, and the result must not be
	// flagged — no circumscription was equated.
	repo := seededMatchRepo(t)
	ids := seedBackboneNames(t, repo, "test-agg", "Festuca ovina aggr.", "Festuca ovina")
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Festuca ovina aggr. s. l.", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("report.Matched = %d, want 1", report.Matched)
	}
	if len(report.Normalized) != 1 || report.Normalized[0].Rule != domain.RuleAggregate {
		t.Fatalf("report.Normalized = %+v, want the unflagged %q rule", report.Normalized, domain.RuleAggregate)
	}
	if report.Normalized[0].Flagged {
		t.Error("aggregate-to-aggregate match reported as flagged; nothing was equated")
	}
	if len(report.FlaggedSample) != 0 {
		t.Errorf("report.FlaggedSample = %v, want empty", report.FlaggedSample)
	}
	sets, err := repo.Traits(ctx, ids["Festuca ovina"], nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("Traits(Festuca ovina) = %+v, want none — the aggregate's value must not land on the nominate species", sets)
	}
}

func TestIngestTraits_NonNominateSubspeciesIsNeverCollapsedOntoItsSpecies(t *testing.T) {
	// EIVE miss "Allium circinatum subsp. peloponnesiacum": the
	// infraspecific epithet differs from the species epithet, so this is a
	// different taxon. It must stay unmatched rather than inherit the
	// species' concept.
	repo := seededMatchRepo(t)
	ids := seedBackboneNames(t, repo, "test-nonauto", "Allium circinatum")
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Allium circinatum subsp. peloponnesiacum", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Unmatched != 1 || report.Matched != 0 {
		t.Errorf("Matched/Unmatched = %d/%d, want 0/1", report.Matched, report.Unmatched)
	}
	sets, err := repo.Traits(ctx, ids["Allium circinatum"], nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("Traits(Allium circinatum) = %+v, want none", sets)
	}
}

func TestIngestTraits_ExactMatchIsNeverRerouted(t *testing.T) {
	// Regression guard: a name that already resolved must keep resolving to
	// the SAME concept and must be reported as exact (absent from
	// Normalized), even when a normalisation rule could produce another key
	// that also exists in the index.
	repo := seededMatchRepo(t)
	ids := seedBackboneNames(t, repo, "test-exact", "Acer opalus", "Acer opalus aggr.")
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Acer opalus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("report.Matched = %d, want 1", report.Matched)
	}
	if len(report.Normalized) != 0 {
		t.Errorf("report.Normalized = %+v, want empty — an exact hit must not be attributed to a normalisation rule", report.Normalized)
	}
	sets, err := repo.Traits(ctx, ids["Acer opalus"], nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 1 {
		t.Fatalf("Traits(Acer opalus) = %+v, want the exact hit's value", sets)
	}
	// An exact match asserts nothing about normalisation, so it stores
	// nothing: Resolution stays empty (SQL NULL) rather than "exact".
	if got := sets[0].Values[0].Resolution; got != "" {
		t.Errorf("persisted TraitValue.Resolution = %q, want empty for an exact match", got)
	}
}

func TestIngestTraits_AmbiguousNormalisedKeyIsNotRescuedByALaterRule(t *testing.T) {
	// "Acer opalus aggr." strips to "Acer opalus", which here resolves to
	// TWO distinct concepts. That is a genuine ambiguity about which taxon
	// the source meant; the walk must stop and report ambiguous rather than
	// continue until some rule happens to yield a single-concept key.
	repo := seededMatchRepo(t)
	ctx := context.Background()
	seedBackboneNames(t, repo, "test-amb-a", "Acer opalus")
	seedBackboneNames(t, repo, "test-amb-b", "Acer opalus")

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Acer opalus aggr.", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Ambiguous != 1 || report.Matched != 0 || report.Unmatched != 0 {
		t.Errorf("Matched/Unmatched/Ambiguous = %d/%d/%d, want 0/0/1", report.Matched, report.Unmatched, report.Ambiguous)
	}
	if len(report.Normalized) != 0 {
		t.Errorf("report.Normalized = %+v, want empty — an ambiguous outcome wrote nothing and must not be counted as a rule's gain", report.Normalized)
	}
}

func TestIngestTraits_NormalizedCountsAreSortedAndAggregateAcrossRows(t *testing.T) {
	repo := seededMatchRepo(t)
	seedBackboneNames(t, repo, "test-multi",
		"Acer obtusatum",      // autonym target
		"Acer × coriaceum",    // hybrid spacing target
		"Alchemilla vulgaris", // aggregate fallback target
	)
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Acer obtusatum subsp. obtusatum", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0},
		{Taxon: "Acer obtusatum subsp. obtusatum", Vocab: "eive", VocabVersion: "1.0", Dim: "N", Value: 2.0},
		{Taxon: "Acer ×coriaceum", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 3.0},
		{Taxon: "Alchemilla vulgaris aggr.", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	want := []application.RuleCount{
		{Rule: domain.RuleAggregateToNominate, Rows: 1, Taxa: 1, Flagged: true},
		{Rule: domain.RuleAutonym, Rows: 2, Taxa: 1, Flagged: true},
		{Rule: domain.RuleHybridSpacing, Rows: 1, Taxa: 1},
	}
	if len(report.Normalized) != len(want) {
		t.Fatalf("report.Normalized = %+v, want %+v", report.Normalized, want)
	}
	for i := range want {
		if report.Normalized[i] != want[i] {
			t.Fatalf("report.Normalized = %+v, want %+v (sorted by rule name)", report.Normalized, want)
		}
	}
	wantFlagged := []string{"Acer obtusatum subsp. obtusatum", "Alchemilla vulgaris aggr."}
	if !equalStrings(report.FlaggedSample, wantFlagged) {
		t.Errorf("report.FlaggedSample = %v, want %v", report.FlaggedSample, wantFlagged)
	}
}

// TestIngestTraits_ExactMatchWinsTheSlotRegardlessOfRowOrder pins
// selectTraitWinners: "Acer opalus" (exact) and "Acer opalus aggr." (only
// resolvable through the flagged aggregate fallback) both land on the SAME
// concept and dim, so trait_value's primary key can hold only one of them.
// The exact value must win in BOTH row orders — otherwise an aggregate's
// collective mean could silently overwrite a directly-matched value, and the
// stored resolution flag would describe the slot by CSV row order rather than
// by what is true of it.
func TestIngestTraits_ExactMatchWinsTheSlotRegardlessOfRowOrder(t *testing.T) {
	exactRow := application.TraitRow{Taxon: "Acer opalus", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 1.0}
	aggRow := application.TraitRow{Taxon: "Acer opalus aggr.", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 9.0}

	for _, tc := range []struct {
		name string
		rows []application.TraitRow
	}{
		{"exact first", []application.TraitRow{exactRow, aggRow}},
		{"aggregate first", []application.TraitRow{aggRow, exactRow}},
	} {
		t.Run(tc.name, func(t *testing.T) { assertExactOwnsTheSlot(t, tc.rows) })
	}
}

// assertExactOwnsTheSlot runs one row order of
// TestIngestTraits_ExactMatchWinsTheSlotRegardlessOfRowOrder and checks that
// the exactly-matched value is the one stored, uncredited to any rule.
func assertExactOwnsTheSlot(t *testing.T, rows []application.TraitRow) {
	t.Helper()
	repo := seededMatchRepo(t)
	ids := seedBackboneNames(t, repo, "test-slot", "Acer opalus")
	ctx := context.Background()

	report, err := application.IngestTraits(ctx, repo, fakeTraitRowSource{rows: rows}, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	// Both rows resolved, so both count as matched...
	if report.Matched != 2 {
		t.Errorf("report.Matched = %d, want 2 (both rows resolve)", report.Matched)
	}
	// ...but only the exact one was stored, so no rule may claim a gain.
	if len(report.Normalized) != 0 {
		t.Errorf("report.Normalized = %+v, want empty — the aggregate row stored nothing", report.Normalized)
	}
	if len(report.FlaggedSample) != 0 {
		t.Errorf("report.FlaggedSample = %v, want empty", report.FlaggedSample)
	}

	sets, err := repo.Traits(ctx, ids["Acer opalus"], nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 1 {
		t.Fatalf("Traits = %+v, want exactly one stored value", sets)
	}
	got := sets[0].Values[0]
	if got.Value != 1.0 {
		t.Errorf("stored value = %v, want 1.0 (the exact match, never the aggregate's 9.0)", got.Value)
	}
	if got.Resolution != "" {
		t.Errorf("stored Resolution = %q, want empty (the exact match owns the slot)", got.Resolution)
	}
}

// TestIngestTraits_TwoNormalisedRowsOnOneSlotCreditExactlyOne guards the
// other half of selectTraitWinners: when NO exact row competes, the first
// row in order keeps the slot, and the loser is credited to no rule — so the
// Normalized counts still agree with what the database actually holds.
func TestIngestTraits_TwoNormalisedRowsOnOneSlotCreditExactlyOne(t *testing.T) {
	repo := seededMatchRepo(t)
	ids := seedBackboneNames(t, repo, "test-slot2", "Acer opalus")
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Acer opalus aggr.", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 7.0},
		{Taxon: "Acer opalus s. l.", Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 8.0},
	}}
	report, err := application.IngestTraits(ctx, repo, src, eiveMeta)
	if err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	if report.Matched != 2 {
		t.Errorf("report.Matched = %d, want 2", report.Matched)
	}
	want := []application.RuleCount{{Rule: domain.RuleAggregateToNominate, Rows: 1, Taxa: 1, Flagged: true}}
	if len(report.Normalized) != 1 || report.Normalized[0] != want[0] {
		t.Errorf("report.Normalized = %+v, want %+v (only the stored row is credited)", report.Normalized, want)
	}

	sets, err := repo.Traits(ctx, ids["Acer opalus"], nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 1 || sets[0].Values[0].Value != 7.0 {
		t.Errorf("Traits = %+v, want the FIRST row's value (7.0) to own the slot", sets)
	}
}
