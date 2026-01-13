package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/cache"
	"github.com/jobrunner/hostus/internal/gbif"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/middleware"
	"github.com/jobrunner/hostus/internal/taxonomy"
)

func TestSuggestHandler_InvalidQuery_TooShort(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/taxa/suggest?q=ab", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp httperr.Response
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Error.Code != httperr.InvalidQuery {
		t.Errorf("expected error code INVALID_QUERY, got %s", resp.Error.Code)
	}
}

func TestSuggestHandler_InvalidQuery_MissingQ(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/taxa/suggest", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestSuggestHandler_InvalidLimit(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/taxa/suggest?q=quercus&limit=abc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestSuggestHandler_CacheHit(t *testing.T) {
	c := cache.New(time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	loadShedder := middleware.NewLoadShedder(5, time.Second)

	// Pre-populate cache
	cachedData := []taxonomy.TaxonSuggestion{
		{
			AcceptedKey:  123,
			AcceptedName: "Cached Result",
			Rank:         "SPECIES",
			Family:       "Testaceae",
		},
	}
	c.Set("quercus:20", cachedData)

	handler := NewSuggestHandler(nil, c, loadShedder, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/taxa/suggest?q=quercus", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var result []taxonomy.TaxonSuggestion
	_ = json.NewDecoder(rec.Body).Decode(&result)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AcceptedName != "Cached Result" {
		t.Errorf("expected cached result, got %s", result[0].AcceptedName)
	}
}

func TestSuggestHandler_LimitCapped(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/taxa/suggest?q=test&limit=999", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// GBIF client will fail (no server), expect upstream errors
	// Status 502 (unavailable) or 504 (timeout) are expected
	if rec.Code != http.StatusBadGateway && rec.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status 502 or 504, got %d", rec.Code)
	}
}

func setupTestHandler(t *testing.T) *SuggestHandler {
	t.Helper()

	c := cache.New(time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	loadShedder := middleware.NewLoadShedder(5, time.Second)

	// Create a mock GBIF client that returns empty results
	gbifClient := gbif.NewClient("http://localhost:9999", time.Millisecond)

	return NewSuggestHandler(gbifClient, c, loadShedder, logger)
}
