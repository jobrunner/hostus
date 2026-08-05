package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
)

// TestIngestCommand_FixtureManifest_PrintsReport drives "hostus ingest
// --dataset <fixture> --db <temp file>" end to end through the real cobra
// wiring and asserts it reports per-backbone counts a human (or T9's
// downstream automation) can read, rather than just silently succeeding.
func TestIngestCommand_FixtureManifest_PrintsReport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml", "--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "wcvp") {
		t.Errorf("report %q, want it to mention backbone %q", got, "wcvp")
	}
	// 20 taxon rows in the wcvp-sample fixture, every one gets a Name row
	// (see internal/application/ingest_test.go's TestIngest_WCVPFixture_ReportCounts,
	// which pins the same fixture's exact counts).
	if !strings.Contains(got, strconv.Itoa(20)) {
		t.Errorf("report %q, want it to mention the Names count %d", got, 20)
	}

	// The fixture manifest's trait_vocabularies section (eive, tichy2023)
	// must also print a report, including the unmatched sample — the
	// crosswalk loss must be visible on the terminal, not just in the DB.
	if !strings.Contains(got, "eive") {
		t.Errorf("report %q, want it to mention trait vocabulary %q", got, "eive")
	}
	if !strings.Contains(got, "tichy2023") {
		t.Errorf("report %q, want it to mention trait vocabulary %q", got, "tichy2023")
	}
	if !strings.Contains(got, "unmatched sample") {
		t.Errorf("report %q, want it to print the unmatched sample (Abies alba/Quercus robur are absent from the wcvp fixture)", got)
	}
	if !strings.Contains(got, "Abies alba") {
		t.Errorf("report %q, want the unmatched sample to name the specific lost taxa", got)
	}
}

// TestPrintIngestReport_OtherRanksNotice proves Hardening Task 1's "visible,
// not silent" requirement at the CLI layer: a backbone report carrying
// OtherRanks must print a "ranks: other=N (...)" line naming the exotic
// spellings (WCVP's "proles", the empty string rendered as "(empty)" so it
// stays readable rather than printing nothing) — and a backbone with
// OtherRanks == 0 must print no such line at all.
func TestPrintIngestReport_OtherRanksNotice(t *testing.T) {
	report := application.IngestReport{
		Backbones: []application.BackboneReport{
			{
				ID:         "wcvp",
				Names:      3,
				OtherRanks: 3,
				OtherRankSample: []application.RankVerbatimCount{
					{Verbatim: "proles", Count: 2},
					{Verbatim: "", Count: 1},
				},
			},
			{ID: "clean", Names: 1},
		},
	}

	var out bytes.Buffer
	printIngestReport(&out, report)

	got := out.String()
	if !strings.Contains(got, "ranks: other=3 (proles 2, (empty) 1)") {
		t.Errorf("report %q, want a \"ranks: other=3 (proles 2, (empty) 1)\" line", got)
	}
	cleanIdx := strings.Index(got, "clean:")
	if cleanIdx == -1 {
		t.Fatalf("report %q, want it to mention backbone %q", got, "clean")
	}
	cleanSection := got[cleanIdx:]
	if strings.Contains(cleanSection, "ranks: other") {
		t.Errorf("report %q, want no \"ranks: other\" line for a backbone with OtherRanks == 0", cleanSection)
	}
}

// TestIngestCommand_RestrictedVocabulary_PrintsRedistributionNotice drives
// "hostus ingest" against a manifest whose eive trait vocabulary is pinned
// redistribution: unknown (testdata/dataset-restricted.yaml) and asserts
// the printed report includes the German "hinweis:" notice line — local
// ingest itself must still succeed (it is never gated), but the operator
// must SEE that this source cannot be shipped in an exported bundle without
// --force-include-restricted (see bundle_test.go's companion smoke test).
func TestIngestCommand_RestrictedVocabulary_PrintsRedistributionNotice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dataset=testdata/dataset-restricted.yaml", "--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "hinweis: eive (redistribution=unknown)") {
		t.Errorf("report %q, want a hinweis line naming eive's redistribution=unknown", got)
	}
	if !strings.Contains(got, "nicht redistribuierbar") {
		t.Errorf("report %q, want the notice to state it is not redistributable", got)
	}
}

// TestIngestCommand_MissingDatasetFlag_ReturnsError pins the "not
// implemented" stub's old exit-1 behavior (see main_test.go's TestRun,
// which invokes "hostus ingest" with no flags at all and expects exit 1):
// the real implementation must still fail, just for a different reason
// (--dataset is required), not silently succeed against an empty path.
func TestIngestCommand_MissingDatasetFlag_ReturnsError(t *testing.T) {
	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --dataset is missing, got nil")
	}
}

// TestIngestCommand_MissingDBFlag_ReturnsError confirms --db is likewise
// required: ingest must never silently pick an implicit database location.
func TestIngestCommand_MissingDBFlag_ReturnsError(t *testing.T) {
	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --db is missing, got nil")
	}
}

