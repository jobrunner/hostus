package httpx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

type stubAreaRepo struct {
	output.Repository
	areas []domain.Area
	err   error
}

func (s stubAreaRepo) Areas(context.Context) ([]domain.Area, error) {
	return s.areas, s.err
}

type areaListResponse struct {
	Areas []struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Scheme string `json:"scheme"`
	} `json:"areas"`
}

func TestHandleAreas_ListsAreasWithNames(t *testing.T) {
	repo := stubAreaRepo{areas: []domain.Area{
		{Scheme: "wgsrpd_l3", Code: "FRA", Name: "France"},
		{Scheme: "wgsrpd_l3", Code: "GER", Name: "Germany"},
	}}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/areas", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[areaListResponse](t, rr.Body)
	if len(got.Areas) != 2 {
		t.Fatalf("got %d areas, want 2 (body: %s)", len(got.Areas), rr.Body.String())
	}
	if got.Areas[0].Code != "FRA" || got.Areas[0].Name != "France" || got.Areas[0].Scheme != "wgsrpd_l3" {
		t.Errorf("Areas[0] = %+v, want {FRA, France, wgsrpd_l3}", got.Areas[0])
	}
	if got.Areas[1].Code != "GER" || got.Areas[1].Name != "Germany" {
		t.Errorf("Areas[1] = %+v, want {GER, Germany}", got.Areas[1])
	}
}

func TestHandleAreas_Empty_ReturnsEmptyArrayNotNull(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubAreaRepo{areas: nil}})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/areas", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, `"areas":[]`) {
		t.Errorf("body = %s, want an empty JSON array, not null", body)
	}
}

func TestHandleAreas_RepoError_Returns500(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{Repo: stubAreaRepo{err: errors.New("boom")}})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/areas", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "INTERNAL_ERROR") {
		t.Errorf("body = %s, want an INTERNAL_ERROR envelope", body)
	}
}
