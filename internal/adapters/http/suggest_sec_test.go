package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

func suggestRawResults(t *testing.T, db *sqlite.DB, query string) []map[string]json.RawMessage {
	t.Helper()
	r := httpx.NewRouter(httpx.Deps{Repo: db})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/suggest?"+query, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/suggest?%s: status = %d (body %s)", query, rr.Code, rr.Body.String())
	}
	var env struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decoding suggest: %v", err)
	}
	return env.Results
}

// TestSuggest_SecPresentForCDMHits: same-name CDM concepts carry a `sec`
// {id,title}, which is the only thing that tells them apart on the wire.
func TestSuggest_SecPresentForCDMHits(t *testing.T) {
	db := translateRepoDB(t)
	results := suggestRawResults(t, db, "q=Abies&limit=10")
	if len(results) == 0 {
		t.Fatal("no suggest results for a CDM name")
	}
	for _, r := range results {
		sec, present := r["sec"]
		if !present {
			t.Fatalf("CDM suggest hit %s lacks a sec field", r["concept_id"])
		}
		var got struct{ ID, Title string }
		if err := json.Unmarshal(sec, &got); err != nil {
			t.Fatalf("sec unmarshal: %v", err)
		}
		if got.ID == "" || got.Title == "" {
			t.Errorf("sec = %+v, want both id and title populated", got)
		}
	}
}

// TestSuggest_SecAbsentForWCVPHits: WCVP concepts (no sec. reference) omit the
// field, so the SP1/SP2 shape is unchanged.
func TestSuggest_SecAbsentForWCVPHits(t *testing.T) {
	db := seededRepo(t)
	results := suggestRawResults(t, db, "q=Coryn&limit=10")
	if len(results) == 0 {
		t.Fatal("no suggest results for a WCVP name")
	}
	for _, r := range results {
		if _, present := r["sec"]; present {
			t.Errorf("WCVP suggest hit %s carries a sec field; want it absent", r["concept_id"])
		}
	}
}