// TestIngestCommand_InvalidManifest_ReturnsError confirms a manifest that
// fails schema validation is rejected before any database write is
// attempted.
func TestIngestCommand_InvalidManifest_ReturnsError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset-invalid.yaml", "--db=" + dbPath})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error for an invalid manifest, got nil")
	}
}

// TestIngestCommand_RegisteredOnRoot confirms "hostus ingest" is wired into
// the command tree, not just constructible in isolation.
func TestIngestCommand_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{ingestCmdName})
	if err != nil {
		t.Fatalf("Find(ingest): %v", err)
	}
	if cmd.Use != ingestCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, ingestCmdName)
	}
}

// TestPrintTraitReports_NormalisationVisibleAndFlagged is Hardening Task 5's
// "visible, not silent" requirement at the CLI layer: a normalised match
// must be attributed to its rule, and the two rules that equate two
// circumscriptions (aggregate-to-nominate-species, autonym-to-species) must
// additionally be marked as flagged and name their taxa — otherwise a
// judgement call would be indistinguishable from an exact hit in the only
// output an operator ever sees.
func TestPrintTraitReports_NormalisationVisibleAndFlagged(t *testing.T) {
	reports := []application.TraitIngestReport{{
		Vocab: "eive", Rows: 3, Matched: 3,
		Normalized: []application.RuleCount{
			{Rule: "aggregate_to_nominate", Rows: 2, Taxa: 1, Flagged: true},
			{Rule: "hybrid_spacing", Rows: 1, Taxa: 1},
		},
		FlaggedSample: []string{"Acer opalus aggr."},
	}}

	var buf bytes.Buffer
	printTraitReports(&buf, reports)
	got := buf.String()

	for _, want := range []string{
		"normalized aggregate_to_nominate: rows=2 taxa=1",
		"flagged",
		"normalized hybrid_spacing: rows=1 taxa=1",
		"flagged sample: Acer opalus aggr.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report %q, want it to contain %q", got, want)
		}
	}
	// The unflagged rule's line must NOT carry the flag marker.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "hybrid_spacing") && strings.Contains(line, "flagged") {
			t.Errorf("line %q marks an unflagged rule as flagged", line)
		}
	}
}

func TestPrintTraitReports_ExactOnlyVocabularyPrintsNoNormalisationLines(t *testing.T) {
	var buf bytes.Buffer
	printTraitReports(&buf, []application.TraitIngestReport{{Vocab: "eive", Rows: 1, Matched: 1}})
	got := buf.String()
	if strings.Contains(got, "normalized") || strings.Contains(got, "flagged") {
		t.Errorf("report %q, want no normalisation lines when nothing was normalised", got)
	}
}

func TestPrintXrefReports_CoverageConflictsAndSamplesVisible(t *testing.T) {
	reports := []application.XrefIngestReport{{
		Source: "wikidata", Rows: 5, Matched: 3, Unmatched: 1, Conflicting: 1,
		PerAuthority:      map[string]int{"inat": 2, "gbif": 3},
		MultiPerAuthority: map[string]int{"wikidata": 1},
		UnmatchedSample:   []string{"powo:999999-9"},
		ConflictSample:    []string{"inat:900002"},
		Redistribution:    "allowed",
	}}

	var buf bytes.Buffer
	printXrefReports(&buf, reports)
	got := buf.String()

	for _, want := range []string{
		"wikidata: rows=5 matched=3 unmatched=1 conflicting=1",
		"gbif: concepts=3",
		"inat: concepts=2",
		"wikidata: 1 concept(s) hold more than one id (not a conflict)",
		"unmatched sample: powo:999999-9",
		"conflict sample: inat:900002",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report %q, want it to contain %q", got, want)
		}
	}
}

func TestPrintXrefReports_NoSourcesPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	printXrefReports(&buf, nil)
	if got := buf.String(); got != "" {
		t.Errorf("report = %q, want empty when no xref sources were ingested", got)
	}
}

func TestPrintXrefReports_CleanSourcePrintsNoSampleLines(t *testing.T) {
	var buf bytes.Buffer
	printXrefReports(&buf, []application.XrefIngestReport{{Source: "wikidata", Rows: 1, Matched: 1, PerAuthority: map[string]int{"inat": 1}}})
	got := buf.String()
	if strings.Contains(got, "unmatched sample") || strings.Contains(got, "conflict sample") {
		t.Errorf("report %q, want no sample lines when nothing was unmatched or conflicting", got)
	}
}

