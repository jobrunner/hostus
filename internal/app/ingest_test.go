package app_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, for this file's direct concept_agreement query only

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestIngest_ReportsXrefSources drives the REAL composition root ("hostus
// ingest"'s entry point) against a manifest that pins an xref source, on a
// REAL on-disk SQLite file. That combination is what makes this more than a
// happy-path assertion: the sqlite adapter runs with SetMaxOpenConns(1), so
// any repository read issued while the xref ingest transaction is open would
// deadlock here rather than fail — see application.IngestXrefs' two-phase
// contract.
func TestIngest_ReportsXrefSources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.Xrefs) != 1 {
		t.Fatalf("len(reports.Xrefs) = %d, want 1 (the manifest pins one xref source)", len(reports.Xrefs))
	}

	xr := reports.Xrefs[0]
	if xr.Source != "wikidata" {
		t.Errorf("reports.Xrefs[0].Source = %q, want %q", xr.Source, "wikidata")
	}
	if xr.Redistribution != string(domain.RedistributionAllowed) {
		t.Errorf("reports.Xrefs[0].Redistribution = %q, want %q", xr.Redistribution, domain.RedistributionAllowed)
	}
	if xr.Rows == 0 {
		t.Error("reports.Xrefs[0].Rows = 0, want the fixture's rows")
	}
	if xr.Matched == 0 {
		t.Error("reports.Xrefs[0].Matched = 0, want the powo-resolvable fixture rows to have been written")
	}
	if got := xr.Matched + xr.Unmatched + xr.Conflicting; got != xr.Rows {
		t.Errorf("Matched+Unmatched+Conflicting = %d, want %d (= Rows)", got, xr.Rows)
	}
	if xr.PerAuthority["inat"] == 0 {
		t.Error(`reports.Xrefs[0].PerAuthority["inat"] = 0, want the fixture's inat rows to have resolved`)
	}
}

func TestIngest_XrefSourceReadErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset-bad-xref-path.yaml", dbPath)
	if err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable xref CSV path, got nil")
	}
	if len(reports.Xrefs) != 0 {
		t.Errorf("reports.Xrefs = %v, want empty (the failing source must not appear in the report)", reports.Xrefs)
	}
}

func TestIngest_ManifestParseErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	if _, err := app.Ingest(context.Background(), "testdata/does-not-exist.yaml", dbPath); err == nil {
		t.Fatal("app.Ingest: expected an error for a missing manifest, got nil")
	}
}

func TestIngest_OpenDatabaseErrorPropagates(t *testing.T) {
	// A directory is not a usable SQLite file path.
	if _, err := app.Ingest(context.Background(), "testdata/dataset.yaml", t.TempDir()); err == nil {
		t.Fatal("app.Ingest: expected an error for an unopenable database path, got nil")
	}
}

func TestIngest_BackboneIngestErrorPropagates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset-bad-backbone-path.yaml", dbPath)
	if err == nil {
		t.Fatal("app.Ingest: expected an error for an unreadable backbone path, got nil")
	}
	if reports.Xrefs != nil {
		t.Errorf("reports.Xrefs = %v, want nil (xref ingest must not run after the backbone failed)", reports.Xrefs)
	}
}

// TestIngest_ReportsNameSpaces drives the REAL composition root against a
// manifest that pins the FloraVeg name space, on a REAL on-disk SQLite file
// — the same combination that makes the xref test above meaningful:
// application.IngestNameSpace must never read the repository while its
// ingest transaction is open, and with SetMaxOpenConns(1) a violation
// DEADLOCKS here rather than failing.
func TestIngest_ReportsNameSpaces(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.NameSpaces) != 1 {
		t.Fatalf("len(reports.NameSpaces) = %d, want 1 (the manifest pins one name space)", len(reports.NameSpaces))
	}

	ns := reports.NameSpaces[0]
	if ns.Space != "floraveg" {
		t.Errorf("reports.NameSpaces[0].Space = %q, want %q", ns.Space, "floraveg")
	}
	if ns.Rows != 5 {
		t.Errorf("reports.NameSpaces[0].Rows = %d, want 5 (the fixture's rows)", ns.Rows)
	}
	if ns.Matched != 3 || ns.Unmatched != 2 {
		t.Errorf("reports.NameSpaces[0] matched/unmatched = %d/%d, want 3/2", ns.Matched, ns.Unmatched)
	}
	if ns.Aggregates != 3 || ns.AggregatesMatched != 2 {
		t.Errorf("reports.NameSpaces[0] aggregates = %d of %d, want 2 of 3", ns.AggregatesMatched, ns.Aggregates)
	}
	if ns.Concepts != 1 {
		t.Errorf("reports.NameSpaces[0].Concepts = %d, want 1", ns.Concepts)
	}
	if ns.ReaderErrors != 0 {
		t.Errorf("reports.NameSpaces[0].ReaderErrors = %d, want 0", ns.ReaderErrors)
	}
	if ns.Redistribution != string(domain.RedistributionUnknown) {
		t.Errorf("reports.NameSpaces[0].Redistribution = %q, want %q — the gate depends on this reaching name_space", ns.Redistribution, domain.RedistributionUnknown)
	}
	if len(ns.UnmatchedSample) == 0 {
		t.Error("reports.NameSpaces[0].UnmatchedSample is empty, want the lossy crosswalk to name the names it dropped")
	}
}

