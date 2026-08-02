package cdm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/cdm"
)

// fixtureDir is the committed 18-concept/14-relation CDM sample
// (pipelines/cdm/fixtures) — the real 51.466-concept crawl is a 16–20 h job
// and is never a test dependency.
func fixtureDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "pipelines", "cdm", "fixtures")
}

func writeCSV(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

const conceptHeader = "concept_uuid|scientific_name|authorship|rank|status|sec_uuid|sec_title|classification_uuid|parent_uuid\n"
const relationHeader = "from_uuid|to_uuid|relation_type|relation_symbol|is_concept_relation|relationship_uuid\n"

func TestReadConceptsFixture(t *testing.T) {
	ds, err := cdm.ReadConcepts(filepath.Join(fixtureDir(t), "cdm-concepts-fixture.csv"))
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	if len(ds.Errors) != 0 {
		t.Fatalf("unexpected row errors: %v", ds.Errors)
	}
	if len(ds.Rows) != 18 {
		t.Fatalf("got %d concept rows, want 18", len(ds.Rows))
	}
	// The hub concept: Abies alba sec. Wisskirchen & Haeupler 1998.
	var hub *cdm.ConceptRow
	for i := range ds.Rows {
		if ds.Rows[i].ConceptUUID == "872088a4-95f4-472c-ae79-a29028bb3fbf" {
			hub = &ds.Rows[i]
		}
	}
	if hub == nil {
		t.Fatal("hub concept not found")
	}
	if hub.ScientificName != "Abies alba" || hub.Authorship != "Mill." {
		t.Errorf("hub name/authorship = %q/%q", hub.ScientificName, hub.Authorship)
	}
	if hub.Rank != "Species" || hub.Status != "Accepted" {
		t.Errorf("hub rank/status = %q/%q", hub.Rank, hub.Status)
	}
	if !strings.HasPrefix(hub.SecTitle, "Wisskirchen & Haeupler 1998") {
		t.Errorf("hub sec title = %q", hub.SecTitle)
	}
	if hub.ParentUUID != "edb469f3-aa1d-4edb-97c2-66cff43bc885" {
		t.Errorf("hub parent = %q", hub.ParentUUID)
	}
}

func TestReadConceptsKeepsEmptySecAndStatusAsEmpty(t *testing.T) {
	// Empty status is HONEST data (the tree walk has not reached that
	// concept), not a missing value to be defaulted. Same for an unmapped
	// sec.
	ds, err := cdm.ReadConcepts(filepath.Join(fixtureDir(t), "cdm-concepts-fixture.csv"))
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	var found bool
	for _, r := range ds.Rows {
		if r.ConceptUUID == "2a9439bf-0cd9-4d49-a140-7ee0e695de06" {
			found = true
			if r.SecUUID != "" || r.SecTitle != "" || r.ClassificationUUID != "" {
				t.Errorf("expected empty sec/classification, got %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("sec-less concept not in fixture")
	}
}

func TestReadConceptsParsesRFC4180QuotingNotStringSplit(t *testing.T) {
	// 237 concepts in the real crawl carry a '"'. A strings.Split on '|'
	// would silently corrupt them; encoding/csv with Comma='|' must not.
	path := writeCSV(t, "quoted.csv", conceptHeader+
		`u1|"Achillea millefolium ""Sammelart"""|L.|Species|Accepted|s1|Sec One|c1|`+"\n")
	ds, err := cdm.ReadConcepts(path)
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(ds.Rows))
	}
	if want := `Achillea millefolium "Sammelart"`; ds.Rows[0].ScientificName != want {
		t.Errorf("scientific_name = %q, want %q", ds.Rows[0].ScientificName, want)
	}
}

func TestReadConceptsCollectsShortRowsWithLineNumbers(t *testing.T) {
	path := writeCSV(t, "short.csv", conceptHeader+
		"u1|Abies alba|Mill.|Species|Accepted|s1|Sec One|c1|\n"+
		"u2|Only three|fields\n"+
		"u3|Pinus abies|L.|Species|Accepted|s1|Sec One|c1|\n")
	ds, err := cdm.ReadConcepts(path)
	if err != nil {
		t.Fatalf("ReadConcepts must not fail on a malformed row: %v", err)
	}
	if len(ds.Rows) != 2 {
		t.Fatalf("got %d rows, want 2 (short row skipped)", len(ds.Rows))
	}
	if len(ds.Errors) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(ds.Errors), ds.Errors)
	}
	if !strings.Contains(ds.Errors[0].Error(), ":3:") {
		t.Errorf("error %q does not name line 3", ds.Errors[0])
	}
}

func TestReadConceptsRejectsARowOneFieldShortOfTheHeader(t *testing.T) {
	// The exact boundary: 8 fields where the header declares 9. Anything
	// looser than "at least as many fields as the RIGHTMOST wanted column's
	// position" would index past the row and panic on the real crawl.
	path := writeCSV(t, "boundary.csv", conceptHeader+
		"u1|Abies alba|Mill.|Species|Accepted|s1|Sec One|c1\n")
	ds, err := cdm.ReadConcepts(path)
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	if len(ds.Rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(ds.Rows))
	}
	if len(ds.Errors) != 1 || !strings.Contains(ds.Errors[0].Error(), "want at least 9") {
		t.Fatalf("want one short-row error naming the 9-field minimum, got %v", ds.Errors)
	}
}

