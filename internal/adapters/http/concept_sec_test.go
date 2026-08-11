package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

func getConceptRaw(t *testing.T, db *sqlite.DB, id string) map[string]json.RawMessage {
	t.Helper()
	r := httpx.NewRouter(httpx.Deps{Repo: db})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+id, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/concept/%s: status = %d (body %s)", id, rr.Code, rr.Body.String())
	}
	var m map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decoding concept: %v", err)
	}
	return m
}

// TestConcept_SecPresentForCDMConcept: a sec-bearing (CDM) concept carries a
// `sec` {id,title}, so two same-name concepts are distinguishable.
func TestConcept_SecPresentForCDMConcept(t *testing.T) {
	db := translateRepoDB(t)
	m := getConceptRaw(t, db, tConceptA)

	sec, present := m["sec"]
	if !present {
		t.Fatal("CDM concept: sec absent, want {id,title}")
	}
	var got struct{ ID, Title string }
	if err := json.Unmarshal(sec, &got); err != nil {
		t.Fatalf("sec unmarshal: %v", err)
	}
	if got.ID != tSecA || got.Title != "Rothmaler, Exkursionsflora, 8. Aufl." {
		t.Errorf("sec = %+v, want id=%s title=Rothmaler…", got, tSecA)
	}
}

// TestConcept_SecAbsentForWCVPConcept: a concept with no sec. reference (WCVP)
// omits the field — absence is unchanged from the SP1 shape.
func TestConcept_SecAbsentForWCVPConcept(t *testing.T) {
	db := seededRepo(t)
	m := getConceptRaw(t, db, corynephorusConceptID)
	if _, present := m["sec"]; present {
		t.Error("WCVP concept carries a sec field; want it absent")
	}
}
