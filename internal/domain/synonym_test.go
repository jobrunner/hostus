package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func boolPtr(b bool) *bool { return &b }

// TestClassifyNomStatus_CorynephorusIncanescens is the acceptance test named
// in the task brief: the UC5 worked example carries ", nom. illeg. superfl."
// (measured on wcvp:name:405842), NOT ", nom. superfl.". An equality match
// against the three doc-named values would miss it.
func TestClassifyNomStatus_CorynephorusIncanescens(t *testing.T) {
	v := domain.ClassifyNomStatus(", nom. illeg. superfl.")

	if v.Judgement != domain.JudgementDisqualifying {
		t.Fatalf("judgement = %q, want %q", v.Judgement, domain.JudgementDisqualifying)
	}
	if v.Normalized != "nom. illeg. superfl." {
		t.Fatalf("normalized = %q, want %q", v.Normalized, "nom. illeg. superfl.")
	}
	if v.Reason() == "" {
		t.Fatal("Reason() must state why the synonym was disqualified")
	}
	tokens := make([]string, 0, len(v.Matched))
	for _, r := range v.Matched {
		tokens = append(tokens, r.Fragment)
	}
	want := []string{"illeg", "superfl"}
	if !reflect.DeepEqual(tokens, want) {
		t.Fatalf("matched tokens = %v, want %v", tokens, want)
	}
}

// TestClassifyNomStatus_EqualityWouldMiss pins containment over equality: the
// exact value ", nom. superfl." covers 1.716 names, containment on "superfl"
// covers 12.502. Both must land on the same judgement.
func TestClassifyNomStatus_EqualityWouldMiss(t *testing.T) {
	for _, raw := range []string{
		", nom. superfl.",
		", nom. illeg. superfl.",
		", nom. illeg. superfl. as it includes the type of Oncocalyx.",
		", nom. illeg. homonym. post.",
		", not validly publ.",
		", nom. nud.",
		", pro syn.",
	} {
		if got := domain.ClassifyNomStatus(raw).Judgement; got != domain.JudgementDisqualifying {
			t.Errorf("ClassifyNomStatus(%q) = %q, want %q", raw, got, domain.JudgementDisqualifying)
		}
	}
}