func TestPrintConceptSourceReports_LossCountersVisible(t *testing.T) {
	var buf bytes.Buffer
	printConceptSourceReports(&buf, []application.CDMIngestReport{{
		Backbone: "cdm", Concepts: 18, ConceptsWritten: 18, SecReferences: 10,
		Relations: 14, RelationsWritten: 13,
		PerRelationType:   map[string]int{"congruent": 9, "includes": 1},
		NonConcept:        1,
		NonConceptSample:  []string{"3a04771f"},
		UnresolvedEnds:    2,
		UnresolvedSample:  []string{"aaa->ghost"},
		UnresolvedParents: 1,
		ReaderErrors:      0,
		UnknownFlag:       3,
		OtherRanks:        4,
		OtherRankSample:   []application.RankVerbatimCount{{Verbatim: "Species Aggregate", Count: 4}},
		Redistribution:    "unknown",
	}})
	out := buf.String()
	for _, want := range []string{
		"cdm: concepts=18 written=18 sec_spaces=10 relations=14 written=13",
		"congruent: 9",
		"misapplied/non-concept=1",
		"unresolved ends=2",
		"unresolved parents=1",
		"unknown concept-relation flag=3",
		"Species Aggregate 4",
		"non-concept sample: 3a04771f",
		"unresolved sample: aaa->ghost",
		"redistribution=unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintConceptSourceReports_NoSourcesPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	printConceptSourceReports(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("want no output, got %q", buf.String())
	}
}

func TestPrintConceptSourceReports_CleanSourcePrintsNoSampleLines(t *testing.T) {
	var buf bytes.Buffer
	printConceptSourceReports(&buf, []application.CDMIngestReport{{
		Backbone: "cdm", Concepts: 1, ConceptsWritten: 1, SecReferences: 1,
		Redistribution: "allowed",
	}})
	out := buf.String()
	for _, unwanted := range []string{"sample:", "ranks:", "hinweis:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("clean source must not print %q:\n%s", unwanted, out)
		}
	}
}

// TestPrintNameSpaceReports_LossCountersAndAggregatesVisible pins that every
// loss mode of the name-space crosswalk reaches the operator's terminal —
// and that the aggregates line is printed separately from the headline rate,
// since an aggregate can only ever resolve through the flagged
// aggregate-to-nominate rule.
func TestPrintNameSpaceReports_LossCountersAndAggregatesVisible(t *testing.T) {
	reports := []application.NameSpaceIngestReport{{
		Space: "floraveg", Rows: 6, Matched: 3, Unmatched: 1, Ambiguous: 1,
		Concepts: 2, Aggregates: 3, AggregatesMatched: 2,
		DuplicateExtIDs: 1, ReaderErrors: 2,
		Normalized: []application.RuleCount{
			{Rule: "aggregate_to_nominate", Rows: 2, Taxa: 2, Flagged: true},
			{Rule: "hybrid_spacing", Rows: 1, Taxa: 1},
		},
		FlaggedSample:   []string{"Festuca ovina aggr."},
		UnmatchedSample: []string{"Abies alba"},
		AmbiguousSample: []string{"Ambiguous name"},
		DuplicateSample: []string{"5647"},
		Redistribution:  "unknown",
	}}

	var buf bytes.Buffer
	printNameSpaceReports(&buf, reports)
	got := buf.String()

	for _, want := range []string{
		"floraveg: rows=6 matched=3 unmatched=1 ambiguous=1 concepts=2",
		"aggregates: 2 of 3 resolved",
		"dropped: duplicate ext_ids=1 reader errors=2",
		"normalized aggregate_to_nominate: rows=2 taxa=2",
		"normalized hybrid_spacing: rows=1 taxa=1",
		"flagged sample: Festuca ovina aggr.",
		"unmatched sample: Abies alba",
		"ambiguous sample: Ambiguous name",
		"duplicate ext_id sample: 5647",
		"hinweis: floraveg (redistribution=unknown)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report %q, want it to contain %q", got, want)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "hybrid_spacing") && strings.Contains(line, "flagged") {
			t.Errorf("line %q marks an unflagged rule as flagged", line)
		}
	}
}

func TestPrintNameSpaceReports_NoSpacesPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	printNameSpaceReports(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("want no output, got %q", buf.String())
	}
}

// TestPrintNameSpaceReports_CleanSpacePrintsNoSampleLines pins the other
// boundary: a space that resolved everything prints its counters but no
// sample or normalisation noise, and an "allowed" space gets no hinweis.
func TestPrintNameSpaceReports_CleanSpacePrintsNoSampleLines(t *testing.T) {
	var buf bytes.Buffer
	printNameSpaceReports(&buf, []application.NameSpaceIngestReport{{
		Space: "floraveg", Rows: 1, Matched: 1, Concepts: 1, Redistribution: "allowed",
	}})
	out := buf.String()
	for _, unwanted := range []string{"sample:", "normalized", "hinweis:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("clean space must not print %q:\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "aggregates: 0 of 0 resolved") {
		t.Errorf("want the aggregates line even at zero (an absent line reads as \"not measured\"):\n%s", out)
	}
}

// TestIngestCommand_NameSpace_PrintsReport drives the whole CLI against the
// fixture manifest (which pins the FloraVeg name space) and asserts the
// name-space section reaches stdout.
func TestIngestCommand_NameSpace_PrintsReport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostus.sqlite")

	cmd := newIngestCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml", "--db=" + dbPath})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"Name spaces:",
		"floraveg: rows=5 matched=3 unmatched=2 ambiguous=0 concepts=1",
		"aggregates: 2 of 3 resolved",
		"hinweis: floraveg (redistribution=unknown)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("hostus ingest output %q, want it to contain %q", out, want)
		}
	}
}
