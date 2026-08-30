package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// The WCVP fixture's two useful UC5 shapes, both real ingested concepts:
// Festuca ovina carries two nomenclatural defects among five synonyms,
// Corynephorus canescens carries three infraspecific ranks among four.
const (
	festucaOvinaConceptID = "wcvp:concept:415853"
	corynephorusUC5ID     = "wcvp:concept:405825"
	// corynephorusGenusID has no synonyms at all in the fixture — the
	// "empty list, not a 404" case.
	corynephorusGenusID = "wcvp:concept:451295"
	// uc5ConceptID is the seeded acceptance case (see seedUC5Concept).
	uc5ConceptID = "uc5:concept:corynephorus"
)

// seedUC5Concept writes the UC5 acceptance case straight through the
// output.Repository port that the router was handed, the same technique
// internal/app/integration_test.go's seedFestucaOvinaAggregate uses.
//
// It exists because the WCVP fixture cannot express the one relation UC5
// rule 4 turns on: none of its accepted names has a basionym that is also
// one of its own synonyms, so `is_basionym` is false throughout it and a
// handler that never rendered the flag would still pass. The shape seeded
// here is the measured one — Corynephorus canescens' basionym Aira
// canescens L., its homotypic recombination Weingaertneria canescens, the
// illegitimate Corynephorus incanescens Bubani (", nom. illeg. superfl.",
// wcvp:name:405842 in the real index) and one infraspecific rank.
func seedUC5Concept(t *testing.T, db *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "uc5", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	basionym := domain.Name{ID: "uc5:name:aira-canescens", Canonical: "Aira canescens", Authorship: "L.", Rank: domain.RankSpecies}
	accepted := domain.Name{ID: "uc5:name:corynephorus-canescens", Canonical: "Corynephorus canescens", Authorship: "(L.) P.Beauv.", Rank: domain.RankSpecies, BasionymID: basionym.ID}
	others := []domain.Name{
		{ID: "uc5:name:incanescens", Canonical: "Corynephorus incanescens", Authorship: "Bubani", Rank: domain.RankSpecies, NomStatus: ", nom. illeg. superfl."},
		{ID: "uc5:name:var-montana", Canonical: "Corynephorus canescens var. montana", Authorship: "Cout.", Rank: domain.RankVariety},
		{ID: "uc5:name:weingaertneria", Canonical: "Weingaertneria canescens", Authorship: "(L.) Bernh.", Rank: domain.RankSpecies},
	}
	// basionym first: accepted.BasionymID is a foreign key onto name(id).
	for _, n := range append([]domain.Name{basionym, accepted}, others...) {
		if err := tx.UpsertName(n); err != nil {
			t.Fatalf("UpsertName(%q): unexpected error: %v", n.ID, err)
		}
	}

	concept := domain.Concept{ID: uc5ConceptID, BackboneID: "uc5", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, accepted.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(accepted): unexpected error: %v", err)
	}
	homotypic := true
	links := []struct {
		nameID    string
		homotypic *bool
	}{
		{basionym.ID, &homotypic},
		{"uc5:name:weingaertneria", &homotypic},
		{"uc5:name:incanescens", nil},
		{"uc5:name:var-montana", nil},
	}
	for _, l := range links {
		if err := tx.LinkName(concept.ID, l.nameID, "synonym", l.homotypic); err != nil {
			t.Fatalf("LinkName(%q): unexpected error: %v", l.nameID, err)
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// synonymsResponse mirrors the handler's wire shape for decoding. The
// pointer-free bools are deliberate: every judgement field is required to
// be present, and a missing one would decode to the zero value here, so
// presence is asserted separately via rawSynonymKeys.
type synonymsResponse struct {
	ConceptID       string `json:"concept_id"`
	Relevance       string `json:"relevance"`
	PublicationRank string `json:"publication_rank"`
	Ordering        string `json:"ordering"`
	Synonyms        []struct {
		Position           int    `json:"position"`
		NameID             string `json:"name_id"`
		Canonical          string `json:"canonical"`
		Authorship         string `json:"authorship"`
		Rank               string `json:"rank"`
		RankVerbatim       string `json:"rank_verbatim"`
		Typification       string `json:"typification"`
		IsBasionym         bool   `json:"is_basionym"`
		NomStatus          string `json:"nom_status"`
		NomStatusJudgement string `json:"nom_status_judgement"`
		Publishable        bool   `json:"publishable"`
		Exclusion          string `json:"exclusion"`
		Reason             string `json:"reason"`
	} `json:"synonyms"`
	Summary struct {
		Total                int            `json:"total"`
		Publishable          int            `json:"publishable"`
		Returned             int            `json:"returned"`
		Truncated            int            `json:"truncated"`
		Absent               int            `json:"absent"`
		Excluded             map[string]int `json:"excluded"`
		UnclassifiedStatuses []string       `json:"unclassified_statuses"`
	} `json:"summary"`
}

// getSynonyms issues one GET against the real router and returns the
// recorder plus the decoded body (nil-safe for error responses).
func getSynonyms(t *testing.T, repo output.Repository, conceptID, query string) (*httptest.ResponseRecorder, synonymsResponse) {
	t.Helper()
	r := httpx.NewRouter(httpx.Deps{Repo: repo})
	url := "/v1/concept/" + conceptID + "/synonyms"
	if query != "" {
		url += "?" + query
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, url, nil))

	var body synonymsResponse
	if rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding %s: %v (body %s)", url, err, rr.Body.String())
		}
	}
	return rr, body
}

