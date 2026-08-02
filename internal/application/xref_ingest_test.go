package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/xref"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// xrefRowSource adapts a *xref.Dataset (T1/T2's reader output) into
// application.XrefRowSource, the same boundary-respecting bridge
// traitsRowSource uses for trait vocabularies — application never imports
// internal/adapters/xref directly (depguard).
type xrefRowSource struct{ ds *xref.Dataset }

func (s xrefRowSource) Rows() []application.XrefRow {
	out := make([]application.XrefRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.XrefRow{
			JoinAuthority: r.JoinAuthority,
			JoinID:        r.JoinID,
			Authority:     r.Authority,
			ExtID:         r.ExtID,
		})
	}
	return out
}

func loadWikidataFixture(t *testing.T) xrefRowSource {
	t.Helper()
	ds, err := xref.Read("../adapters/xref/testdata/wikidata-sample.csv")
	if err != nil {
		t.Fatalf("xref.Read(wikidata-sample.csv): unexpected error: %v", err)
	}
	return xrefRowSource{ds: ds}
}

var wikidataMeta = domain.XrefSourceMeta{
	ID:             "wikidata",
	Version:        "2026-08-02",
	License:        "CC0",
	SourceURL:      "https://query.wikidata.org/sparql",
	Redistribution: domain.RedistributionAllowed,
}

func TestIngestXrefs_MatchedRowLandsAsXref(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref(powo, 396681-1): unexpected error: %v", err)
	}

	report, err := application.IngestXrefs(ctx, repo, loadWikidataFixture(t), wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	if report.Source != "wikidata" {
		t.Errorf("report.Source = %q, want %q", report.Source, "wikidata")
	}

	got, err := repo.ConceptByXref(ctx, "inat", "160927")
	if err != nil {
		t.Fatalf("ConceptByXref(inat, 160927): unexpected error: %v", err)
	}
	if got.ID != concept.ID {
		t.Errorf("ConceptByXref(inat, 160927).ID = %q, want %q (same concept as powo 396681-1)", got.ID, concept.ID)
	}
}

