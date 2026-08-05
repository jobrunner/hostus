package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// seedFloraVegHTTP attaches a floraveg name space + entries to an already
// seeded repo, straight through the ingest tx — the same write path the real
// ingest uses. Kept in the http test package so the wire-shape tests can build
// a target space to ask for.
func seedFloraVegHTTP(t *testing.T, db *sqlite.DB, entries map[string][]domain.NameSpaceEntry) {
	t.Helper()
	tx, err := db.BeginTraitIngest(context.Background())
	if err != nil {
		t.Fatalf("BeginTraitIngest: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", ManifestSHA: "x",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: %v", err)
	}
	for conceptID, es := range entries {
		for _, e := range es {
			if err := tx.AddNameSpaceEntry(conceptID, e); err != nil {
				t.Fatalf("AddNameSpaceEntry(%s): %v", conceptID, err)
			}
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// postMatch posts body to /v1/match and returns the recorder.
func postMatch(t *testing.T, db *sqlite.DB, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httpx.NewRouter(httpx.Deps{Repo: db})
	rr := httptest.NewRecorder()
	rr.Body = new(bytes.Buffer)
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/match", bytes.NewBufferString(body)))
	return rr
}

// rawResults decodes the results array as raw key/value maps so a test can
// assert on a field's PRESENCE, not just its value — the UC4 fields are opt-in
// and "absent" is a meaningful state distinct from "empty".
func rawResults(t *testing.T, rr *httptest.ResponseRecorder) []map[string]json.RawMessage {
	t.Helper()
	var env struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decoding results: %v", err)
	}
	return env.Results
}

// TestHandleMatch_NoTargetSpace_OmitsUC4Fields pins the opt-in contract: with
// no target_space the response carries NONE of the three UC4 fields, so UC3/UC6
// (which share this endpoint) see a byte-for-byte unchanged shape.
func TestHandleMatch_NoTargetSpace_OmitsUC4Fields(t *testing.T) {
	db := seededRepo(t)
	rr := postMatch(t, db, `{"names":[{"id":"1","verbatim":"Corynephorus canescens"}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	res := rawResults(t, rr)[0]
	for _, k := range []string{"target_space_name", "aggregate_policy", "esy_diagnostic_relevance"} {
		if _, present := res[k]; present {
			t.Errorf("result carries %q without a target_space; want it absent", k)
		}
	}
}

// TestHandleMatch_TargetSpaceFloraVeg_PlainSpecies pins the wire shape for a
// plain species asked in a target space: esy_diagnostic_relevance is present
// and equals the documented sentinel, aggregate_policy is ABSENT (a plain
// species has none), and the ESy spelling is handed back.
func TestHandleMatch_TargetSpaceFloraVeg_PlainSpecies(t *testing.T) {
	db := seededRepo(t)
	seedFloraVegHTTP(t, db, map[string][]domain.NameSpaceEntry{
		corynephorusConceptID: {
			{Space: "floraveg", ExtID: "9001", Name: "Corynephorus canescens", Aggregate: false},
		},
	})
	rr := postMatch(t, db, `{"target_space":"floraveg","names":[{"id":"1","verbatim":"Corynephorus canescens"}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	res := rawResults(t, rr)[0]

	esy, present := res["esy_diagnostic_relevance"]
	if !present {
		t.Fatal("esy_diagnostic_relevance absent; with a target_space it must be conspicuously present")
	}
	if string(esy) != `"not_determinable"` {
		t.Errorf("esy_diagnostic_relevance = %s, want %q", esy, "not_determinable")
	}
	if _, present := res["aggregate_policy"]; present {
		t.Errorf("aggregate_policy present for a plain species; want it absent")
	}
	if name, present := res["target_space_name"]; !present || string(name) != `"Corynephorus canescens"` {
		t.Errorf("target_space_name = %s (present=%v), want %q", name, present, "Corynephorus canescens")
	}
}

// TestHandleMatch_UnknownTargetSpace_Returns400 pins that an un-ingested target
// space is a 400 INVALID_QUERY that names the offending space, not a silent
// no-op.
func TestHandleMatch_UnknownTargetSpace_Returns400(t *testing.T) {
	db := seededRepo(t)
	rr := postMatch(t, db, `{"target_space":"germansl","names":[{"id":"1","verbatim":"Corynephorus canescens"}]}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want INVALID_QUERY", got.Error.Code)
	}
	if !bytes.Contains([]byte(got.Error.Message), []byte("germansl")) {
		t.Errorf("error.message = %q, want it to name the unknown space %q", got.Error.Message, "germansl")
	}
}