func synonymNameIDs(body synonymsResponse) []string {
	out := make([]string, len(body.Synonyms))
	for i, s := range body.Synonyms {
		out[i] = s.NameID
	}
	return out
}

func sameStrings(a, b []string) bool {
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

// TestSynonyms_UC5AcceptanceCase is the case the task is defined by:
// Corynephorus incanescens (", nom. illeg. superfl.") must be gone and Aira
// canescens L. must lead, as the accepted name's basionym.
func TestSynonyms_UC5AcceptanceCase(t *testing.T) {
	repo := seededRepo(t)
	seedUC5Concept(t, repo)

	rr, body := getSynonyms(t, repo, uc5ConceptID, "relevance=publication&rank=species&max=3")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}

	want := []string{"uc5:name:aira-canescens", "uc5:name:weingaertneria"}
	if got := synonymNameIDs(body); !sameStrings(got, want) {
		t.Fatalf("synonyms = %v, want %v", got, want)
	}
	lead := body.Synonyms[0]
	if !lead.IsBasionym {
		t.Errorf("lead synonym %q is_basionym = false; UC5 rule 4 requires the basionym to lead", lead.NameID)
	}
	if lead.Typification != string(domain.TypificationHomotypic) {
		t.Errorf("lead typification = %q, want %q", lead.Typification, domain.TypificationHomotypic)
	}
	if lead.Position != 1 || body.Synonyms[1].Position != 2 {
		t.Errorf("positions = %d,%d, want 1,2", lead.Position, body.Synonyms[1].Position)
	}
	for _, s := range body.Synonyms {
		if s.NameID == "uc5:name:incanescens" {
			t.Errorf("Corynephorus incanescens (%q) is in the publication list", s.NomStatus)
		}
	}
	if body.Summary.Total != 4 || body.Summary.Publishable != 2 {
		t.Errorf("summary total/publishable = %d/%d, want 4/2", body.Summary.Total, body.Summary.Publishable)
	}
	if body.Summary.Excluded["nom_status"] != 1 || body.Summary.Excluded["rank"] != 1 {
		t.Errorf("excluded = %v, want {nom_status:1, rank:1}", body.Summary.Excluded)
	}
	if body.Ordering == "" {
		t.Error("ordering is empty; the response must state the rule that ordered it")
	}
	assertWireSummaryReconciles(t, body)
}

