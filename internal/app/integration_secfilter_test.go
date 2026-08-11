//go:build integration

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedFestucaOvinaInTwoSecs ingests, through the real CDM path, two "Festuca
// ovina" concepts under two sec spaces plus a congruent edge between them —
// so the name is shared with the WCVP fixture's own Festuca ovina (three
// concepts total) and translate has a relation to follow.
func seedFestucaOvinaInTwoSecs(t *testing.T, a *app.App) {
	t.Helper()
	yes := true
	concepts := []application.CDMConceptRow{
		{ConceptUUID: "fo-roth", ScientificName: "Festuca ovina", Authorship: "L.", Rank: "Species", Status: "Accepted", SecUUID: "sec-roth", SecTitle: "Rothmaler, Exkursionsflora, 8. Aufl."},
		{ConceptUUID: "fo-wh", ScientificName: "Festuca ovina", Authorship: "L.", Rank: "Species", Status: "Accepted", SecUUID: "sec-wh", SecTitle: "Wisskirchen & Haeupler 1998: Standardliste"},
	}
	rels := []application.CDMRelationRow{
		{FromUUID: "fo-roth", ToUUID: "fo-wh", RelationType: "Congruent to", IsConceptRelation: &yes, RelationshipUUID: "r1"},
	}
	if _, err := application.IngestCDM(context.Background(), a.Repo, concepts, rels,
		domain.BackboneVersion{ID: "cdm", Version: "2026-08-02", Redistribution: domain.RedistributionUnknown}); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
}

func postJSON(t *testing.T, client *http.Client, url, body string) map[string]any {
	t.Helper()
	resp, err := client.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: status = %d", url, resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decoding %s: %v", url, err)
	}
	return m
}

func firstResult(t *testing.T, got map[string]any) map[string]any {
	t.Helper()
	rs, ok := got["results"].([]any)
	if !ok || len(rs) == 0 {
		t.Fatalf("results = %v, want at least one", got["results"])
	}
	return rs[0].(map[string]any)
}

// TestIntegration_SecFilterAndSecOutput drives the SP5 lever end to end over
// real HTTP: a name shared across WCVP + two CDM sec spaces is ambiguous
// without a filter, resolves to the WCVP concept with entry_backbone=wcvp and
// to one CDM concept with entry_sec; /v1/concept and /v1/suggest carry `sec`
// for the CDM concept and not the WCVP one; and /v1/translate verbatim with
// entry_sec translates over the seeded relation.
func TestIntegration_SecFilterAndSecOutput(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	ctx := context.Background()
	if _, err := app.Ingest(ctx, "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: %v", err)
	}
	cfg := testConfig()
	cfg.SQLite.Path = dbPath
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	seedFestucaOvinaInTwoSecs(t, a)

	ts := httptest.NewServer(a.Router)
	defer ts.Close()
	client := ts.Client()

	// (a) no filter -> ambiguous across WCVP + 2 CDM -> unresolvable.
	amb := firstResult(t, postJSON(t, client, ts.URL+"/v1/match",
		`{"names":[{"id":"1","verbatim":"Festuca ovina L."}]}`))
	if amb["match_type"] != "unresolvable" {
		t.Errorf("no filter: match_type = %v, want unresolvable", amb["match_type"])
	}

	// (b) entry_backbone=wcvp -> the WCVP concept, exact.
	wc := firstResult(t, postJSON(t, client, ts.URL+"/v1/match",
		`{"entry_backbone":"wcvp","names":[{"id":"1","verbatim":"Festuca ovina L."}]}`))
	if wc["concept_id"] != festucaOvinaConceptID {
		t.Errorf("entry_backbone=wcvp: concept_id = %v, want %s", wc["concept_id"], festucaOvinaConceptID)
	}

	// (c) entry_sec=sec-roth -> exactly the one CDM concept in that space.
	sr := firstResult(t, postJSON(t, client, ts.URL+"/v1/match",
		`{"entry_sec":"sec-roth","names":[{"id":"1","verbatim":"Festuca ovina L."}]}`))
	if sr["concept_id"] != "cdm:concept:fo-roth" {
		t.Errorf("entry_sec=sec-roth: concept_id = %v, want cdm:concept:fo-roth", sr["concept_id"])
	}

	// (d) /v1/concept sec output: present for CDM, absent for WCVP.
	assertConceptSec(t, client, ts.URL, "cdm:concept:fo-roth", "sec-roth")
	assertConceptNoSec(t, client, ts.URL, festucaOvinaConceptID)

	// (e) /v1/translate verbatim + entry_sec -> translated over the relation.
	tr := postJSON(t, client, ts.URL+"/v1/translate",
		`{"verbatim":"Festuca ovina L.","entry_sec":"sec-roth","target_space":"sec-wh"}`)
	cands, ok := tr["candidates"].([]any)
	if !ok || len(cands) != 1 {
		t.Fatalf("translate candidates = %v, want exactly one (the sec-wh concept)", tr["candidates"])
	}
}

func assertConceptSec(t *testing.T, client *http.Client, baseURL, id, wantSecID string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/concept/" + id)
	if err != nil {
		t.Fatalf("GET concept %s: %v", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	sec, ok := m["sec"].(map[string]any)
	if !ok {
		t.Fatalf("concept %s: sec absent, want id=%s", id, wantSecID)
	}
	if sec["id"] != wantSecID {
		t.Errorf("concept %s: sec.id = %v, want %s", id, sec["id"], wantSecID)
	}
}

func assertConceptNoSec(t *testing.T, client *http.Client, baseURL, id string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/concept/" + id)
	if err != nil {
		t.Fatalf("GET concept %s: %v", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	if _, present := m["sec"]; present {
		t.Errorf("WCVP concept %s carries sec, want absent", id)
	}
}
