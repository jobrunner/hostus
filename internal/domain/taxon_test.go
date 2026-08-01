package domain_test

import (
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestParseRank(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Rank
	}{
		{"Family", domain.RankFamily},
		{"FAMILY", domain.RankFamily},
		{"family", domain.RankFamily},
		{"Genus", domain.RankGenus},
		{"GENUS", domain.RankGenus},
		{"Species", domain.RankSpecies},
		{"SPECIES", domain.RankSpecies},
		{"species", domain.RankSpecies},
		{"Subspecies", domain.RankSubspecies},
		{"subspecies", domain.RankSubspecies},
		{"Variety", domain.RankVariety},
		{"variety", domain.RankVariety},
		{"Subvariety", domain.RankSubvariety},
		{"SUBVARIETY", domain.RankSubvariety},
		{"Form", domain.RankForm},
		{"form", domain.RankForm},
		{"Subform", domain.RankSubform},
		{"SUBFORM", domain.RankSubform},
		{"Nothosubspecies", domain.RankNothosubspecies},
		{"NOTHOSUBSPECIES", domain.RankNothosubspecies},
		{"Nothovariety", domain.RankNothovariety},
		{"Nothoform", domain.RankNothoform},
		{"Other", domain.RankOther},
		{"OTHER", domain.RankOther},
	}
	for _, c := range cases {
		got, err := domain.ParseRank(c.in)
		if err != nil {
			t.Fatalf("ParseRank(%q): unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseRank(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseRank_Unknown(t *testing.T) {
	_, err := domain.ParseRank("kingdom")
	if err == nil {
		t.Fatal("ParseRank(\"kingdom\"): expected error, got nil")
	}
}

func TestParseRank_Empty(t *testing.T) {
	_, err := domain.ParseRank("")
	if err == nil {
		t.Fatal("ParseRank(\"\"): expected error, got nil")
	}
}

// TestParseRank_RejectsBogus pins the strict API-facing behavior SP2's
// `GET /v1/suggest?rank=bogus` -> 400 INVALID_QUERY test depends on: the
// strict parser must keep rejecting garbage even though ParseRankLenient
// (below) now tolerates the exact same input for ingest purposes.
func TestParseRank_RejectsBogus(t *testing.T) {
	_, err := domain.ParseRank("bogus")
	if err == nil {
		t.Fatal("ParseRank(\"bogus\"): expected error, got nil")
	}
}

// wcvpRankInventory is the full measured WCVP taxonrank inventory from
// docs/research/reality-check.md (real wcvp_taxon.csv, column 8): every
// spelling that is NOT one of ParseRank's canonical constants, including
// the empty string. ParseRankLenient must map every single one of these to
// RankOther, and must never error, or a full WCVP ingest aborts again
// (this is the exact defect this task fixes).
var wcvpRankInventory = []string{
	"", "proles", "lusus", "microgène", "Convariety", "monstr.", "grex",
	"subproles", "stirps", "provar.", "psp.", "modif.", "mut.", "sublusus",
	"subap.", "subsubsp.", "subspecioid", "positio", "nid", "micromorphe",
	"microf.", "group", "ecas.", "agamosp.",
}

func TestParseRankLenient_ExoticInventory_NeverErrorsAlwaysOther(t *testing.T) {
	for _, in := range wcvpRankInventory {
		t.Run("input:"+in, func(t *testing.T) {
			rank, verbatim := domain.ParseRankLenient(in)
			if rank != domain.RankOther {
				t.Errorf("ParseRankLenient(%q) rank = %q, want %q", in, rank, domain.RankOther)
			}
			wantVerbatim := strings.TrimSpace(in)
			if verbatim != wantVerbatim {
				t.Errorf("ParseRankLenient(%q) verbatim = %q, want %q (preserved, readable)", in, verbatim, wantVerbatim)
			}
		})
	}
}

func TestParseRankLenient_CanonicalAndNothotaxonRanks(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Rank
	}{
		{"Family", domain.RankFamily},
		{"Genus", domain.RankGenus},
		{"Species", domain.RankSpecies},
		{"Subspecies", domain.RankSubspecies},
		{"Variety", domain.RankVariety},
		{"Subvariety", domain.RankSubvariety},
		{"Form", domain.RankForm},
		{"Subform", domain.RankSubform},
		{"nothosubsp.", domain.RankNothosubspecies},
		{"nothovar.", domain.RankNothovariety},
		{"nothof.", domain.RankNothoform},
	}
	for _, c := range cases {
		rank, verbatim := domain.ParseRankLenient(c.in)
		if rank != c.want {
			t.Errorf("ParseRankLenient(%q) rank = %q, want %q", c.in, rank, c.want)
		}
		if verbatim != c.in {
			t.Errorf("ParseRankLenient(%q) verbatim = %q, want %q", c.in, verbatim, c.in)
		}
	}
}

func TestParseStatus(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Status
	}{
		{"Accepted", domain.StatusAccepted},
		{"ACCEPTED", domain.StatusAccepted},
		{"accepted", domain.StatusAccepted},
		{"Synonym", domain.StatusSynonym},
		{"SYNONYM", domain.StatusSynonym},
		{"synonym", domain.StatusSynonym},
		{"Unplaced", domain.StatusUnplaced},
		{"unplaced", domain.StatusUnplaced},
		{"Unknown Value", domain.StatusUnknown},
		{"", domain.StatusUnknown},
	}
	for _, c := range cases {
		got := domain.ParseStatus(c.in)
		if got != c.want {
			t.Errorf("ParseStatus(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCanonicalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  Silene   otites  ", "silene otites"},
		{"SILENE OTITES", "silene otites"},
		{"Weingärtneria", "weingartneria"},
		{"Weingaertneria", "weingaertneria"},
		{"Café-plante", "cafe-plante"},
		{"", ""},
		{"   ", ""},
		{"Ångström", "angstrom"},
		// ß/ł/ø/đ are NOT decomposable diacritics in Unicode (no plain
		// base letter + combining mark to strip), so SQLite's FTS5
		// `unicode61 remove_diacritics 2` tokenizer does not fold them
		// either (verified in internal/adapters/sqlite/fts_parity_test.go).
		// Canonicalize must leave them as-is so its comparison keys match
		// the ones the FTS5 index computes at query time.
		{"Straße", "straße"},
		{"Włodzimierz", "włodzimierz"},
		{"Øster", "øster"},
		{"Đorđe", "đorđe"},
	}
	for _, c := range cases {
		got := domain.Canonicalize(c.in)
		if got != c.want {
			t.Errorf("Canonicalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCanonicalize_DifferentWordsStayDifferent(t *testing.T) {
	a := domain.Canonicalize("Silene otites")
	b := domain.Canonicalize("Silene otitis")
	if a == b {
		t.Fatalf("Canonicalize must not collapse distinct epithets: %q == %q", a, b)
	}
}

func TestNormalizeAuthor(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"(L.) P.Beauv.", "(L.) P.Beauv."},
		{"(L.)P.Beauv.", "(L.)P.Beauv."},
		{"(L.)  P.Beauv.", "(L.) P.Beauv."},
		{"L. & Beauv.", "L. & Beauv."},
		{"L.&Beauv.", "L. & Beauv."},
		{"L . & Beauv .", "L. & Beauv."},
		{"  L.  ", "L."},
		{"", ""},
	}
	for _, c := range cases {
		got := domain.NormalizeAuthor(c.in)
		if got != c.want {
			t.Errorf("NormalizeAuthor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
