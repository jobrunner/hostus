package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// The sec. spaces and concepts the translate tests run against: one name in
// three reference spaces, with A --congruent--> B and B --includes--> C, so
// both the typed-relation rendering and the one-hop boundary are
// observable over the real HTTP surface.
const (
	tSecA = "sec-rothmaler"
	tSecB = "sec-wh98"
	tSecC = "sec-hegi"

	tConceptA = "cdm:concept:a"
	tConceptB = "cdm:concept:b"
	tConceptC = "cdm:concept:c"
)

func translateConcept(uuid, sec, title string) application.CDMConceptRow {
	return application.CDMConceptRow{
		ConceptUUID: uuid, ScientificName: "Abies alba", Authorship: "Mill.",
		Rank: "Species", Status: "Accepted", SecUUID: sec, SecTitle: title,
	}
}

func translateRelation(from, to, relType string) application.CDMRelationRow {
	yes := true
	return application.CDMRelationRow{
		FromUUID: from, ToUUID: to, RelationType: relType,
		IsConceptRelation: &yes, RelationshipUUID: from + to + relType,
	}
}

func translateRepoDB(t *testing.T, relations ...application.CDMRelationRow) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	concepts := []application.CDMConceptRow{
		translateConcept("a", tSecA, "Rothmaler, Exkursionsflora, 8. Aufl."),
		translateConcept("b", tSecB, "Wisskirchen & Haeupler 1998: Standardliste"),
		translateConcept("c", tSecC, "HEGI: Illustrierte Flora von Mitteleuropa"),
	}
	meta := domain.BackboneVersion{
		ID: "cdm", Version: "2026-08-02",
		SourceURL:      "https://api.cybertaxonomy.org/rl_standardliste",
		Redistribution: domain.RedistributionUnknown,
	}
	if _, err := application.IngestCDM(context.Background(), db, concepts, relations, meta); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}
	return db
}

func postTranslate(t *testing.T, db *sqlite.DB, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httpx.NewRouter(httpx.Deps{Repo: db})
	req := httptest.NewRequest(http.MethodPost, "/v1/translate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func decodeTranslate(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v (body %s)", err, rr.Body.String())
	}
	return got
}

func firstCandidate(t *testing.T, got map[string]any) map[string]any {
	t.Helper()
	cands, ok := got["candidates"].([]any)
	if !ok || len(cands) != 1 {
		t.Fatalf("candidates = %v, want exactly one", got["candidates"])
	}
	return cands[0].(map[string]any)
}

// TestTranslateCongruentIsTheOnlyEqualityOnTheWire: the wire shape must
// carry is_equality EXPLICITLY on every candidate — true only for
// congruent, and present (not omitted) when false, since an absent field
// reads as "unknown" rather than "no".
func TestTranslateCongruentIsTheOnlyEqualityOnTheWire(t *testing.T) {
	cases := []struct {
		relType      string
		wantRelation string
		wantEquality bool
	}{
		{"Congruent to", "congruent", true},
		{"Includes", "includes", false},
		{"Overlaps", "overlaps", false},
		{"Included in or Includes or Overlaps", "includes_or_included_in_or_overlaps", false},
		{"Not Congruent to", "not_congruent", false},
		{"is pro parte synonym for", "pro_parte", false},
	}
	for _, tc := range cases {
		db := translateRepoDB(t, translateRelation("a", "b", tc.relType))
		got := decodeTranslate(t, postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`"}`))

		if got["result"] != "translated" {
			t.Errorf("%s: result = %v, want translated", tc.relType, got["result"])
		}
		c := firstCandidate(t, got)
		if c["relation"] != tc.wantRelation {
			t.Errorf("%s: relation = %v, want %q", tc.relType, c["relation"], tc.wantRelation)
		}
		eq, present := c["is_equality"]
		if !present {
			t.Fatalf("%s: is_equality missing from the payload", tc.relType)
		}
		if eq != tc.wantEquality {
			t.Errorf("%s: is_equality = %v, want %v", tc.relType, eq, tc.wantEquality)
		}
		if !tc.wantEquality && c["note"] == nil {
			t.Errorf("%s: no note on a non-identity relation", tc.relType)
		}
	}
}

