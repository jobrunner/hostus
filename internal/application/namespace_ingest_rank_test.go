package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestIngestNameSpace_CarriesTheSourceRank pins the wiring whose absence let a
// species resolve to one of its own subspecies. The source list states each
// spelling's rank — Euro+Med's own column carries Species for 88.083 rows and
// Subspecies for 29.330 — and that statement has to survive the
// reader -> DTO -> entry path, exactly as Status had to before it.
//
// Without it, a concept holding both the species and a subspecies of a space
// sees two equally ACCEPTED entries and falls back to ext_id order, which
// answered a plain Bromus erectus with "Bromopsis erecta subsp. permixta".
func TestIngestNameSpace_CarriesTheSourceRank(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{{
		Taxon: "Festuca ovina", SourceID: "5647", Status: "accepted", Rank: "Species",
	}}

	if _, err := application.IngestNameSpace(ctx, repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no floraveg entries stored for the Festuca ovina concept")
	}
	for _, e := range entries {
		if e.Rank != domain.RankSpecies {
			t.Errorf("entry %s carries rank %q, want %q from the source list", e.ExtID, e.Rank, domain.RankSpecies)
		}
	}
}

// TestIngestNameSpace_NormalisesTheSourceRankVocabulary pins that the rank is
// stored in hostus' own vocabulary rather than verbatim. The spaces disagree —
// Euro+Med writes "Subspecies", GermanSL "SSP", WCVP "subsp." — and
// domain.rankCongruent compares Rank values, so an unnormalised string would
// simply never match and the rule would silently never bite.
func TestIngestNameSpace_NormalisesTheSourceRankVocabulary(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{{
		Taxon: "Festuca ovina", SourceID: "5647", Status: "accepted", Rank: "Subspecies",
	}}

	if _, err := application.IngestNameSpace(ctx, repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no floraveg entries stored for the Festuca ovina concept")
	}
	if got := entries[0].Rank; got != domain.RankSubspecies {
		t.Errorf("rank = %q, want the normalised %q rather than the source's own spelling", got, domain.RankSubspecies)
	}
}

// TestIngestNameSpace_RanklessSourceStaysUnknown pins that an absent rank stays
// absent rather than becoming RankOther. The two are different statements: the
// zero value means "the source said nothing", which stands the congruence rule
// down, while RankOther means "the source named a rank hostus has no constant
// for" — and several spaces (euromed's flat listing among them) carry no rank
// column at all.
func TestIngestNameSpace_RanklessSourceStaysUnknown(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{{
		Taxon: "Festuca ovina", SourceID: "5647", Status: "accepted",
	}}

	if _, err := application.IngestNameSpace(ctx, repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no floraveg entries stored for the Festuca ovina concept")
	}
	if got := entries[0].Rank; got != "" {
		t.Errorf("rank = %q, want the zero value for a source row that names no rank", got)
	}
}

// TestIngestNameSpace_CarriesTheSourceAcceptedName pins the last field the
// anchor needs. NameRow has carried AcceptedTaxon since the synonymy-closure
// pass, but only as an in-run grouping key — it was never written onto the
// entry, so nothing downstream could follow the space's own synonymy.
func TestIngestNameSpace_CarriesTheSourceAcceptedName(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{{
		Taxon: "Festuca ovina", SourceID: "5647", Status: "synonymobjective",
		Rank: "Species", AcceptedTaxon: "Festuca guestfalica",
	}}

	if _, err := application.IngestNameSpace(ctx, repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no floraveg entries stored for the Festuca ovina concept")
	}
	if got := entries[0].AcceptedName; got != "Festuca guestfalica" {
		t.Errorf("accepted_name = %q, want the source's own accepted_taxon", got)
	}
}