func TestReadRelationsRejectsARowOneFieldShortOfTheHeader(t *testing.T) {
	path := writeCSV(t, "boundary.csv", relationHeader+
		"a|b|Congruent to|≜|true\n")
	ds, err := cdm.ReadRelations(path)
	if err != nil {
		t.Fatalf("ReadRelations: %v", err)
	}
	if len(ds.Rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(ds.Rows))
	}
	if len(ds.Errors) != 1 || !strings.Contains(ds.Errors[0].Error(), "want at least 6") {
		t.Fatalf("want one short-row error naming the 6-field minimum, got %v", ds.Errors)
	}
}

func TestReadConceptsRejectsMissingColumn(t *testing.T) {
	path := writeCSV(t, "badheader.csv", "concept_uuid|scientific_name\nu1|Abies alba\n")
	if _, err := cdm.ReadConcepts(path); err == nil {
		t.Fatal("want error for a header missing required columns")
	}
}

func TestReadConceptsMissingFile(t *testing.T) {
	if _, err := cdm.ReadConcepts(filepath.Join(t.TempDir(), "nope.csv")); err == nil {
		t.Fatal("want error for a missing file")
	}
}

func TestReadConceptsEmptyFile(t *testing.T) {
	if _, err := cdm.ReadConcepts(writeCSV(t, "empty.csv", "")); err == nil {
		t.Fatal("want error for an empty file (no header)")
	}
}

func TestReadRelationsFixture(t *testing.T) {
	ds, err := cdm.ReadRelations(filepath.Join(fixtureDir(t), "cdm-relations-fixture.csv"))
	if err != nil {
		t.Fatalf("ReadRelations: %v", err)
	}
	if len(ds.Errors) != 0 {
		t.Fatalf("unexpected row errors: %v", ds.Errors)
	}
	if len(ds.Rows) != 14 {
		t.Fatalf("got %d relation rows, want 14", len(ds.Rows))
	}
	byType := map[string]int{}
	for _, r := range ds.Rows {
		byType[r.RelationType]++
	}
	for raw, want := range map[string]int{
		"Congruent to":                        9,
		"Includes":                            1,
		"Overlaps":                            1,
		"Included in or Includes or Overlaps": 1,
		"is pro parte synonym for":            1,
		"is misapplied name for":              1,
	} {
		if byType[raw] != want {
			t.Errorf("relation type %q: got %d, want %d", raw, byType[raw], want)
		}
	}
}

func TestReadRelationsTriStateConceptFlag(t *testing.T) {
	// is_concept_relation is a TRI-STATE: "true", "false", and EMPTY
	// (unknown — a to-end-only edge), which must not be folded into false.
	path := writeCSV(t, "rel.csv", relationHeader+
		"a|b|Congruent to|≜|true|r1\n"+
		"c|d|is misapplied name for|misapplied for|false|r2\n"+
		"e|f|Includes|⊃||r3\n")
	ds, err := cdm.ReadRelations(path)
	if err != nil {
		t.Fatalf("ReadRelations: %v", err)
	}
	if len(ds.Rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(ds.Rows))
	}
	if ds.Rows[0].IsConceptRelation == nil || !*ds.Rows[0].IsConceptRelation {
		t.Error(`"true" must decode to a non-nil true`)
	}
	if ds.Rows[1].IsConceptRelation == nil || *ds.Rows[1].IsConceptRelation {
		t.Error(`"false" must decode to a non-nil false`)
	}
	if ds.Rows[2].IsConceptRelation != nil {
		t.Error("empty must decode to nil (unknown), never to false")
	}
}

