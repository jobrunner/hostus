package httperr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/hostus/internal/httperr"
)

func TestWriteEnvelope_NotFound(t *testing.T) {
	rr := httptest.NewRecorder()

	httperr.Write(rr, http.StatusNotFound, httperr.NotFound, "concept not found")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}

	const want = `{"error":{"code":"NOT_FOUND","message":"concept not found"}}` + "\n"
	if got := rr.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestWriteEnvelope_Unresolvable(t *testing.T) {
	rr := httptest.NewRecorder()

	httperr.Write(rr, http.StatusConflict, httperr.Unresolvable, "cannot resolve synonym chain")

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}

	const want = `{"error":{"code":"UNRESOLVABLE","message":"cannot resolve synonym chain"}}` + "\n"
	if got := rr.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestCodeConstants_Values(t *testing.T) {
	if httperr.NotFound != "NOT_FOUND" {
		t.Fatalf("NotFound = %q, want %q", httperr.NotFound, "NOT_FOUND")
	}
	if httperr.Unresolvable != "UNRESOLVABLE" {
		t.Fatalf("Unresolvable = %q, want %q", httperr.Unresolvable, "UNRESOLVABLE")
	}
}

func TestWriteEnvelope_JSONShape(t *testing.T) {
	rr := httptest.NewRecorder()
	httperr.Write(rr, http.StatusNotFound, httperr.NotFound, "x")

	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error.Code != "NOT_FOUND" || got.Error.Message != "x" {
		t.Fatalf("bad envelope: %+v", got)
	}
}
