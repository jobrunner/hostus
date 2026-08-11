package httpx_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedDuplicateCorynephorusInBackbone ingests a second accepted concept with
// the SAME canonical+author as the WCVP fixture's Corynephorus canescens, but
// in a different backbone — reproducing the multi-backbone ambiguity (WCVP +
// CDM) that the entry_backbone filter resolves. Returns the new backbone id.
func seedDuplicateCorynephorusInBackbone(t *testing.T, db *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	const bb = "cdm-test"
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: bb, Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	name := domain.Name{ID: bb + ":name:cory", Canonical: "Corynephorus canescens", Authorship: "(L.) P.Beauv.", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: bb + ":concept:cory", BackboneID: bb, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return bb
}

// TestHandleMatch_EntryBackboneDisambiguates: a name shared by two backbones
// is unresolvable (ambiguous) without a filter and resolves to the WCVP
// concept with entry_backbone=wcvp.
func TestHandleMatch_EntryBackboneDisambiguates(t *testing.T) {
	db := seededRepo(t)
	seedDuplicateCorynephorusInBackbone(t, db)

	body := `{"names":[{"id":"1","verbatim":"Corynephorus canescens (L.) P.Beauv."}]}`
	rr := postMatch(t, db, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if mt := rawResults(t, rr)[0]["match_type"]; string(mt) != `"unresolvable"` {
		t.Fatalf("no filter: match_type = %s, want unresolvable (ambiguous across backbones)", mt)
	}

	rr = postMatch(t, db, `{"entry_backbone":"wcvp","names":[{"id":"1","verbatim":"Corynephorus canescens (L.) P.Beauv."}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	res := rawResults(t, rr)[0]
	if string(res["match_type"]) != `"exact_author"` {
		t.Errorf("entry_backbone=wcvp: match_type = %s, want exact_author", res["match_type"])
	}
	if string(res["concept_id"]) != `"`+corynephorusConceptID+`"` {
		t.Errorf("entry_backbone=wcvp: concept_id = %s, want %q", res["concept_id"], corynephorusConceptID)
	}
}

// TestHandleMatch_UnknownEntryFilters_Return400 pins that an un-ingested
// entry_backbone / entry_sec is a 400 INVALID_QUERY naming it.
func TestHandleMatch_UnknownEntryFilters_Return400(t *testing.T) {
	db := seededRepo(t)

	for _, tc := range []struct{ field, value string }{
		{"entry_backbone", "nope"},
		{"entry_sec", "nope"},
	} {
		body := `{"` + tc.field + `":"` + tc.value + `","names":[{"id":"1","verbatim":"Corynephorus canescens"}]}`
		rr := postMatch(t, db, body)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s=%s: status = %d, want 400 (body: %s)", tc.field, tc.value, rr.Code, rr.Body.String())
		}
		env := decodeJSON[errorEnvelope](t, rr.Body)
		if env.Error.Code != "INVALID_QUERY" {
			t.Errorf("%s: error.code = %q, want INVALID_QUERY", tc.field, env.Error.Code)
		}
		if !contains(env.Error.Message, tc.field) || !contains(env.Error.Message, tc.value) {
			t.Errorf("%s: message = %q, want it to name %q and %q", tc.field, env.Error.Message, tc.field, tc.value)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