// TestIngest_WiresFallBAndAgreementIntoRealCompositionRoot is the final-
// review Critical-1 regression test: before this fix, app.Ingest()'s
// NameSpaces loop only ever called ingestNameSpace (Fall A) — the Fall-B
// bridge (ingestNativeSpace) and application.ComputeConceptAgreement had NO
// production caller at all, so a real "hostus ingest" run left
// concept_aggregate and concept_agreement permanently empty regardless of
// what the manifest pinned.
//
// The fixture manifest (testdata/dataset-agreement.yaml) pins the SAME WCVP
// backbone as dataset.yaml plus two synthetic name spaces, eurosl and
// germansl, each with one genus row, one SPECIES_AGGREGATE row ("Festuca
// ovina agg.") and one Species row ("Festuca ovina", which crosswalks by
// name onto the WCVP fixture's real Festuca ovina concept,
// wcvp:concept:415853) under the SAME aggregate name in both spaces — the
// minimal shape that exercises Fall B's own-concept write, Task 6's
// aggregate-member wiring, AND Task 7's agreement comparison end to end.
//
// It proves the fix two ways: (1) via app.Ingest's own Reports (the
// production return value), and (2) via a direct SQL query against
// concept_agreement — the exact table the finding named as staying
// permanently empty.
func TestIngest_WiresFallBAndAgreementIntoRealCompositionRoot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	reports, err := app.Ingest(context.Background(), "testdata/dataset-agreement.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	t.Run("NativeSpaces", func(t *testing.T) {
		assertNativeSpacesReport(t, reports.NativeSpaces)
	})
	t.Run("ConceptAgreementReport", func(t *testing.T) {
		assertConceptAgreementReport(t, reports.ConceptAgreement)
	})
	t.Run("RepositoryAndRawSQL", func(t *testing.T) {
		assertConceptAgreementPersisted(t, dbPath)
	})
}

// assertNativeSpacesReport checks Fall B's own report entries — see
// TestIngest_WiresFallBAndAgreementIntoRealCompositionRoot's doc comment for
// the fixture shape.
func assertNativeSpacesReport(t *testing.T, nativeSpaces []application.NativeSpaceIngestReport) {
	t.Helper()
	if len(nativeSpaces) != 2 {
		t.Fatalf("len(reports.NativeSpaces) = %d, want 2 (eurosl and germansl both run Fall B)", len(nativeSpaces))
	}
	for _, nsr := range nativeSpaces {
		// Genus + SPECIES_AGGREGATE both qualify at minRank=domain.RankRoot;
		// the Species row is Fall A's territory and must NOT be written here.
		if nsr.Written != 2 {
			t.Errorf("reports.NativeSpaces[%q].Written = %d, want 2 (genus + aggregate)", nsr.Space, nsr.Written)
		}
		// nativeMemberLinks (ingest.go) links EVERY ParentID edge without a
		// rank filter, so the genus row is ALSO handed a memberLinks entry
		// (genus -> aggregate's source id) here (minRank=RankRoot) exactly
		// like the aggregate row is (aggregate -> species's source id). But
		// linkAggregateMembers (nativespace_ingest.go) only ever writes a
		// concept_aggregate edge for a row whose OWN rank is a genuine
		// collective/aggregate rank (isCollectiveRank) — GENUS never
		// qualifies, so its memberLinks entry resolves to nothing written.
		// Only the aggregate row's edge (to wcvp:concept:415853) counts — this
		// is the final-review residual-finding fix: previously GENUS also
		// wrote a concept_aggregate edge here, contaminating the table with a
		// second, wrong aggregating-side candidate for the same member.
		if nsr.MembersLinked != 1 {
			t.Errorf("reports.NativeSpaces[%q].MembersLinked = %d, want 1 (only the aggregate->species edge; GENUS is never the aggregating side)", nsr.Space, nsr.MembersLinked)
		}
	}
}

