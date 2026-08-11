package httpx_test

import (
	"net/http"
	"testing"
)

// TestTranslate_EntrySecResolvesAmbiguousVerbatim: "Abies alba" lives in three
// sec spaces, so a bare verbatim entry is unresolvable (422); entry_sec picks
// one source concept, which then translates over its congruent edge.
func TestTranslate_EntrySecResolvesAmbiguousVerbatim(t *testing.T) {
	db := translateRepoDB(t, translateRelation("a", "b", "Congruent to"))

	// Without a filter: ambiguous across sec spaces -> 422 UNRESOLVABLE.
	rr := postTranslate(t, db, `{"verbatim":"Abies alba","target_space":"`+tSecB+`"}`)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no filter: status = %d, want 422 (body %s)", rr.Code, rr.Body.String())
	}

	// With entry_sec: resolves to concept a, translates to b.
	rr = postTranslate(t, db, `{"verbatim":"Abies alba","entry_sec":"`+tSecA+`","target_space":"`+tSecB+`"}`)
	got := decodeTranslate(t, rr) // asserts 200
	entry, _ := got["entry"].(map[string]any)
	if entry["mode"] != "verbatim" {
		t.Errorf("entry.mode = %v, want verbatim", entry["mode"])
	}
	_ = firstCandidate(t, got) // exactly one candidate (the b concept)
}

// TestTranslate_UnknownEntryFilters_Return400 pins the 400 INVALID_QUERY for an
// un-ingested entry_backbone / entry_sec on the verbatim path.
func TestTranslate_UnknownEntryFilters_Return400(t *testing.T) {
	db := translateRepoDB(t)

	for _, tc := range []struct{ field, value string }{
		{"entry_backbone", "nope"},
		{"entry_sec", "nope"},
	} {
		body := `{"verbatim":"Abies alba","` + tc.field + `":"` + tc.value + `","target_space":"` + tSecB + `"}`
		rr := postTranslate(t, db, body)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s=%s: status = %d, want 400 (body %s)", tc.field, tc.value, rr.Code, rr.Body.String())
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