// TestSynonyms_DefaultIsTheUnfilteredList pins the default-relevance
// decision: no parameter means every synonym, so this endpoint never
// becomes the only door and quietly hides data.
func TestSynonyms_DefaultIsTheUnfilteredList(t *testing.T) {
	repo := seededRepo(t)

	_, all := getSynonyms(t, repo, festucaOvinaConceptID, "")
	if all.Relevance != "all" {
		t.Errorf("relevance = %q, want %q", all.Relevance, "all")
	}
	if len(all.Synonyms) != 5 {
		t.Fatalf("unfiltered returned %d synonyms, want all 5", len(all.Synonyms))
	}

	_, pub := getSynonyms(t, repo, festucaOvinaConceptID, "relevance=publication")
	if len(pub.Synonyms) != 3 {
		t.Fatalf("publication returned %d synonyms, want 3", len(pub.Synonyms))
	}
	if len(pub.Synonyms) >= len(all.Synonyms) {
		t.Errorf("filtered (%d) is not shorter than unfiltered (%d) — the two modes must differ", len(pub.Synonyms), len(all.Synonyms))
	}
	// Both modes carry the same summary: the filter changes the list, not
	// the facts about the concept.
	if all.Summary.Total != pub.Summary.Total || all.Summary.Publishable != pub.Summary.Publishable {
		t.Errorf("summaries differ: all=%+v pub=%+v", all.Summary, pub.Summary)
	}
	if all.Summary.Returned != 5 || pub.Summary.Returned != 3 {
		t.Errorf("returned = %d / %d, want 5 / 3", all.Summary.Returned, pub.Summary.Returned)
	}
}

// TestSynonyms_UnfilteredStillStatesWhyEachWasWithheld: relevance=all is
// not the dumb mode.
func TestSynonyms_UnfilteredStillStatesWhyEachWasWithheld(t *testing.T) {
	repo := seededRepo(t)

	_, all := getSynonyms(t, repo, festucaOvinaConceptID, "")
	var excluded int
	for _, s := range all.Synonyms {
		if s.Publishable {
			continue
		}
		excluded++
		if s.Exclusion == "" || s.Reason == "" {
			t.Errorf("withheld synonym %q carries exclusion=%q reason=%q; both must be set", s.NameID, s.Exclusion, s.Reason)
		}
		if s.NomStatusJudgement != string(domain.JudgementDisqualifying) {
			t.Errorf("synonym %q judgement = %q, want %q", s.NameID, s.NomStatusJudgement, domain.JudgementDisqualifying)
		}
	}
	if excluded != 2 {
		t.Errorf("counted %d withheld synonyms, want 2 (Avena dura, Festuca ovina var. vulgaris)", excluded)
	}
}

// TestSynonyms_ExclusionSummaryCountsRankExclusions uses the fixture's
// Corynephorus canescens, whose three infraspecific synonyms carry no
// nomenclatural defect and are therefore removed by the rank rule alone.
func TestSynonyms_ExclusionSummaryCountsRankExclusions(t *testing.T) {
	repo := seededRepo(t)

	_, pub := getSynonyms(t, repo, corynephorusUC5ID, "relevance=publication&rank=species")
	if pub.PublicationRank != "species" {
		t.Errorf("publication_rank = %q, want %q", pub.PublicationRank, "species")
	}
	if pub.Summary.Total != 4 {
		t.Fatalf("summary.total = %d, want 4", pub.Summary.Total)
	}
	assertWireSummaryReconciles(t, pub)
	if pub.Summary.Excluded["rank"] != 3 {
		t.Errorf("excluded[rank] = %d, want 3 (two varieties and one form)", pub.Summary.Excluded["rank"])
	}
	if len(pub.Synonyms) != 1 || pub.Synonyms[0].Rank != string(domain.RankSubspecies) {
		t.Errorf("publication list = %v, want the single SUBSPECIES synonym", synonymNameIDs(pub))
	}

	// Without `rank`, nothing is excluded by rank at all.
	_, noRank := getSynonyms(t, repo, corynephorusUC5ID, "relevance=publication")
	if noRank.Summary.Excluded["rank"] != 0 || len(noRank.Synonyms) != 4 {
		t.Errorf("without rank: excluded[rank]=%d, %d synonyms; want 0 and 4", noRank.Summary.Excluded["rank"], len(noRank.Synonyms))
	}
	if noRank.PublicationRank != "" {
		t.Errorf("publication_rank = %q, want it omitted", noRank.PublicationRank)
	}
}