// assertConceptAgreementReport checks Task 7's agreement comparison found
// the one name-matched eurosl/germansl aggregate pair the fixture sets up.
func assertConceptAgreementReport(t *testing.T, report application.ConceptAgreementReport) {
	t.Helper()
	if len(report.Pairs) != 1 {
		t.Fatalf("len(reports.ConceptAgreement.Pairs) = %d, want 1 (one name-matched eurosl/germansl aggregate pair)", len(report.Pairs))
	}
	pair := report.Pairs[0]
	if pair.EuroslConceptID != "eurosl:concept:e-agg1" || pair.GermanslConceptID != "germansl:concept:g-agg1" {
		t.Errorf("ConceptAgreement.Pairs[0] = %+v, want eurosl/germansl concept ids e-agg1/g-agg1", pair)
	}
	if pair.Agreement != domain.AgreementIdentical {
		t.Errorf("ConceptAgreement.Pairs[0].Agreement = %q, want %q (both sides resolve the SAME WCVP member)", pair.Agreement, domain.AgreementIdentical)
	}
}

// assertConceptAgreementPersisted proves concept_aggregate and
// concept_agreement are NOT empty on the real on-disk database at dbPath,
// bypassing the Reports return value entirely — first via
// repo.AggregateMembers (a repository method), then via a DIRECT SQL query
// against concept_agreement, the exact table the finding named as staying
// permanently empty.
func assertConceptAgreementPersisted(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()

	repo, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open: unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	members, err := repo.AggregateMembers(ctx, "eurosl:concept:e-agg1")
	if err != nil {
		t.Fatalf("AggregateMembers: unexpected error: %v", err)
	}
	if len(members) != 1 || members[0] != "wcvp:concept:415853" {
		t.Errorf("AggregateMembers(eurosl:concept:e-agg1) = %v, want [wcvp:concept:415853]", members)
	}

	assertGenusNeverAggregatingSide(t, repo)

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: unexpected error: %v", err)
	}
	defer func() { _ = rawDB.Close() }()

	var count int
	if err := rawDB.QueryRow("SELECT COUNT(*) FROM concept_agreement").Scan(&count); err != nil {
		t.Fatalf("direct SQL query on concept_agreement: unexpected error: %v", err)
	}
	if count == 0 {
		t.Error("SELECT COUNT(*) FROM concept_agreement = 0, want > 0 — the exact regression this test guards against")
	}

	var agreement string
	if err := rawDB.QueryRow(
		"SELECT agreement FROM concept_agreement WHERE eurosl_concept_id = ?",
		"eurosl:concept:e-agg1",
	).Scan(&agreement); err != nil {
		t.Fatalf("direct SQL query for the eurosl:concept:e-agg1 row: unexpected error: %v", err)
	}
	if agreement != string(domain.AgreementIdentical) {
		t.Errorf("concept_agreement.agreement (raw SQL) = %q, want %q", agreement, domain.AgreementIdentical)
	}
}

// assertGenusNeverAggregatingSide is the final-review residual-finding
// regression check, split out of assertConceptAgreementPersisted to keep
// that function's cyclomatic complexity within the linter's bound (gocyclo):
// the GENUS row (e-gen1) also qualifies as its own Fall-B concept at
// minRank=RankRoot and also gets a memberLinks entry (nativeMemberLinks
// links every ParentID edge without a rank filter) — but GENUS must NEVER
// end up as a concept_aggregate edge's aggregating side. Proven two ways:
// (1) e-gen1 itself has no members, and (2) AggregatesByMember for the
// shared WCVP member returns ONLY the real aggregate concept, not the
// genus — the exact ambiguity internal/adapters/http/taxa.go's
// aggregateMembershipsFor (no ORDER BY, prefix match) would otherwise
// resolve non-deterministically.
func assertGenusNeverAggregatingSide(t *testing.T, repo *sqlite.DB) {
	t.Helper()
	ctx := context.Background()

	genusMembers, err := repo.AggregateMembers(ctx, "eurosl:concept:e-gen1")
	if err != nil {
		t.Fatalf("AggregateMembers(e-gen1): unexpected error: %v", err)
	}
	if len(genusMembers) != 0 {
		t.Errorf("AggregateMembers(eurosl:concept:e-gen1) = %v, want empty (GENUS is never the aggregating side)", genusMembers)
	}

	aggregatesByMember, err := repo.AggregatesByMember(ctx, "wcvp:concept:415853")
	if err != nil {
		t.Fatalf("AggregatesByMember: unexpected error: %v", err)
	}
	for _, id := range aggregatesByMember {
		if id == "eurosl:concept:e-gen1" || id == "germansl:concept:g-gen1" {
			t.Errorf("AggregatesByMember(wcvp:concept:415853) = %v, must not contain a GENUS concept id (%q)", aggregatesByMember, id)
		}
	}
}