func TestReadRelationsUnparseableFlagIsCollectedNotGuessed(t *testing.T) {
	path := writeCSV(t, "rel.csv", relationHeader+
		"a|b|Congruent to|≜|maybe|r1\n"+
		"c|d|Congruent to|≜|true|r2\n")
	ds, err := cdm.ReadRelations(path)
	if err != nil {
		t.Fatalf("ReadRelations: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("got %d rows, want 1 (bad flag row skipped)", len(ds.Rows))
	}
	if len(ds.Errors) != 1 || !strings.Contains(ds.Errors[0].Error(), "maybe") {
		t.Fatalf("want one error naming the offending value, got %v", ds.Errors)
	}
}

func TestReadRelationsCollectsShortRowsWithLineNumbers(t *testing.T) {
	path := writeCSV(t, "rel.csv", relationHeader+
		"a|b\n"+
		"c|d|Congruent to|≜|true|r2\n")
	ds, err := cdm.ReadRelations(path)
	if err != nil {
		t.Fatalf("ReadRelations: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(ds.Rows))
	}
	if len(ds.Errors) != 1 || !strings.Contains(ds.Errors[0].Error(), ":2:") {
		t.Fatalf("want one error naming line 2, got %v", ds.Errors)
	}
}

func TestReadRelationsRejectsMissingColumn(t *testing.T) {
	path := writeCSV(t, "rel.csv", "from_uuid|to_uuid\na|b\n")
	if _, err := cdm.ReadRelations(path); err == nil {
		t.Fatal("want error for a header missing required columns")
	}
}

func TestReadRelationsMissingFile(t *testing.T) {
	if _, err := cdm.ReadRelations(filepath.Join(t.TempDir(), "nope.csv")); err == nil {
		t.Fatal("want error for a missing file")
	}
}

func TestReadRelationsEmptyFile(t *testing.T) {
	if _, err := cdm.ReadRelations(writeCSV(t, "empty.csv", "")); err == nil {
		t.Fatal("want error for an empty file (no header)")
	}
}

func TestReadConceptsLineNumbersSurviveAQuotedNewline(t *testing.T) {
	// A quoted field may span newlines. A counter incremented once per record
	// would then point at the wrong file line — csv.Reader.FieldPos does not.
	path := writeCSV(t, "multiline.csv", conceptHeader+
		"u1|\"Abies\nalba\"|Mill.|Species|Accepted|s1|Sec One|c1|\n"+
		"u2|too|few\n")
	ds, err := cdm.ReadConcepts(path)
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	if len(ds.Rows) != 1 || ds.Rows[0].ScientificName != "Abies\nalba" {
		t.Fatalf("rows = %+v", ds.Rows)
	}
	// The header is line 1, the quoted record spans lines 2-3, so the short
	// row is on line 4 — not line 3, which a per-record counter would report.
	if len(ds.Errors) != 1 || !strings.Contains(ds.Errors[0].Error(), ":4:") {
		t.Fatalf("want one error naming line 4, got %v", ds.Errors)
	}
}

func TestReadConceptsGivesUpOnAWallOfUnreadableRecords(t *testing.T) {
	// The one case that does NOT advance the file on its own: a csv read
	// error. A sticky I/O error would otherwise make Read return the same
	// error forever and grow ds.Errors without bound. (Short rows and bad
	// flag values are already bounded by the file, since each consumes
	// input.) The reader reports that it gave up and stops.
	var b strings.Builder
	b.WriteString(conceptHeader)
	for range 200 {
		b.WriteString(`u|Abies "alba"|Mill.|Species|Accepted|s1|Sec One|c1|` + "\n")
	}
	ds, err := cdm.ReadConcepts(writeCSV(t, "allbad.csv", b.String()))
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	if len(ds.Rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(ds.Rows))
	}
	// Exactly the bound's worth of read failures plus the one give-up line,
	// pinned as an exact number so the bound's boundary is observable.
	if len(ds.Errors) != 21 {
		t.Fatalf("collected %d errors, want 21 (20 unreadable records + the give-up line)", len(ds.Errors))
	}
	last := ds.Errors[len(ds.Errors)-1].Error()
	if !strings.Contains(last, "giving up after 20 consecutive unreadable records") {
		t.Errorf("last error %q does not say the reader gave up", last)
	}
}

func TestReadConceptsParseErrorReportsTheParseErrorsOwnLine(t *testing.T) {
	// After a quoted field spanning two lines, the record ORDINAL and the
	// real file line have diverged. A parse error must report the latter,
	// which means taking it from the csv.ParseError rather than the ordinal.
	path := writeCSV(t, "multiline-then-bad.csv", conceptHeader+
		"u1|\"Abies\nalba\"|Mill.|Species|Accepted|s1|Sec One|c1|\n"+
		`u2|Pinus "abies"|L.|Species|Accepted|s1|Sec One|c1|`+"\n")
	ds, err := cdm.ReadConcepts(path)
	if err != nil {
		t.Fatalf("ReadConcepts: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(ds.Rows))
	}
	if len(ds.Errors) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(ds.Errors), ds.Errors)
	}
	// Header is line 1, the quoted record spans lines 2-3, so the bad record
	// sits on line 4 — while its record ordinal is only 3.
	got := ds.Errors[0].Error()
	if !strings.Contains(got, ".csv:4:") {
		t.Errorf("error %q does not report file line 4", got)
	}
	if strings.Contains(got, ".csv:3:") {
		t.Errorf("error %q reports the record ordinal instead of the file line", got)
	}
}

func TestReadRelationsGivesUpOnAFileOfNothingButBadRecords(t *testing.T) {
	var b strings.Builder
	b.WriteString(relationHeader)
	for range 200 {
		b.WriteString("a|b|Congruent to|≜|maybe|r\n")
	}
	ds, err := cdm.ReadRelations(writeCSV(t, "allbad.csv", b.String()))
	if err != nil {
		t.Fatalf("ReadRelations: %v", err)
	}
	// An unparseable FLAG is a per-row emit error, not a csv read error, so
	// it does not trip the consecutive-read-failure bound — it is still
	// bounded by the file, and every row is reported.
	if len(ds.Rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(ds.Rows))
	}
	if len(ds.Errors) != 200 {
		t.Fatalf("got %d errors, want one per bad row", len(ds.Errors))
	}
}