// TestSynonyms_MaxTruncatesAfterRanking: with max=1 the single returned
// synonym must be the RANKED first (the homotypic Bromus ovinus), not the
// first candidate by name id (Avena dura, which is not publishable at all).
func TestSynonyms_MaxTruncatesAfterRanking(t *testing.T) {
	repo := seededRepo(t)

	_, body := getSynonyms(t, repo, festucaOvinaConceptID, "relevance=publication&max=1")
	if len(body.Synonyms) != 1 {
		t.Fatalf("got %d synonyms, want 1", len(body.Synonyms))
	}
	if body.Synonyms[0].Typification != string(domain.TypificationHomotypic) {
		t.Errorf("max=1 returned %q (%s); want the homotypic synonym, which only post-ranking truncation can pick",
			body.Synonyms[0].NameID, body.Synonyms[0].Typification)
	}
	if body.Summary.Truncated != 2 {
		t.Errorf("summary.truncated = %d, want 2", body.Summary.Truncated)
	}
	if body.Summary.Total != 5 || body.Summary.Publishable != 3 {
		t.Errorf("truncation altered the summary (%+v); it must describe the concept, not the page", body.Summary)
	}
	assertWireSummaryReconciles(t, body)
}

// assertWireSummaryReconciles checks, on the WIRE shape, the arithmetic a
// client reads the summary for:
//
//	publishable + Sum(excluded) == total
//	returned + truncated        == publishable  (under relevance=publication)
//
// plus the rule that truncation never shows up as an exclusion reason.
// Folding Truncated into `excluded` would break the first sum on every
// capped response — and a capped response is the only place it can break,
// which is why this runs on one.
func assertWireSummaryReconciles(t *testing.T, body synonymsResponse) {
	t.Helper()
	if _, ok := body.Summary.Excluded["truncated"]; ok {
		t.Errorf("excluded carries a \"truncated\" key: a capped synonym was not judged irrelevant and must not be counted as excluded (%v)", body.Summary.Excluded)
	}
	sum := body.Summary.Publishable
	for _, n := range body.Summary.Excluded {
		sum += n
	}
	if sum != body.Summary.Total {
		t.Errorf("publishable + excluded = %d, want total = %d (excluded = %v)", sum, body.Summary.Total, body.Summary.Excluded)
	}
	if body.Summary.Returned != len(body.Synonyms) {
		t.Errorf("summary.returned = %d, but the array carries %d entries", body.Summary.Returned, len(body.Synonyms))
	}
	if body.Relevance == "publication" {
		if got := body.Summary.Returned + body.Summary.Truncated; got != body.Summary.Publishable {
			t.Errorf("returned + truncated = %d, want publishable = %d", got, body.Summary.Publishable)
		}
	}
}

