package domain_test

import (
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
		{"Form", domain.RankForm},
		{"form", domain.RankForm},
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