// TestNormalizeNomStatus covers the measured surface traps: the leading ", "
// on 99,86 % of values, mixed case (", nom. Cons.", 2 names), and the double
// space in ", contrary to  Art. 39.1. (ICN, 2012)." (16 names).
func TestNormalizeNomStatus(t *testing.T) {
	cases := map[string]string{
		", nom. nud.":                            "nom. nud.",
		"nom. nud.":                              "nom. nud.",
		", nom. Cons.":                           "nom. cons.",
		" , nom. utique rej.":                    "nom. utique rej.",
		",  nom. utique rejic.":                  "nom. utique rejic.",
		", contrary to  Art. 39.1. (ICN, 2012).": "contrary to art. 39.1. (icn, 2012).",
		"":                                       "",
		"   ":                                    "",
		",":                                      "",
	}
	for raw, want := range cases {
		if got := domain.NormalizeNomStatus(raw); got != want {
			t.Errorf("NormalizeNomStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestClassifyNomStatus_Absent: 1.349.732 of 1.448.984 names carry no
// nom_status at all. Absence is its own judgement — it must not be reported
// as "acceptable" (that would claim the name was checked and found clean),
// and it must not be reported as "unclassified" (nothing was recorded to
// classify). It is the only judgement that stays publishable by default.
func TestClassifyNomStatus_Absent(t *testing.T) {
	for _, raw := range []string{"", "   ", ", "} {
		v := domain.ClassifyNomStatus(raw)
		if v.Judgement != domain.JudgementAbsent {
			t.Errorf("ClassifyNomStatus(%q) = %q, want %q", raw, v.Judgement, domain.JudgementAbsent)
		}
		if len(v.Matched) != 0 {
			t.Errorf("ClassifyNomStatus(%q) matched %v, want none", raw, v.Matched)
		}
	}
}

// TestClassifyNomStatus_Acceptable pins the values that assert the name IS
// nomenclaturally sound: nom. cons. (1.237), orth. cons. (11),
// nom. altern. (103) / nom. alt. (36), legitimate homonym (12).
func TestClassifyNomStatus_Acceptable(t *testing.T) {
	for _, raw := range []string{
		", nom. cons.",
		", nom. & orth. cons.",
		", nom. altern.",
		", nom. alt.",
		", legitimate homonym.",
	} {
		if got := domain.ClassifyNomStatus(raw).Judgement; got != domain.JudgementAcceptable {
			t.Errorf("ClassifyNomStatus(%q) = %q, want %q", raw, got, domain.JudgementAcceptable)
		}
	}
}

// TestClassifyNomStatus_DisqualifyingBeatsAcceptable: 684 cells carry several
// statuses. A defect anywhere in the cell wins over a soundness assertion.
func TestClassifyNomStatus_DisqualifyingBeatsAcceptable(t *testing.T) {
	v := domain.ClassifyNomStatus(", nom. cons., nom. illeg.")
	if v.Judgement != domain.JudgementDisqualifying {
		t.Fatalf("judgement = %q, want %q", v.Judgement, domain.JudgementDisqualifying)
	}
	if len(v.Matched) != 2 {
		t.Fatalf("matched = %v, want both the disqualifying and the acceptable rule", v.Matched)
	}
}

// TestClassifyNomStatus_PendingProposalsAreMasked: ", nom. cons. prop." (33)
// and ", nom. rej. prop." (48) are PROPOSALS, not decisions. Naive
// containment would read them as conserved / rejected; the guard pass masks
// them out first so neither the acceptable nor the disqualifying token fires.
func TestClassifyNomStatus_PendingProposalsAreMasked(t *testing.T) {
	for _, raw := range []string{", nom. cons. prop.", ", nom. rej. prop.", ", nom. utique rej. prop."} {
		v := domain.ClassifyNomStatus(raw)
		if v.Judgement != domain.JudgementUnclassified {
			t.Errorf("ClassifyNomStatus(%q) = %q, want %q", raw, v.Judgement, domain.JudgementUnclassified)
		}
		if len(v.Matched) != 1 {
			t.Errorf("ClassifyNomStatus(%q) matched %v, want exactly the guard rule", raw, v.Matched)
		}
	}
}

// TestClassifyNomStatus_BotanicalOpenItems: the five values that need a
// botanical, not a technical, decision must be surfaced as open items and
// never classified silently. ", not validly publ.?" (8, literal question
// mark) is the sharp one — plain containment would read it as the 18.623
// "not validly publ." defect although the SOURCE itself is unsure.
func TestClassifyNomStatus_BotanicalOpenItems(t *testing.T) {
	for _, raw := range []string{
		", sensu auct.",
		", tentatively listed as a synonym.",
		", fossil name.",
		", isonym",
		", not validly publ.?",
	} {
		v := domain.ClassifyNomStatus(raw)
		if v.Judgement != domain.JudgementUnclassified {
			t.Errorf("ClassifyNomStatus(%q) = %q, want %q", raw, v.Judgement, domain.JudgementUnclassified)
		}
		if len(v.Matched) != 1 || !v.Matched[0].OpenItem {
			t.Errorf("ClassifyNomStatus(%q) matched %v, want exactly one open-item rule", raw, v.Matched)
		}
	}

	if got := len(domain.BotanicalOpenItems()); got != 5 {
		t.Fatalf("BotanicalOpenItems() has %d entries, want the 5 named in Task 1 §5.3", got)
	}
}

// TestClassifyNomStatus_OpenItemDoesNotHideADefect: an open-item token must
// not shield a real defect that sits in the same cell (", nom. illeg., later
// homonym of a fossil name." — 2 names).
func TestClassifyNomStatus_OpenItemDoesNotHideADefect(t *testing.T) {
	v := domain.ClassifyNomStatus(", nom. illeg., later homonym of a fossil name.")
	if v.Judgement != domain.JudgementDisqualifying {
		t.Fatalf("judgement = %q, want %q", v.Judgement, domain.JudgementDisqualifying)
	}
}

// TestClassifyNomStatus_LongTail: 1.225 values have fewer than 10 hits and
// 141 values are not statuses at all (citation fragments, free text). They
// take the documented unclassified path — no rule matched, nothing guessed.
func TestClassifyNomStatus_LongTail(t *testing.T) {
	for _, raw := range []string{
		"[Cusc.: 184]",
		`published as "mutatio nova"`,
		", not accepted by the author.",
		", descr. ampl.",
	} {
		v := domain.ClassifyNomStatus(raw)
		if v.Judgement != domain.JudgementUnclassified {
			t.Errorf("ClassifyNomStatus(%q) = %q, want %q", raw, v.Judgement, domain.JudgementUnclassified)
		}
		if len(v.Matched) != 0 {
			t.Errorf("ClassifyNomStatus(%q) matched %v, want no rule", raw, v.Matched)
		}
	}

	// A citation fragment that also carries a real status still resolves.
	if got := domain.ClassifyNomStatus("[Cusc.: 183], nom. illeg.").Judgement; got != domain.JudgementDisqualifying {
		t.Errorf("mixed citation/status = %q, want %q", got, domain.JudgementDisqualifying)
	}
}

// TestNomStatusRules_AllTraceable: every rule must carry the token, a
// measured name count and a note — no rule for a token nobody observed.
func TestNomStatusRules_AllTraceable(t *testing.T) {
	rules := domain.NomStatusRules()
	if len(rules) == 0 {
		t.Fatal("NomStatusRules() is empty")
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if r.Fragment == "" || r.Names <= 0 || r.Note == "" {
			t.Errorf("rule %+v is not traceable to a measured count", r)
		}
		if r.Fragment != domain.NormalizeNomStatus(r.Fragment) {
			t.Errorf("rule token %q is not in normalized form", r.Fragment)
		}
		if seen[r.Fragment] {
			t.Errorf("duplicate rule token %q", r.Fragment)
		}
		seen[r.Fragment] = true
	}
}

func TestTypificationOf(t *testing.T) {
	cases := []struct {
		in   *bool
		want domain.Typification
	}{
		{boolPtr(true), domain.TypificationHomotypic},
		{nil, domain.TypificationUnknown},
		{boolPtr(false), domain.TypificationHeterotypic},
	}
	for _, c := range cases {
		if got := domain.TypificationOf(c.in); got != c.want {
			t.Errorf("TypificationOf(%v) = %q, want %q", c.in, got, c.want)
		}
	}

	if a, b, c := domain.TypificationOrder(domain.TypificationHomotypic),
		domain.TypificationOrder(domain.TypificationUnknown),
		domain.TypificationOrder(domain.TypificationHeterotypic); a >= b || b >= c {
		t.Fatalf("typification order = %d/%d/%d, want homotypic < unknown < heterotypic", a, b, c)
	}
}

// TestRankSynonyms_NullHomotypicIsNotHeterotypic is the tri-state trap:
// concept_name.homotypic is NULL on 692.941 rows. NULL means UNKNOWN, and
// SP3 deliberately refused to guess. It must rank between the two known
// states and must be reported as "unknown", never as "heterotypic".
func TestRankSynonyms_NullHomotypicIsNotHeterotypic(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "het", Homotypic: boolPtr(false)},
		{NameID: "unk", Homotypic: nil},
		{NameID: "hom", Homotypic: boolPtr(true)},
	}
	got := domain.RankSynonyms(items, domain.SynonymOptions{})

	wantOrder := []string{"hom", "unk", "het"}
	for i, want := range wantOrder {
		if got[i].Candidate.NameID != want {
			t.Fatalf("position %d = %q, want %q (full: %v)", i, got[i].Candidate.NameID, want, ids(got))
		}
	}
	if got[1].Typification != domain.TypificationUnknown {
		t.Errorf("NULL homotypic reported as %q, want %q", got[1].Typification, domain.TypificationUnknown)
	}
	for _, r := range got {
		if !r.Publishable {
			t.Errorf("%q must stay publishable: %s", r.Candidate.NameID, r.Reason)
		}
	}
}

// TestRankSynonyms_BasionymLeadsHomotypicBlock: the source doc's worked
// example puts Aira canescens L. — the basionym — first among the homotypic
// synonyms of Corynephorus canescens.
func TestRankSynonyms_BasionymLeadsHomotypicBlock(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "b-unknown", Homotypic: nil, IsBasionym: true},
		{NameID: "a-homotypic", Homotypic: boolPtr(true)},
		{NameID: "a-basionym", Homotypic: boolPtr(true), IsBasionym: true},
	}
	got := domain.RankSynonyms(items, domain.SynonymOptions{})
	if got[0].Candidate.NameID != "a-basionym" {
		t.Fatalf("order = %v, want the homotypic basionym first", ids(got))
	}
	if got[1].Candidate.NameID != "a-homotypic" {
		t.Fatalf("order = %v, want the homotypic non-basionym second", ids(got))
	}
}

