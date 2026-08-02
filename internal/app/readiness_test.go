package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
)

// corynephorusConceptID is the deterministic id application.Ingest assigns
// the WCVP fixture's Corynephorus canescens accepted concept
// (backboneID+":concept:"+taxonid, taxonid 405825 — see
// internal/adapters/http/taxa_test.go, which pins the same id).
const corynephorusConceptID = "wcvp:concept:405825"

// TestNew_ReadinessGatedOnDB drives the full composition root end to end:
// app.New opens the configured (initially empty) SQLite database and wires
// its repository into the HTTP router, so /health/ready must report 503
// until app.Ingest (the same entry point "hostus ingest" uses) has written
// at least one backbone_version row into that exact database file — after
// which readiness flips to 200 and the ingested data is servable over
// /v1/concept without ever rebuilding the App.
func TestNew_ReadinessGatedOnDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")

	cfg := testConfig()
	cfg.SQLite.Path = dbPath

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	rr := httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready before ingest: got %d, want 503 (body: %s)", rr.Code, rr.Body.String())
	}

	if _, _, _, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}

	rr = httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ready after ingest: got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("concept lookup: got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
}

// TestNew_NoSQLitePathConfigured_StaysNotReady confirms serve still starts
// (New must not error) when SQLite.Path is left empty, but readiness stays
// permanently 503 since there is no database to become ready.
func TestNew_NoSQLitePathConfigured_StaysNotReady(t *testing.T) {
	cfg := testConfig()
	cfg.SQLite.Path = ""

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	rr := httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (no sqlite.path configured)", rr.Code)
	}

	rr = httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("liveness got %d, want 200 even without a DB", rr.Code)
	}
}

// TestNew_UnopenableSQLitePath_StaysNotReadyButAppStillBuilds confirms a
// path pointing at a directory instead of a file (an unopenable database)
// does not fail New — the composition root must degrade to "not ready"
// rather than refusing to serve at all.
func TestNew_UnopenableSQLitePath_StaysNotReadyButAppStillBuilds(t *testing.T) {
	dir := t.TempDir() // a directory can never be opened as a sqlite file

	cfg := testConfig()
	cfg.SQLite.Path = dir

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error, want New to degrade gracefully: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	rr := httptest.NewRecorder()
	a.Router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (unopenable sqlite path)", rr.Code)
	}
}
