package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// seedBackboneVersion is the exact domain.BackboneVersion testdata/seed.sql
// writes for "wcvp" via raw SQL. Tests that need to go through BeginIngest
// (e.g. to reach the ingestTx trait-value writers) pass this so the
// INSERT OR REPLACE it issues for backbone_version is a byte-for-byte no-op
// against the seeded row, rather than silently mutating it.
var seedBackboneVersion = domain.BackboneVersion{
	ID:          "wcvp",
	Version:     "2026-06-15",
	License:     "CC-BY-4.0",
	SourceURL:   "https://example.org/wcvp.zip",
	IngestedAt:  "2026-07-31T00:00:00Z",
	ManifestSHA: "deadbeef",
}

func eiveM(niche float64, n int) domain.TraitValue {
	return domain.TraitValue{
		Vocab:        domain.VocabEIVE,
		VocabVersion: "1.0",
		Dim:          domain.DimM,
		Value:        5.5,
		NicheWidth:   &niche,
		NSystems:     &n,
	}
}

func tichyM() domain.TraitValue {
	return domain.TraitValue{
		Vocab:        domain.VocabTichy,
		VocabVersion: "2023",
		Dim:          domain.DimM,
		Value:        6.2,
		// NicheWidth/NSystems intentionally left nil: Tichý does not
		// provide either datum (domain.TraitValue's doc comment).
	}
}

// seedTraits writes one EIVE and one Tichý trait_value row for conceptID,
// plus both vocabularies' trait_vocabulary metadata, via a real IngestTx —
// exercising AddTraitValue/UpsertTraitVocabulary end-to-end rather than
// poking rows in directly.
func seedTraits(t *testing.T, db *DB, conceptID string) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	if err := tx.AddTraitValue(conceptID, eiveM(2.5, 12)); err != nil {
		t.Fatalf("AddTraitValue(eive): unexpected error: %v", err)
	}
	if err := tx.AddTraitValue(conceptID, tichyM()); err != nil {
		t.Fatalf("AddTraitValue(tichy): unexpected error: %v", err)
	}
	if err := tx.UpsertTraitVocabulary(domain.TraitVocabMeta{
		Vocab: domain.VocabEIVE, Version: "1.0", Taxonomy: "euromed-aligned",
		License: "CC-BY-4.0", SourceURL: "https://example.org/eive",
	}); err != nil {
		t.Fatalf("UpsertTraitVocabulary(eive): unexpected error: %v", err)
	}
	if err := tx.UpsertTraitVocabulary(domain.TraitVocabMeta{
		Vocab: domain.VocabTichy, Version: "2023", Taxonomy: "floraveg-eunis-aligned",
		License: "CC-BY-NC-4.0", SourceURL: "https://example.org/tichy",
	}); err != nil {
		t.Fatalf("UpsertTraitVocabulary(tichy): unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

func setByVocab(t *testing.T, sets []domain.TraitSet, vocab domain.TraitVocab) domain.TraitSet {
	t.Helper()
	for _, s := range sets {
		if s.Vocab == vocab {
			return s
		}
	}
	t.Fatalf("TraitSet for vocab %q not found in %+v", vocab, sets)
	return domain.TraitSet{}
}

func TestTraits_GroupsTwoVocabulariesSeparatelyWithPointerRoundTrip(t *testing.T) {
	db := openSeededDB(t)
	seedTraits(t, db, corynephorusID)

	got, err := db.Traits(context.Background(), corynephorusID, nil)
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Traits() = %d sets, want exactly 2 (never merged): %+v", len(got), got)
	}

	assertEiveSet(t, setByVocab(t, got, domain.VocabEIVE))
	assertTichySet(t, setByVocab(t, got, domain.VocabTichy))
}

func assertEiveSet(t *testing.T, eive domain.TraitSet) {
	t.Helper()
	if eive.VocabVersion != "1.0" {
		t.Errorf("eive.VocabVersion = %q, want %q", eive.VocabVersion, "1.0")
	}
	if eive.Taxonomy != "euromed-aligned" {
		t.Errorf("eive.Taxonomy = %q, want %q", eive.Taxonomy, "euromed-aligned")
	}
	if len(eive.Values) != 1 {
		t.Fatalf("eive.Values = %+v, want exactly 1", eive.Values)
	}
	ev := eive.Values[0]
	if ev.Dim != domain.DimM || ev.Value != 5.5 {
		t.Errorf("eive value = %+v, want Dim=M Value=5.5", ev)
	}
	if ev.NicheWidth == nil || *ev.NicheWidth != 2.5 {
		t.Errorf("eive.NicheWidth = %v, want non-nil 2.5", ev.NicheWidth)
	}
	if ev.NSystems == nil || *ev.NSystems != 12 {
		t.Errorf("eive.NSystems = %v, want non-nil 12", ev.NSystems)
	}
}

// assertTichySet's core assertion is the pointer round-trip: Tichý provides
// neither NicheWidth nor NSystems, so both must come back nil — NOT a
// stand-in 0/0.0 (domain.TraitValue's doc comment).
func assertTichySet(t *testing.T, tichy domain.TraitSet) {
	t.Helper()
	if tichy.VocabVersion != "2023" {
		t.Errorf("tichy.VocabVersion = %q, want %q", tichy.VocabVersion, "2023")
	}
	if tichy.Taxonomy != "floraveg-eunis-aligned" {
		t.Errorf("tichy.Taxonomy = %q, want %q", tichy.Taxonomy, "floraveg-eunis-aligned")
	}
	if len(tichy.Values) != 1 {
		t.Fatalf("tichy.Values = %+v, want exactly 1", tichy.Values)
	}
	tv := tichy.Values[0]
	if tv.Dim != domain.DimM || tv.Value != 6.2 {
		t.Errorf("tichy value = %+v, want Dim=M Value=6.2", tv)
	}
	if tv.NicheWidth != nil {
		t.Errorf("tichy.NicheWidth = %v, want nil (Tichý does not provide niche width)", *tv.NicheWidth)
	}
	if tv.NSystems != nil {
		t.Errorf("tichy.NSystems = %v, want nil (Tichý does not provide n_systems)", *tv.NSystems)
	}
}

func TestTraits_FiltersByRequestedVocabs(t *testing.T) {
	db := openSeededDB(t)
	seedTraits(t, db, corynephorusID)

	got, err := db.Traits(context.Background(), corynephorusID, []domain.TraitVocab{domain.VocabEIVE})
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Traits(vocabs=[eive]) = %d sets, want exactly 1: %+v", len(got), got)
	}
	if got[0].Vocab != domain.VocabEIVE {
		t.Errorf("Traits(vocabs=[eive])[0].Vocab = %q, want %q", got[0].Vocab, domain.VocabEIVE)
	}
}