// TestRankSynonyms_StatusExclusion: a disqualified name is EXCLUDED, not
// down-ranked — a nom. nud. does not belong in a publication at any
// position — but it is still returned so Task 3 can build the exclusion
// summary, and the reason is stated.
func TestRankSynonyms_StatusExclusion(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "illeg", Canonical: "Corynephorus incanescens", NomStatus: ", nom. illeg. superfl.", Homotypic: boolPtr(true), IsBasionym: true},
		{NameID: "clean", Canonical: "Aira canescens", Homotypic: boolPtr(false)},
	}
	got := domain.RankSynonyms(items, domain.SynonymOptions{})

	if got[0].Candidate.NameID != "clean" {
		t.Fatalf("order = %v, want the publishable name first even though the excluded one is the homotypic basionym", ids(got))
	}
	ex := got[1]
	if ex.Publishable {
		t.Fatal("a nom. illeg. superfl. synonym must not be publishable")
	}
	if ex.Exclusion != domain.ExclusionNomStatus {
		t.Errorf("exclusion = %q, want %q", ex.Exclusion, domain.ExclusionNomStatus)
	}
	if ex.Reason == "" {
		t.Error("exclusion must carry a stated reason")
	}
	if ex.Status.Raw != ", nom. illeg. superfl." {
		t.Errorf("raw status = %q, want it preserved verbatim", ex.Status.Raw)
	}
}