func TestSynonyms_UnknownConceptIsNotFound(t *testing.T) {
	repo := seededRepo(t)

	rr, _ := getSynonyms(t, repo, "wcvp:concept:does-not-exist", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"NOT_FOUND"`) {
		t.Errorf("body %s does not carry the NOT_FOUND code", rr.Body.String())
	}
}

func TestSynonyms_ConceptWithoutSynonymsIsEmptyList(t *testing.T) {
	repo := seededRepo(t)

	rr, body := getSynonyms(t, repo, corynephorusGenusID, "relevance=publication")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (an existing concept with no synonyms is not a 404)", rr.Code)
	}
	if len(body.Synonyms) != 0 {
		t.Errorf("got %d synonyms, want 0", len(body.Synonyms))
	}
	if body.Summary.Total != 0 {
		t.Errorf("summary.total = %d, want 0", body.Summary.Total)
	}
	if body.Summary.Excluded == nil || body.Summary.UnclassifiedStatuses == nil {
		t.Errorf("summary.excluded/%v unclassified_statuses/%v must be present (possibly empty), never null",
			body.Summary.Excluded, body.Summary.UnclassifiedStatuses)
	}
	if !strings.Contains(rr.Body.String(), `"synonyms":[]`) {
		t.Errorf("body %s renders synonyms as null rather than []", rr.Body.String())
	}
}

func TestSynonyms_InvalidQueryNamesTheOffendingValue(t *testing.T) {
	repo := seededRepo(t)

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"unknown relevance", "relevance=wichtig", `unknown relevance "wichtig"`},
		{"unsupported rank", "rank=genus", `unsupported rank "genus" (supported: species; omit rank for no rank exclusion)`},
		{"non-numeric max", "max=viele", `max "viele" is not an integer`},
		{"negative max", "max=-1", `max "-1"`},
		{"absurd max", "max=999999", `max "999999"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr, _ := getSynonyms(t, repo, festucaOvinaConceptID, tc.query)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rr.Code, rr.Body.String())
			}
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decoding error envelope: %v (body %s)", err, rr.Body.String())
			}
			if envelope.Error.Code != "INVALID_QUERY" {
				t.Errorf("code = %q, want INVALID_QUERY", envelope.Error.Code)
			}
			if !strings.Contains(envelope.Error.Message, tc.want) {
				t.Errorf("message %q does not contain %q", envelope.Error.Message, tc.want)
			}
			if strings.Contains(envelope.Error.Message, "application:") {
				t.Errorf("message %q leaks the internal package prefix", envelope.Error.Message)
			}
		})
	}
}

// TestSynonyms_JudgementFieldsAreAlwaysPresent guards the honesty rule that
// `is_basionym: false` and `publishable: true` are answers, not defaults —
// omitempty on either would make "no" indistinguishable from "not checked".
func TestSynonyms_JudgementFieldsAreAlwaysPresent(t *testing.T) {
	repo := seededRepo(t)

	rr, _ := getSynonyms(t, repo, festucaOvinaConceptID, "")
	var raw struct {
		Synonyms []map[string]any `json:"synonyms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	required := []string{"position", "name_id", "canonical", "rank", "typification", "is_basionym", "nom_status_judgement", "publishable", "reason"}
	for _, s := range raw.Synonyms {
		for _, key := range required {
			if _, ok := s[key]; !ok {
				t.Errorf("synonym %v is missing the required field %q", s["name_id"], key)
			}
		}
		// nom_status is the ONE field whose absence is the statement.
		if _, ok := s["nom_status"]; ok && s["nom_status"] == "" {
			t.Errorf("synonym %v renders nom_status as an empty string; it must be omitted instead", s["name_id"])
		}
	}
}

// TestSynonyms_ConceptEndpointUnchanged pins the constraint that
// /v1/concept/{id} keeps its own synonym shape: no publication verdict, no
// nom_status, no is_basionym leaked into it.
func TestSynonyms_ConceptEndpointUnchanged(t *testing.T) {
	repo := seededRepo(t)

	r := httpx.NewRouter(httpx.Deps{Repo: repo})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+festucaOvinaConceptID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var raw struct {
		Synonyms []map[string]any `json:"synonyms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(raw.Synonyms) != 5 {
		t.Fatalf("/v1/concept returned %d synonyms, want all 5 (it is never filtered)", len(raw.Synonyms))
	}
	for _, s := range raw.Synonyms {
		for _, forbidden := range []string{"nom_status", "is_basionym", "publishable", "exclusion", "typification"} {
			if _, ok := s[forbidden]; ok {
				t.Errorf("/v1/concept synonym gained the field %q; its shape must stay unchanged", forbidden)
			}
		}
	}
}

