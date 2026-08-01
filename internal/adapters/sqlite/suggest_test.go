package sqlite_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// wcvpRowSource adapts a *wcvp.Dataset into application.RowSource, mirroring
// internal/application's own test harness (ingest_test.go). It is
// duplicated here (rather than imported) because this file lives in the
// sqlite_test package specifically so it can exercise the real end-to-end
// path — application.Ingest, driven by the real WCVP fixture, writing
// through the real sqlite.DB — which proves Suggest sees data that
// Finalize (wired into application.Ingest, see internal/application/ingest.go)
// actually populated, not data a test seeded by hand.
type wcvpRowSource struct{ ds *wcvp.Dataset }

func (s wcvpRowSource) Taxa() []application.TaxonRow {
	out := make([]application.TaxonRow, 0, len(s.ds.Taxa))
	for _, t := range s.ds.Taxa {
		out = append(out, application.TaxonRow{
			TaxonID:         t.TaxonID,
			AcceptedTaxonID: t.AcceptedNameUsageID,
			Accepted:        t.IsAccepted(),
			Canonical:       t.Canonical,
			Authorship:      t.Authorship,
			Rank:            t.Rank,
			Status:          t.Status,
			POWOID:          t.POWOID(),
			ParentTaxonID:   t.ParentNameUsageID,
			BasionymTaxonID: t.OriginalNameUsageID,
		})
	}
	return out
}

func (s wcvpRowSource) Distributions() []application.DistributionRow {
	out := make([]application.DistributionRow, 0, len(s.ds.Distributions))
	for _, d := range s.ds.Distributions {
		out = append(out, application.DistributionRow{TaxonID: d.CoreID, AreaCode: d.AreaCode()})
	}
	return out
}

func wcvpReaderFor(b application.Backbone) (application.RowSource, error) {
	ds, err := wcvp.Read(b.Path)
	if err != nil {
		return nil, err
	}
	return wcvpRowSource{ds: ds}, nil
}

// ingestWCVPFixture opens a fresh in-memory sqlite.DB and ingests the real
// WCVP test fixture (internal/adapters/wcvp/testdata/wcvp-sample) through
// the real application.Ingest, so Suggest is exercised against data whose
// FTS index was populated by the production ingest path (including
// Finalize), not seeded by a test shortcut.
func ingestWCVPFixture(t *testing.T) *sqlite.DB {
	t.Helper()
	ctx := context.Background()

	ds, err := manifest.Parse("../../application/testdata/dataset.yaml")
	if err != nil {
		t.Fatalf("manifest.Parse: unexpected error: %v", err)
	}
	backbones := make([]application.Backbone, 0, len(ds.Backbones))
	for _, b := range ds.Backbones {
		backbones = append(backbones, application.Backbone{
			ID:        b.ID,
			Version:   b.Version,
			License:   b.License,
			SourceURL: b.SourceURL,
			Path:      b.Path,
		})
	}
	appDS := &application.Dataset{Backbones: backbones, ManifestSHA: ds.ManifestSHA}

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := application.Ingest(ctx, appDS, wcvpReaderFor, db); err != nil {
		t.Fatalf("application.Ingest: unexpected error: %v", err)
	}
	return db
}

func conceptIDs(items []domain.SuggestItem) map[string]domain.SuggestItem {
	out := make(map[string]domain.SuggestItem, len(items))
	for _, it := range items {
		out[it.ConceptID] = it
	}
	return out
}

// TestSuggest_PrefixOnAcceptedNameFindsCorynephorus proves the whole wire:
// a real ingest populates fts_name (via the new Finalize wiring), and a
// short genus-level prefix matches the accepted Corynephorus canescens
// concept with PrefixHit set.
func TestSuggest_PrefixOnAcceptedNameFindsCorynephorus(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	got, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Suggest(\"coryn\") = empty, want at least the Corynephorus canescens concept")
	}

	byID := conceptIDs(got)
	species, ok := byID["wcvp:concept:405825"]
	if !ok {
		t.Fatalf("Suggest(\"coryn\") = %+v, want an item for concept wcvp:concept:405825 (Corynephorus canescens)", got)
	}
	if species.Canonical != "Corynephorus canescens" {
		t.Errorf("species.Canonical = %q, want %q", species.Canonical, "Corynephorus canescens")
	}
	if !species.PrefixHit {
		t.Error("species.PrefixHit = false, want true (result came from an FTS5 MATCH)")
	}
	if species.Rank != domain.RankSpecies {
		t.Errorf("species.Rank = %q, want %q", species.Rank, domain.RankSpecies)
	}
	if species.Status != domain.StatusAccepted {
		t.Errorf("species.Status = %q, want %q", species.Status, domain.StatusAccepted)
	}
}

// TestSuggest_PrefixOnSynonymResolvesToAcceptedConcept proves that a
// prefix matching only a SYNONYM's canonical (never the accepted name's)
// still resolves back to the accepted concept — because Finalize indexes
// every concept_name row (accepted AND synonym), all mapped to the same
// accepted concept_id.
func TestSuggest_PrefixOnSynonymResolvesToAcceptedConcept(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	// "weingaertneria" only ever appears as a synonym canonical in the
	// fixture (Weingaertneria canescens var. pallida, taxonid 450134) —
	// never as any accepted name.
	got, err := db.Suggest(ctx, "weingaertneria", output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Suggest(\"weingaertneria\") = %+v, want exactly 1 item", got)
	}
	if got[0].ConceptID != "wcvp:concept:405825" {
		t.Errorf("ConceptID = %q, want %q (the accepted Corynephorus canescens concept)", got[0].ConceptID, "wcvp:concept:405825")
	}
	if got[0].Canonical != "Corynephorus canescens" {
		t.Errorf("Canonical = %q, want %q (Suggest always reports the accepted name, not the matched synonym text — see Finalize's doc comment)", got[0].Canonical, "Corynephorus canescens")
	}
}