// TestRankSynonyms_UnclassifiedIsWithheldAndCounted: an unclassified status
// must never silently mean "fine to publish". It is withheld from the
// publishable block, keeps its raw value visible, and is counted.
func TestRankSynonyms_UnclassifiedIsWithheldAndCounted(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "tail", NomStatus: `published as "mutatio nova"`, Homotypic: boolPtr(true)},
		{NameID: "auct", NomStatus: ", sensu auct.", Homotypic: boolPtr(true)},
		{NameID: "clean", Homotypic: boolPtr(false)},
	}
	got := domain.RankSynonyms(items, domain.SynonymOptions{})

	if got[0].Candidate.NameID != "clean" {
		t.Fatalf("order = %v, want the publishable name first", ids(got))
	}
	for _, r := range got[1:] {
		if r.Publishable {
			t.Errorf("%q with unclassified status must be withheld", r.Candidate.NameID)
		}
		if r.Exclusion != domain.ExclusionUnclassifiedStatus {
			t.Errorf("%q exclusion = %q, want %q", r.Candidate.NameID, r.Exclusion, domain.ExclusionUnclassifiedStatus)
		}
		if r.Status.Raw == "" {
			t.Errorf("%q must keep its raw status visible", r.Candidate.NameID)
		}
	}

	sum := domain.SummarizeSynonyms(got)
	if sum.Total != 3 || sum.Publishable != 1 {
		t.Fatalf("summary = %+v, want 3 total / 1 publishable", sum)
	}
	if sum.Excluded[domain.ExclusionUnclassifiedStatus] != 2 {
		t.Fatalf("summary unclassified count = %d, want 2", sum.Excluded[domain.ExclusionUnclassifiedStatus])
	}
	wantRaw := []string{", sensu auct.", `published as "mutatio nova"`}
	if !reflect.DeepEqual(sum.UnclassifiedStatuses, wantRaw) {
		t.Fatalf("summary raw statuses = %v, want %v", sum.UnclassifiedStatuses, wantRaw)
	}
}

// TestRankSynonyms_RankExclusionIsCallerControlled: varieties and forms are
// excluded only when the caller says it publishes at species level.
func TestRankSynonyms_RankExclusionIsCallerControlled(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "var", Rank: domain.RankVariety},
		{NameID: "form", Rank: domain.RankForm},
		{NameID: "subvar", Rank: domain.RankSubvariety},
		{NameID: "subform", Rank: domain.RankSubform},
		{NameID: "species", Rank: domain.RankSpecies},
	}

	for _, r := range domain.RankSynonyms(items, domain.SynonymOptions{}) {
		if !r.Publishable {
			t.Errorf("%q excluded although the caller asked for no rank filter", r.Candidate.NameID)
		}
	}

	got := domain.RankSynonyms(items, domain.SynonymOptions{ExcludeRanks: domain.RanksBelowSpecies()})
	if got[0].Candidate.NameID != "species" {
		t.Fatalf("order = %v, want the species first", ids(got))
	}
	for _, r := range got[1:] {
		if r.Publishable {
			t.Errorf("%q must be excluded at species-level publication", r.Candidate.NameID)
		}
		if r.Exclusion != domain.ExclusionRank {
			t.Errorf("%q exclusion = %q, want %q", r.Candidate.NameID, r.Exclusion, domain.ExclusionRank)
		}
	}
}