func TestTraits_UnknownConceptReturnsErrNotFound(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.Traits(context.Background(), "nope", nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Traits(%q) error = %v, want errors.Is(err, domain.ErrNotFound)", "nope", err)
	}
	if got != nil {
		t.Errorf("Traits(%q) = %v, want nil", "nope", got)
	}
}

func TestTraits_ConceptWithoutTraitsReturnsEmptySliceNoError(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.Traits(context.Background(), jacobaeaID, nil)
	if err != nil {
		t.Fatalf("Traits(%q): unexpected error: %v", jacobaeaID, err)
	}
	if len(got) != 0 {
		t.Fatalf("Traits(%q) = %+v, want empty slice", jacobaeaID, got)
	}
}

func TestTraitVocabularies_ListsIngestedMetadata(t *testing.T) {
	db := openSeededDB(t)
	seedTraits(t, db, corynephorusID)

	got, err := db.TraitVocabularies(context.Background())
	if err != nil {
		t.Fatalf("TraitVocabularies: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("TraitVocabularies() = %+v, want exactly 2 entries", got)
	}

	byVocab := map[domain.TraitVocab]domain.TraitVocabMeta{}
	for _, m := range got {
		byVocab[m.Vocab] = m
	}
	eive, ok := byVocab[domain.VocabEIVE]
	if !ok {
		t.Fatalf("TraitVocabularies() missing eive entry: %+v", got)
	}
	if eive.Version != "1.0" || eive.Taxonomy != "euromed-aligned" || eive.License != "CC-BY-4.0" || eive.SourceURL != "https://example.org/eive" {
		t.Errorf("eive meta = %+v, want Version=1.0 Taxonomy=euromed-aligned License=CC-BY-4.0 SourceURL=https://example.org/eive", eive)
	}
	tichy, ok := byVocab[domain.VocabTichy]
	if !ok {
		t.Fatalf("TraitVocabularies() missing tichy entry: %+v", got)
	}
	if tichy.Version != "2023" || tichy.Taxonomy != "floraveg-eunis-aligned" {
		t.Errorf("tichy meta = %+v, want Version=2023 Taxonomy=floraveg-eunis-aligned", tichy)
	}
}

// TestTraits_ResolutionRoundTripsAndEmptyStaysNull pins Hardening Task 5's
// persistence contract at the adapter boundary: the normalisation rule that
// crosswalked a vocabulary's taxon name onto a concept must survive
// AddTraitValue -> trait_value -> Traits, and an EMPTY Resolution (the
// ordinary exact match) must be stored as SQL NULL rather than as ” — the
// same "absence is information" rule niche_width/n_systems follow, so a
// consumer can tell "matched exactly" from "reached through a rewrite".
func TestTraits_ResolutionRoundTripsAndEmptyStaysNull(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	exact := tichyM()
	normalised := tichyM()
	normalised.Dim = domain.DimN
	normalised.Resolution = string(domain.RuleAggregateToNominate)
	if err := tx.AddTraitValue(corynephorusID, exact); err != nil {
		t.Fatalf("AddTraitValue(exact): unexpected error: %v", err)
	}
	if err := tx.AddTraitValue(corynephorusID, normalised); err != nil {
		t.Fatalf("AddTraitValue(normalised): unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	sets, err := db.Traits(ctx, corynephorusID, []domain.TraitVocab{domain.VocabTichy})
	if err != nil {
		t.Fatalf("Traits: unexpected error: %v", err)
	}
	set := setByVocab(t, sets, domain.VocabTichy)
	for _, v := range set.Values {
		want := ""
		if v.Dim == domain.DimN {
			want = string(domain.RuleAggregateToNominate)
		}
		if v.Resolution != want {
			t.Errorf("dim %s: Resolution = %q, want %q", v.Dim, v.Resolution, want)
		}
	}

	// The empty case must be a real SQL NULL, not the empty string: '' would
	// be an assertion ("resolved by a rule named nothing"), NULL is the
	// absence the domain type means.
	var nulls int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM trait_value WHERE concept_id = ? AND dim = 'M' AND resolution IS NULL`,
		corynephorusID).Scan(&nulls); err != nil {
		t.Fatalf("counting NULL resolutions: unexpected error: %v", err)
	}
	if nulls != 1 {
		t.Errorf("rows with resolution IS NULL = %d, want 1 (an exact match stores NULL, never '')", nulls)
	}
}