// TestTranslateOverlapsCannotBeReadAsEquality is the explicit
// anti-conflation check: nothing in an overlaps payload says "same".
func TestTranslateOverlapsCannotBeReadAsEquality(t *testing.T) {
	db := translateRepoDB(t, translateRelation("a", "b", "Overlaps"))
	rr := postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`"}`)
	body := rr.Body.String()

	if !strings.Contains(body, `"is_equality":false`) {
		t.Errorf("body does not carry is_equality:false — an overlaps result could be read as identity: %s", body)
	}
	if strings.Contains(body, `"is_equality":true`) {
		t.Errorf("overlaps body claims equality: %s", body)
	}
	c := firstCandidate(t, decodeTranslate(t, rr))
	if c["relation"] != "overlaps" || c["relation_from_source"] != "overlaps" {
		t.Errorf("relation fields = %v / %v, want overlaps twice", c["relation"], c["relation_from_source"])
	}
}

// TestTranslateUncertainKeepsItsOwnValue: ⊂⊃⊕ reaches the client as its own
// relation value, not as overlaps.
func TestTranslateUncertainKeepsItsOwnValue(t *testing.T) {
	db := translateRepoDB(t, translateRelation("a", "b", "Included in or Includes or Overlaps"))
	body := postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`"}`).Body.String()

	if !strings.Contains(body, `"relation":"includes_or_included_in_or_overlaps"`) {
		t.Errorf("uncertain relation not rendered verbatim: %s", body)
	}
	if strings.Contains(body, `"relation":"overlaps"`) {
		t.Errorf("uncertain relation was flattened onto overlaps: %s", body)
	}
}