func TestIngestXrefs_CountsSumToRows(t *testing.T) {
	repo := seededMatchRepo(t)
	report, err := application.IngestXrefs(context.Background(), repo, loadWikidataFixture(t), wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	if got, want := report.Rows, 20; got != want {
		t.Fatalf("report.Rows = %d, want %d", got, want)
	}
	if sum := report.Matched + report.Unmatched + report.Conflicting; sum != report.Rows {
		t.Errorf("Matched(%d)+Unmatched(%d)+Conflicting(%d) = %d, want Rows = %d",
			report.Matched, report.Unmatched, report.Conflicting, sum, report.Rows)
	}
}

func TestIngestXrefs_UnmatchedJoinIDIsCountedAndSampledButWritesNothing(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	report, err := application.IngestXrefs(ctx, repo, loadWikidataFixture(t), wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	if report.Unmatched < 1 {
		t.Fatalf("report.Unmatched = %d, want at least 1 (join_id 999999-9 does not exist)", report.Unmatched)
	}
	if !containsString(report.UnmatchedSample, "powo:999999-9") {
		t.Errorf("UnmatchedSample = %v, want it to contain %q", report.UnmatchedSample, "powo:999999-9")
	}

	if _, err := repo.ConceptByXref(ctx, "inat", "900001"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConceptByXref(inat, 900001) = %v, want %v (unmatched row must write nothing)", err, domain.ErrNotFound)
	}
}

func TestIngestXrefs_ConflictingExtIDIsCountedSampledAndSkipped(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	report, err := application.IngestXrefs(ctx, repo, loadWikidataFixture(t), wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	// The fixture claims inat:900002 from BOTH join_id 396681-1 and
	// join_id 226649-1 — two distinct concepts. Both of those two rows
	// must be counted as Conflicting, not Matched.
	if got, want := report.Conflicting, 2; got != want {
		t.Fatalf("report.Conflicting = %d, want %d", got, want)
	}
	if !containsString(report.ConflictSample, "inat:900002") {
		t.Errorf("ConflictSample = %v, want it to contain %q", report.ConflictSample, "inat:900002")
	}

	if _, err := repo.ConceptByXref(ctx, "inat", "900002"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConceptByXref(inat, 900002) = %v, want %v (a conflicting key must never be written, for either claimant)", err, domain.ErrNotFound)
	}
}

func TestIngestXrefs_MultipleIDsSameAuthorityOneConceptAreBothWrittenNotAConflict(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	concept, err := repo.ConceptByXref(ctx, "powo", "396681-1")
	if err != nil {
		t.Fatalf("ConceptByXref(powo, 396681-1): unexpected error: %v", err)
	}

	report, err := application.IngestXrefs(ctx, repo, loadWikidataFixture(t), wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}

	// Two DIFFERENT Wikidata items (Q159953 and the fixture's synthetic
	// Q900003) both carry join_id 396681-1 — both must resolve and both
	// must be written, since they don't share an (authority, ext_id) key.
	for _, qid := range []string{"Q159953", "Q900003"} {
		got, err := repo.ConceptByXref(ctx, "wikidata", qid)
		if err != nil {
			t.Fatalf("ConceptByXref(wikidata, %s): unexpected error: %v", qid, err)
		}
		if got.ID != concept.ID {
			t.Errorf("ConceptByXref(wikidata, %s).ID = %q, want %q", qid, got.ID, concept.ID)
		}
	}

	if got := report.MultiPerAuthority["wikidata"]; got < 1 {
		t.Errorf(`MultiPerAuthority["wikidata"] = %d, want at least 1`, got)
	}
	if !containsString(report.MultiSample, "wikidata:"+concept.ID) {
		t.Errorf("MultiSample = %v, want it to contain %q", report.MultiSample, "wikidata:"+concept.ID)
	}
}

func TestIngestXrefs_PerAuthorityCountsDistinctConcepts(t *testing.T) {
	repo := seededMatchRepo(t)
	report, err := application.IngestXrefs(context.Background(), repo, loadWikidataFixture(t), wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	// inat: three distinct fixture concepts (396681-1, 226649-1, 331174-2)
	// each carry one real, non-conflicting inat row.
	if got, want := report.PerAuthority["inat"], 3; got != want {
		t.Errorf(`PerAuthority["inat"] = %d, want %d`, got, want)
	}
	// wikidata: same three concepts reachable, even though one of them
	// (396681-1) received TWO wikidata ids — PerAuthority counts CONCEPTS,
	// not rows.
	if got, want := report.PerAuthority["wikidata"], 3; got != want {
		t.Errorf(`PerAuthority["wikidata"] = %d, want %d`, got, want)
	}
}

func TestIngestXrefs_NeverAttachesToASynonymOnlyRow(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	// 77271170-1 is a synonym's powo id in the WCVP fixture (see
	// TestIngest's own assertion that ConceptByXref never resolves it);
	// synonyms never get their own xref row to begin with, so this join_id
	// can only ever be Unmatched — the ID-based join has no way to attach
	// an xref to a synonym-only row.
	src := xrefRowSource{ds: &xref.Dataset{Rows: []xref.Row{
		{JoinAuthority: "powo", JoinID: "77271170-1", Authority: "inat", ExtID: "1"},
	}}}

	report, err := application.IngestXrefs(ctx, repo, src, wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	if got, want := report.Unmatched, 1; got != want {
		t.Fatalf("report.Unmatched = %d, want %d (synonym powo id must not resolve)", got, want)
	}
	if _, err := repo.ConceptByXref(ctx, "inat", "1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConceptByXref(inat, 1) = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestIngestXrefs_NoMatchesLeavesPerAuthorityAndMultiNil(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := xrefRowSource{ds: &xref.Dataset{Rows: []xref.Row{
		{JoinAuthority: "powo", JoinID: "999999-9", Authority: "inat", ExtID: "900001"},
	}}}

	report, err := application.IngestXrefs(ctx, repo, src, wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	if got, want := report.Matched, 0; got != want {
		t.Fatalf("report.Matched = %d, want %d", got, want)
	}
	if report.PerAuthority != nil {
		t.Errorf("report.PerAuthority = %v, want nil when nothing matched", report.PerAuthority)
	}
	if report.MultiPerAuthority != nil {
		t.Errorf("report.MultiPerAuthority = %v, want nil when nothing matched", report.MultiPerAuthority)
	}
}

func TestIngestXrefs_MatchesWithNoRepeatedAuthorityLeaveMultiPerAuthorityNil(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	// A single matched row: exactly one ext_id for one authority on one
	// concept, so the case (b) "multiple ids, same authority" phenomenon
	// cannot occur.
	src := xrefRowSource{ds: &xref.Dataset{Rows: []xref.Row{
		{JoinAuthority: "powo", JoinID: "396681-1", Authority: "inat", ExtID: "160927"},
	}}}

	report, err := application.IngestXrefs(ctx, repo, src, wikidataMeta)
	if err != nil {
		t.Fatalf("IngestXrefs: unexpected error: %v", err)
	}
	if got, want := report.Matched, 1; got != want {
		t.Fatalf("report.Matched = %d, want %d", got, want)
	}
	if report.MultiPerAuthority != nil {
		t.Errorf("report.MultiPerAuthority = %v, want nil when no concept received more than one id per authority", report.MultiPerAuthority)
	}
	if got, want := report.PerAuthority["inat"], 1; got != want {
		t.Errorf(`report.PerAuthority["inat"] = %d, want %d`, got, want)
	}
}

func containsString(haystack []string, want string) bool {
	for _, s := range haystack {
		if s == want {
			return true
		}
	}
	return false
}