// failingSynonymsRepo returns a plain (non-sentinel) error from
// SynonymCandidates. It embeds a nil output.Repository, so any other method
// the handler might reach for panics instead of quietly succeeding.
type failingSynonymsRepo struct{ output.Repository }

func (failingSynonymsRepo) SynonymCandidates(context.Context, string) ([]domain.SynonymCandidate, error) {
	return nil, errors.New("disk on fire")
}

// TestSynonyms_RepositoryFailureIsInternalError pins the last branch: a
// storage failure is a 500, never a 404 ("no such concept") or a 200 with
// an empty list — both of which would be false statements about the data.
func TestSynonyms_RepositoryFailureIsInternalError(t *testing.T) {
	rr, _ := getSynonyms(t, failingSynonymsRepo{}, "wcvp:concept:405825", "")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"INTERNAL_ERROR"`) {
		t.Errorf("body %s does not carry the INTERNAL_ERROR code", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "disk on fire") {
		t.Errorf("body %s leaks the internal error text", rr.Body.String())
	}
}

// TestSynonyms_OtherRankCarriesItsVerbatimSpelling: an OTHER-ranked synonym
// is never excluded by rank=species, so it reaches publication lists — and a
// bare "OTHER" would tell a botanist nothing. 3.731 synonym rows in the
// measured index carry such a spelling (`proles` 2.338, `lusus` 658,
// `microgene` 336, `Convariety` 184, `grex` 41).
func TestSynonyms_OtherRankCarriesItsVerbatimSpelling(t *testing.T) {
	repo := seededRepo(t)
	seedOtherRankSynonym(t, repo)

	_, body := getSynonyms(t, repo, otherRankConceptID, "relevance=publication&rank=species")
	if len(body.Synonyms) != 1 {
		t.Fatalf("got %d synonyms, want 1 (an OTHER rank is not excluded by rank=species)", len(body.Synonyms))
	}
	s := body.Synonyms[0]
	if s.Rank != string(domain.RankOther) {
		t.Fatalf("rank = %q, want %q", s.Rank, domain.RankOther)
	}
	if s.RankVerbatim != "lusus" {
		t.Errorf("rank_verbatim = %q, want %q — otherwise the entry renders as a bare OTHER", s.RankVerbatim, "lusus")
	}

	// And it is OMITTED, not empty, for a canonically-ranked synonym.
	rr, _ := getSynonyms(t, repo, festucaOvinaConceptID, "")
	var raw struct {
		Synonyms []map[string]any `json:"synonyms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, item := range raw.Synonyms {
		if _, ok := item["rank_verbatim"]; ok {
			t.Errorf("synonym %v renders rank_verbatim although its rank is %v; SPECIES/VARIETY already name their own spelling", item["name_id"], item["rank"])
		}
	}
}

const otherRankConceptID = "other:concept:corynephorus"

// seedOtherRankSynonym writes one concept whose single synonym ranks OTHER
// with a verbatim "lusus", through the same output.Repository port
// seedUC5Concept uses. The WCVP fixture carries no exotic rank.
func seedOtherRankSynonym(t *testing.T, db *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "other", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	accepted := domain.Name{ID: "other:name:accepted", Canonical: "Corynephorus divaricatus", Authorship: "(Pourr.) Breistr.", Rank: domain.RankSpecies}
	lusus := domain.Name{ID: "other:name:lusus", Canonical: "Corynephorus articulatus", Authorship: "Desf.", Rank: domain.RankOther, RankVerbatim: "lusus"}
	for _, n := range []domain.Name{accepted, lusus} {
		if err := tx.UpsertName(n); err != nil {
			t.Fatalf("UpsertName(%q): unexpected error: %v", n.ID, err)
		}
	}
	concept := domain.Concept{ID: otherRankConceptID, BackboneID: "other", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, accepted.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(accepted): unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, lusus.ID, "synonym", nil); err != nil {
		t.Fatalf("LinkName(synonym): unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}