// TestTranslateDirectionIsExplicit: an incoming edge is rendered with its
// stored statement AND a source-first reading, and the two are
// distinguishable.
func TestTranslateDirectionIsExplicit(t *testing.T) {
	db := translateRepoDB(t, translateRelation("a", "b", "Includes"))

	outgoing := firstCandidate(t, decodeTranslate(t, postTranslate(t, db,
		`{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`"}`)))
	if outgoing["direction"] != "source_to_target" {
		t.Errorf("direction = %v, want source_to_target", outgoing["direction"])
	}
	if outgoing["relation_from_source"] != "includes" {
		t.Errorf("relation_from_source = %v, want includes", outgoing["relation_from_source"])
	}

	incoming := firstCandidate(t, decodeTranslate(t, postTranslate(t, db,
		`{"concept_id":"`+tConceptB+`","target_space":"`+tSecA+`"}`)))
	if incoming["direction"] != "target_to_source" {
		t.Errorf("direction = %v, want target_to_source", incoming["direction"])
	}
	if incoming["relation"] != "includes" {
		t.Errorf("relation = %v, want the stored includes", incoming["relation"])
	}
	if incoming["relation_from_source"] != "included_in" {
		t.Errorf("relation_from_source = %v, want included_in", incoming["relation_from_source"])
	}
	stmt := incoming["statement"].(map[string]any)
	if stmt["from"] != tConceptA || stmt["to"] != tConceptB || stmt["relation"] != "includes" {
		t.Errorf("statement = %v, want the stored a includes b", stmt)
	}
}

// TestTranslateIncomingProParteOmitsTheSourceFirstReading: pro parte has no
// inverse, so relation_from_source is absent — with the stored statement
// and a note in its place, never a fabricated inverse.
func TestTranslateIncomingProParteOmitsTheSourceFirstReading(t *testing.T) {
	db := translateRepoDB(t, translateRelation("a", "b", "is pro parte synonym for"))
	c := firstCandidate(t, decodeTranslate(t, postTranslate(t, db,
		`{"concept_id":"`+tConceptB+`","target_space":"`+tSecA+`"}`)))

	if _, present := c["relation_from_source"]; present {
		t.Errorf("relation_from_source = %v, want absent for an incoming pro parte", c["relation_from_source"])
	}
	if c["relation"] != "pro_parte" {
		t.Errorf("relation = %v, want pro_parte", c["relation"])
	}
	if c["note"] == nil {
		t.Errorf("no note explaining the missing inverse")
	}
}

// TestTranslateNoRelationIsAn200EmptyAnswer is the second non-negotiable on
// the wire: the database HAS a same-named concept in the target space, and
// the response must still be an explicitly empty, relation-free answer.
func TestTranslateNoRelationIsAn200EmptyAnswer(t *testing.T) {
	db := translateRepoDB(t)
	rr := postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`"}`)
	got := decodeTranslate(t, rr)

	if got["result"] != "no_relation_recorded" {
		t.Errorf("result = %v, want no_relation_recorded", got["result"])
	}
	cands, ok := got["candidates"].([]any)
	if !ok {
		t.Fatalf("candidates is not an array (%T) — the empty answer must be explicit, not omitted", got["candidates"])
	}
	if len(cands) != 0 {
		t.Errorf("candidates = %v, want empty", cands)
	}
	if _, present := got["unrelated_name_candidates"]; present {
		t.Errorf("name candidates present without opting in: %v", got["unrelated_name_candidates"])
	}
	if got["note"] == nil || !strings.Contains(got["note"].(string), "NICHT") {
		t.Errorf("note = %v, want the explicit \"absence of a record is not absence of a relation\" wording", got["note"])
	}
	// The target space is still named, so the client knows where it looked.
	if got["target_space"].(map[string]any)["title"] == nil {
		t.Errorf("target_space carries no title: %v", got["target_space"])
	}
}

// TestTranslateNameCandidatesAreOptInAndSeparate: on explicit opt-in the
// same-name concepts arrive under their own key, marked for review, and
// never inside `candidates`.
func TestTranslateNameCandidatesAreOptInAndSeparate(t *testing.T) {
	db := translateRepoDB(t)
	got := decodeTranslate(t, postTranslate(t, db,
		`{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`","include_name_candidates":true}`))

	if len(got["candidates"].([]any)) != 0 {
		t.Errorf("candidates = %v, want empty — a name match is not a translation", got["candidates"])
	}
	names, ok := got["unrelated_name_candidates"].([]any)
	if !ok || len(names) != 1 {
		t.Fatalf("unrelated_name_candidates = %v, want one entry", got["unrelated_name_candidates"])
	}
	entry := names[0].(map[string]any)
	if entry["concept_id"] != tConceptB {
		t.Errorf("name candidate = %v, want %q", entry["concept_id"], tConceptB)
	}
	if entry["requires_review"] != true {
		t.Errorf("name candidate requires_review = %v, want true", entry["requires_review"])
	}
	if _, present := entry["relation"]; present {
		t.Errorf("name candidate carries a relation field: %v", entry)
	}
	if got["requires_review"] != true {
		t.Errorf("response requires_review = %v, want true", got["requires_review"])
	}
	if got["result"] != "no_relation_recorded" {
		t.Errorf("result = %v, want no_relation_recorded even with a name block", got["result"])
	}
}

// TestTranslateOneHopBoundaryOverHTTP: a --congruent--> b --includes--> c,
// asked for a -> c's space, must answer "nothing recorded".
func TestTranslateOneHopBoundaryOverHTTP(t *testing.T) {
	db := translateRepoDB(t,
		translateRelation("a", "b", "Congruent to"),
		translateRelation("b", "c", "Includes"),
	)
	got := decodeTranslate(t, postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"`+tSecC+`"}`))

	if got["result"] != "no_relation_recorded" {
		t.Errorf("result = %v, want no_relation_recorded: chaining two hops is not sound", got["result"])
	}
	if len(got["candidates"].([]any)) != 0 {
		t.Errorf("candidates = %v, want empty", got["candidates"])
	}
	if got["max_hops"] != float64(1) {
		t.Errorf("max_hops = %v, want 1", got["max_hops"])
	}
}

func TestTranslateRejectsMoreThanOneHop(t *testing.T) {
	db := translateRepoDB(t, translateRelation("a", "b", "Congruent to"))
	rr := postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"`+tSecB+`","max_hops":2}`)
	assertTranslateError(t, rr, http.StatusBadRequest, "INVALID_QUERY")
}

func TestTranslateUnknownConceptIs404(t *testing.T) {
	db := translateRepoDB(t)
	rr := postTranslate(t, db, `{"concept_id":"cdm:concept:nope","target_space":"`+tSecB+`"}`)
	assertTranslateError(t, rr, http.StatusNotFound, "NOT_FOUND")
}

// TestTranslateUnknownTargetSpaceIs404 keeps a typo'd space from looking
// like an honest "nothing recorded".
func TestTranslateUnknownTargetSpaceIs404(t *testing.T) {
	db := translateRepoDB(t)
	rr := postTranslate(t, db, `{"concept_id":"`+tConceptA+`","target_space":"sec-typo"}`)
	assertTranslateError(t, rr, http.StatusNotFound, "NOT_FOUND")
}

func TestTranslateUnresolvableVerbatimIs422(t *testing.T) {
	db := translateRepoDB(t)
	rr := postTranslate(t, db, `{"verbatim":"Quercus nonexistens","target_space":"`+tSecB+`"}`)
	assertTranslateError(t, rr, http.StatusUnprocessableEntity, "UNRESOLVABLE")
}

func TestTranslateMalformedRequestsAre400(t *testing.T) {
	db := translateRepoDB(t)
	cases := map[string]string{
		"malformed json":  `{"concept_id":`,
		"neither id/name": `{"target_space":"` + tSecB + `"}`,
		"both id and name": `{"concept_id":"` + tConceptA + `","verbatim":"Abies alba","target_space":"` +
			tSecB + `"}`,
		"no target space": `{"concept_id":"` + tConceptA + `"}`,
	}
	for name, body := range cases {
		rr := postTranslate(t, db, body)
		t.Run(name, func(t *testing.T) {
			assertTranslateError(t, rr, http.StatusBadRequest, "INVALID_QUERY")
		})
	}
}

// TestTranslateEntryByVerbatimName resolves through /v1/match's path. The
// name is unique to one sec. space here (Carex ornithopoda), so it resolves
// exactly.
func TestTranslateEntryByVerbatimName(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	concepts := []application.CDMConceptRow{
		{ConceptUUID: "a", ScientificName: "Carex ornithopoda", Authorship: "Willd.",
			Rank: "Species", Status: "Accepted", SecUUID: tSecA, SecTitle: "Rothmaler, 8. Aufl."},
		{ConceptUUID: "b", ScientificName: "Carex ornithopodioides", Authorship: "Hausm.",
			Rank: "Species", Status: "Accepted", SecUUID: tSecB, SecTitle: "Wisskirchen & Haeupler 1998"},
	}
	meta := domain.BackboneVersion{ID: "cdm", Version: "2026-08-02", Redistribution: domain.RedistributionUnknown}
	if _, err := application.IngestCDM(context.Background(), db, concepts,
		[]application.CDMRelationRow{translateRelation("a", "b", "Overlaps")}, meta); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}

	got := decodeTranslate(t, postTranslate(t, db,
		`{"verbatim":"Carex ornithopoda Willd.","target_space":"`+tSecB+`"}`))

	entry := got["entry"].(map[string]any)
	if entry["mode"] != "verbatim" || entry["verbatim"] != "Carex ornithopoda Willd." {
		t.Errorf("entry = %v, want the verbatim mode echoing the query", entry)
	}
	c := firstCandidate(t, got)
	if c["relation"] != "overlaps" || c["is_equality"] != false {
		t.Errorf("candidate = %v, want a non-identity overlaps result", c)
	}
}

func assertTranslateError(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", rr.Code, wantStatus, rr.Body.String())
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding error envelope: %v (body %s)", err, rr.Body.String())
	}
	if got.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q", got.Error.Code, wantCode)
	}
	if got.Error.Message == "" {
		t.Errorf("error message is empty")
	}
}