// TestSuggest_RanksFiltersOutNonMatchingRanks proves the Ranks option
// narrows results: "coryn" alone matches both the GENUS-rank accepted
// concept (Corynephorus, taxonid 451295) and the SPECIES-rank accepted
// concept (Corynephorus canescens, taxonid 405825); restricting to
// SPECIES must drop the genus.
func TestSuggest_RanksFiltersOutNonMatchingRanks(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	all, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest (unfiltered): unexpected error: %v", err)
	}
	if _, ok := conceptIDs(all)["wcvp:concept:451295"]; !ok {
		t.Fatalf("Suggest(\"coryn\") unfiltered = %+v, want it to include the GENUS concept wcvp:concept:451295 (precondition for this test)", all)
	}

	speciesOnly, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10, Ranks: []domain.Rank{domain.RankSpecies}})
	if err != nil {
		t.Fatalf("Suggest (Ranks=[SPECIES]): unexpected error: %v", err)
	}
	byID := conceptIDs(speciesOnly)
	if _, ok := byID["wcvp:concept:451295"]; ok {
		t.Errorf("Suggest with Ranks=[SPECIES] still returned the GENUS concept: %+v", speciesOnly)
	}
	if _, ok := byID["wcvp:concept:405825"]; !ok {
		t.Errorf("Suggest with Ranks=[SPECIES] = %+v, want it to still include the SPECIES concept wcvp:concept:405825", speciesOnly)
	}
	for _, item := range speciesOnly {
		if item.Rank != domain.RankSpecies {
			t.Errorf("item %+v has Rank %q, want only %q (Ranks filter)", item, item.Rank, domain.RankSpecies)
		}
	}
}

// TestSuggest_InArea_ResolvesRawWGSRPDCodeAndAliasAndAbsentCode exercises
// the area→L3 handling end to end: a raw WGSRPD L3 passthrough code that
// IS in the concept's distribution, the "DE" alias that resolves to a code
// that is NOT in this concept's distribution (the fixture's Corynephorus
// canescens distribution rows are AUT/BLT/BLR/BGM/BRC/RUC/CNT/CZE/DEN —
// GER is not among them), and a code that plainly is not present.
func TestSuggest_InArea_ResolvesRawWGSRPDCodeAndAliasAndAbsentCode(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	cases := []struct {
		name       string
		area       string
		wantInArea bool
	}{
		{"raw L3 code present in distribution (AUT)", "AUT", true},
		{"DE alias resolves to GER, absent from this concept's distribution", "DE", false},
		{"raw L3 code absent from distribution", "ZZZ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10, Area: tc.area})
			if err != nil {
				t.Fatalf("Suggest: unexpected error: %v", err)
			}
			item, ok := conceptIDs(got)["wcvp:concept:405825"]
			if !ok {
				t.Fatalf("Suggest(Area=%q) = %+v, want it to include wcvp:concept:405825 regardless of area (area only affects InArea, not membership)", tc.area, got)
			}
			if item.InArea != tc.wantInArea {
				t.Errorf("Suggest(Area=%q) InArea = %v, want %v", tc.area, item.InArea, tc.wantInArea)
			}
		})
	}
}

// TestSuggest_EmptyAreaMeansInAreaAlwaysFalse pins the documented
// empty-Area convention: no area filter is applied (every otherwise
// matching concept is still returned), but InArea is false on every item
// since "unknown area" cannot be considered "in area".
func TestSuggest_EmptyAreaMeansInAreaAlwaysFalse(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	got, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Suggest(\"coryn\") = empty, want at least one item")
	}
	for _, item := range got {
		if item.InArea {
			t.Errorf("item %+v: InArea = true with an empty Area, want false", item)
		}
	}
}

// TestSuggest_MatchSpecialCharactersDoNotErrorOrInject proves a q
// containing FTS5 query-syntax special characters is treated as literal
// text (via ftsPrefixToken's quoting), not as FTS5 operators/injection: it
// must not error, and it must not spuriously match everything.
func TestSuggest_MatchSpecialCharactersDoNotErrorOrInject(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	for _, q := range []string{
		`a"b`,
		`co*n`,
		`co-n OR canescens`,
		`"`,
		`)(`,
	} {
		t.Run(q, func(t *testing.T) {
			got, err := db.Suggest(ctx, q, output.SuggestOpts{Limit: 10})
			if err != nil {
				t.Fatalf("Suggest(%q): unexpected error (must not error/inject on FTS5 special characters): %v", q, err)
			}
			// None of these literal strings are a real name prefix in the
			// fixture, so none should match anything — if a special
			// character were interpreted as an FTS5 operator instead of
			// literal text, this could spuriously return results.
			if len(got) != 0 {
				t.Errorf("Suggest(%q) = %+v, want empty (input must be treated as literal text, not FTS5 syntax)", q, got)
			}
		})
	}
}

// TestSuggest_EmptyOrOneRuneQueryReturnsEmpty pins the documented
// too-short-query policy: a canonicalized q shorter than two runes
// (including "" and a single letter) returns an empty, non-error result
// without ever touching FTS5.
func TestSuggest_EmptyOrOneRuneQueryReturnsEmpty(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	for _, q := range []string{"", "c", " ", "ä"} {
		t.Run("q="+q, func(t *testing.T) {
			got, err := db.Suggest(ctx, q, output.SuggestOpts{Limit: 10})
			if err != nil {
				t.Fatalf("Suggest(%q): unexpected error: %v", q, err)
			}
			if len(got) != 0 {
				t.Errorf("Suggest(%q) = %+v, want empty", q, got)
			}
		})
	}
}