// TestRankSynonyms_StatusExclusionOutranksRank: rule 1 before rule 2 — when
// both apply the nomenclatural defect is the reported reason.
func TestRankSynonyms_StatusExclusionOutranksRank(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "both", Rank: domain.RankVariety, NomStatus: ", nom. nud."},
	}
	got := domain.RankSynonyms(items, domain.SynonymOptions{ExcludeRanks: domain.RanksBelowSpecies()})
	if got[0].Exclusion != domain.ExclusionNomStatus {
		t.Fatalf("exclusion = %q, want %q", got[0].Exclusion, domain.ExclusionNomStatus)
	}
}

// TestRankSynonyms_DeterministicAcrossShuffledInput: identical input in any
// order must yield identical output — the tiebreaker is total, not merely
// stable.
func TestRankSynonyms_DeterministicAcrossShuffledInput(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "n1", Homotypic: boolPtr(true)},
		{NameID: "n2", Homotypic: boolPtr(true)},
		{NameID: "n3", Homotypic: nil},
		{NameID: "n4", Homotypic: boolPtr(false)},
		{NameID: "n5", Homotypic: nil, NomStatus: ", nom. nud."},
	}
	want := ids(domain.RankSynonyms(items, domain.SynonymOptions{}))

	for shift := 1; shift < len(items); shift++ {
		shuffled := append(append([]domain.SynonymCandidate{}, items[shift:]...), items[:shift]...)
		if got := ids(domain.RankSynonyms(shuffled, domain.SynonymOptions{})); !reflect.DeepEqual(got, want) {
			t.Fatalf("shift %d: order = %v, want %v", shift, got, want)
		}
	}
}

// TestRankSynonyms_DoesNotMutateInput mirrors RankSuggestions' purity
// guarantee.
func TestRankSynonyms_DoesNotMutateInput(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "b", Homotypic: boolPtr(false)},
		{NameID: "a", Homotypic: boolPtr(true)},
	}
	_ = domain.RankSynonyms(items, domain.SynonymOptions{})
	if items[0].NameID != "b" || items[1].NameID != "a" {
		t.Fatalf("input slice was mutated: %v", items)
	}
}

func TestRankSynonyms_Empty(t *testing.T) {
	if got := domain.RankSynonyms(nil, domain.SynonymOptions{}); len(got) != 0 {
		t.Fatalf("RankSynonyms(nil) = %v, want empty", got)
	}
	sum := domain.SummarizeSynonyms(nil)
	if sum.Total != 0 || sum.Publishable != 0 || len(sum.Excluded) != 0 || len(sum.UnclassifiedStatuses) != 0 {
		t.Fatalf("SummarizeSynonyms(nil) = %+v, want zero", sum)
	}
}

// TestRankSynonyms_TotalOrder pins the full sort key as a TOTAL order:
// typification block first, NameID ascending inside it. A merely "stable"
// comparator (one that reports equal items as less-than) or a reversed
// tiebreaker would both fail here.
func TestRankSynonyms_TotalOrder(t *testing.T) {
	items := []domain.SynonymCandidate{
		{NameID: "x3", Homotypic: boolPtr(false)},
		{NameID: "h3", Homotypic: boolPtr(true)},
		{NameID: "x1", Homotypic: boolPtr(false)},
		{NameID: "h1", Homotypic: boolPtr(true)},
		{NameID: "x2", Homotypic: boolPtr(false)},
		{NameID: "h2", Homotypic: boolPtr(true)},
	}
	want := []string{"h1", "h2", "h3", "x1", "x2", "x3"}
	if got := ids(domain.RankSynonyms(items, domain.SynonymOptions{})); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// TestNomStatusVerdict_Reason pins the three shapes of the stated reason:
// absence is spelled out as absence, an unmatched value says so, and a
// matched value names the tokens that fired.
func TestNomStatusVerdict_Reason(t *testing.T) {
	if got := domain.ClassifyNomStatus("").Reason(); !strings.Contains(got, "no nom_status recorded") {
		t.Errorf("absent reason = %q, want it to state that nothing was recorded", got)
	}
	if got := domain.ClassifyNomStatus("[Cusc.: 184]").Reason(); !strings.Contains(got, "no known token") {
		t.Errorf("long-tail reason = %q, want it to state that no token matched", got)
	}
	got := domain.ClassifyNomStatus(", nom. illeg. superfl.").Reason()
	for _, want := range []string{"nom. illeg. superfl.", "disqualifying", `"illeg"`, `"superfl"`} {
		if !strings.Contains(got, want) {
			t.Errorf("disqualifying reason = %q, want it to contain %q", got, want)
		}
	}
}

func ids(rel []domain.SynonymRelevance) []string {
	out := make([]string, 0, len(rel))
	for _, r := range rel {
		out = append(out, r.Candidate.NameID)
	}
	return out
}
