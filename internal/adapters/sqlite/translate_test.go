package sqlite_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// The three sec. spaces the chain fixture below uses. A/B/C are three
// DIFFERENT reference spaces, which is what makes the one-hop boundary
// observable: A relates to B, B relates to C, and nothing relates A to C.
const (
	secA = "sec-a"
	secB = "sec-b"
	secC = "sec-c"
)

// cdmConcept builds one harvest row. Every fixture below uses the SAME
// name in different sec. spaces on purpose — that is the UC6 shape.
func cdmConcept(uuid, sec, title string) application.CDMConceptRow {
	return application.CDMConceptRow{
		ConceptUUID: uuid, ScientificName: "Abies alba", Authorship: "Mill.",
		Rank: "Species", Status: "Accepted", SecUUID: sec, SecTitle: title,
	}
}

func cdmRelation(from, to, relType string) application.CDMRelationRow {
	yes := true
	return application.CDMRelationRow{
		FromUUID: from, ToUUID: to, RelationType: relType, IsConceptRelation: &yes,
		RelationshipUUID: from + "-" + to + "-" + relType,
	}
}

// chainDB seeds A --congruent--> B --includes--> C, all three carrying the
// same name in three different sec. spaces.
func chainDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db, _ := openTempDB(t)
	concepts := []application.CDMConceptRow{
		cdmConcept("a", secA, "Rothmaler, 8. Aufl."),
		cdmConcept("b", secB, "Wisskirchen & Haeupler 1998"),
		cdmConcept("c", secC, "HEGI, Aufl. 2 u. 3"),
	}
	relations := []application.CDMRelationRow{
		cdmRelation("a", "b", "Congruent to"),
		cdmRelation("b", "c", "Includes"),
	}
	if _, err := application.IngestCDM(context.Background(), db, concepts, relations, cdmBackbone()); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}
	return db
}

// TestConceptRelationsInSecStopsAtOneHop is the third non-negotiable
// verified against a real database rather than a stub: A has a relation
// into B's space and B has one into C's, so a chaining implementation would
// answer "A -> C". Exactly one hop means it must not.
func TestConceptRelationsInSecStopsAtOneHop(t *testing.T) {
	db := chainDB(t)
	ctx := context.Background()

	intoB, err := db.ConceptRelationsInSec(ctx, "cdm:concept:a", secB)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec(a, secB): unexpected error: %v", err)
	}
	if len(intoB.Edges) != 1 || intoB.Edges[0].Relation != domain.RelationCongruent {
		t.Fatalf("a -> secB = %+v, want one congruent edge", intoB.Edges)
	}

	intoC, err := db.ConceptRelationsInSec(ctx, "cdm:concept:a", secC)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec(a, secC): unexpected error: %v", err)
	}
	if len(intoC.Edges) != 0 {
		t.Errorf("a -> secC = %+v, want no edges: the only path is two hops (a congruent b, b includes c)", intoC.Edges)
	}
}

// TestTranslateStopsAtOneHop is the same boundary through the use case, so
// the application layer cannot reintroduce chaining above the repository.
func TestTranslateStopsAtOneHop(t *testing.T) {
	db := chainDB(t)
	res, err := application.Translate(context.Background(), db, application.TranslateRequest{
		ConceptID: "cdm:concept:a", TargetSec: secC,
	})
	if err != nil {
		t.Fatalf("Translate: unexpected error: %v", err)
	}
	if res.HasRelation() {
		t.Errorf("Translate(a -> secC) returned %+v, want the explicit empty answer", res.Candidates)
	}
	if res.Note == "" {
		t.Errorf("empty answer carries no note")
	}
}

// TestConceptRelationsInSecReturnsBothStoredDirections: hostus stores only
// the direction the source states, so B must be able to see its INCOMING
// edge from A — with Outgoing false, and with the relation value left
// exactly as stored.
func TestConceptRelationsInSecReturnsBothStoredDirections(t *testing.T) {
	db := chainDB(t)
	ctx := context.Background()

	out, err := db.ConceptRelationsInSec(ctx, "cdm:concept:b", secA)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec(b, secA): unexpected error: %v", err)
	}
	if len(out.Edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(out.Edges))
	}
	if out.Edges[0].Outgoing {
		t.Errorf("Outgoing = true, want false — the stored row reads a -> b")
	}
	if out.Edges[0].Partner.ID != "cdm:concept:a" {
		t.Errorf("Partner = %q, want cdm:concept:a", out.Edges[0].Partner.ID)
	}
	if out.Edges[0].PartnerSec.ID != secA || !strings.HasPrefix(out.Edges[0].PartnerSec.Title, "Rothmaler") {
		t.Errorf("PartnerSec = %+v, want the resolved secA citation", out.Edges[0].PartnerSec)
	}
	if out.Edges[0].Source != "cdm" {
		t.Errorf("Source = %q, want cdm", out.Edges[0].Source)
	}

	forward, err := db.ConceptRelationsInSec(ctx, "cdm:concept:b", secC)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec(b, secC): unexpected error: %v", err)
	}
	if len(forward.Edges) != 1 || !forward.Edges[0].Outgoing {
		t.Errorf("b -> secC = %+v, want one OUTGOING edge", forward)
	}
	if forward.Edges[0].Relation != domain.RelationIncludes {
		t.Errorf("Relation = %q, want the stored %q (never pre-inverted)", forward.Edges[0].Relation, domain.RelationIncludes)
	}
}

// TestConceptRelationsInSecKeepsTwoRelationTypesOnOnePair uses the shape
// the real harvest emits and the widened primary key exists for: two
// different relation types between the same ordered pair.
func TestConceptRelationsInSecKeepsTwoRelationTypesOnOnePair(t *testing.T) {
	db, _ := openTempDB(t)
	concepts := []application.CDMConceptRow{
		cdmConcept("a", secA, "Rothmaler, 8. Aufl."),
		cdmConcept("b", secB, "Wisskirchen & Haeupler 1998"),
	}
	relations := []application.CDMRelationRow{
		cdmRelation("a", "b", "Congruent to"),
		cdmRelation("a", "b", "Overlaps"),
	}
	if _, err := application.IngestCDM(context.Background(), db, concepts, relations, cdmBackbone()); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}

	got, err := db.ConceptRelationsInSec(context.Background(), "cdm:concept:a", secB)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec: unexpected error: %v", err)
	}
	if len(got.Edges) != 2 {
		t.Fatalf("got %d edges, want 2", len(got.Edges))
	}
	// ORDER BY tc.id, cr.relation: congruent sorts before overlaps.
	if got.Edges[0].Relation != domain.RelationCongruent || got.Edges[1].Relation != domain.RelationOverlaps {
		t.Errorf("relations = %q, %q, want congruent, overlaps in that (deterministic) order", got.Edges[0].Relation, got.Edges[1].Relation)
	}
}

// TestConceptRelationsInSecExcludesTheConceptItself: a relation whose two
// ends are the same concept is not a translation, and it would make
// Outgoing meaningless.
func TestConceptRelationsInSecExcludesTheConceptItself(t *testing.T) {
	db, _ := openTempDB(t)
	concepts := []application.CDMConceptRow{cdmConcept("a", secA, "Rothmaler, 8. Aufl.")}
	relations := []application.CDMRelationRow{cdmRelation("a", "a", "Congruent to")}
	if _, err := application.IngestCDM(context.Background(), db, concepts, relations, cdmBackbone()); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}

	got, err := db.ConceptRelationsInSec(context.Background(), "cdm:concept:a", secA)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec: unexpected error: %v", err)
	}
	if len(got.Edges) != 0 {
		t.Errorf("got %+v, want no edges — a self-relation is not a translation", got.Edges)
	}
}

func TestConceptRelationsInSecUnknownConceptIsNotFound(t *testing.T) {
	db := chainDB(t)
	_, err := db.ConceptRelationsInSec(context.Background(), "cdm:concept:nope", secB)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want domain.ErrNotFound", err)
	}
}

// TestConceptRelationsInSecKnownConceptWithoutRelations is the other half of
// that distinction: a real concept with nothing recorded returns an empty,
// non-error slice.
func TestConceptRelationsInSecKnownConceptWithoutRelations(t *testing.T) {
	db, _ := openTempDB(t)
	concepts := []application.CDMConceptRow{
		cdmConcept("a", secA, "Rothmaler, 8. Aufl."),
		cdmConcept("b", secB, "Wisskirchen & Haeupler 1998"),
	}
	if _, err := application.IngestCDM(context.Background(), db, concepts, nil, cdmBackbone()); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}
	got, err := db.ConceptRelationsInSec(context.Background(), "cdm:concept:a", secB)
	if err != nil {
		t.Fatalf("ConceptRelationsInSec: unexpected error: %v", err)
	}
	if len(got.Edges) != 0 {
		t.Errorf("got %+v, want an empty result", got.Edges)
	}
}

func TestSecReferenceByID(t *testing.T) {
	db := chainDB(t)
	got, err := db.SecReferenceByID(context.Background(), secB)
	if err != nil {
		t.Fatalf("SecReferenceByID: unexpected error: %v", err)
	}
	if got.ID != secB || !strings.HasPrefix(got.Title, "Wisskirchen") {
		t.Errorf("SecReferenceByID = %+v, want the secB citation", got)
	}
	if _, err := db.SecReferenceByID(context.Background(), "sec-nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown id: error = %v, want domain.ErrNotFound", err)
	}
}

// TestTranslateOverTheCommittedCDMFixture drives the whole use case over
// the checked-in harvest fixture (no network, no crawl): "Abies alba
// sec. Rothmaler" translated into the Wisskirchen & Haeupler 1998 space.
func TestTranslateOverTheCommittedCDMFixture(t *testing.T) {
	const (
		rothmalerConcept = "cdm:concept:b7a352aa-1f73-41f3-a4e3-b24fc1c2cd5f"
		wh98Concept      = "cdm:concept:872088a4-95f4-472c-ae79-a29028bb3fbf"
		wh98Sec          = "060afae5-76ef-44a7-921f-1202685ef351"
	)
	db, _ := openTempDB(t)
	concepts, relations := readCDMFixture(t)
	if _, err := application.IngestCDM(context.Background(), db, concepts, relations, cdmBackbone()); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}

	res, err := application.Translate(context.Background(), db, application.TranslateRequest{
		ConceptID: rothmalerConcept, TargetSec: wh98Sec,
	})
	if err != nil {
		t.Fatalf("Translate: unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(res.Candidates), res.Candidates)
	}
	got := res.Candidates[0]
	if got.Concept.ID != wh98Concept {
		t.Errorf("candidate = %q, want %q", got.Concept.ID, wh98Concept)
	}
	if got.Relation != domain.RelationCongruent || !got.IsEquality {
		t.Errorf("relation = %q (IsEquality %v), want congruent/true", got.Relation, got.IsEquality)
	}
	if !strings.HasPrefix(res.SourceSec.Title, "Schubert") {
		t.Errorf("SourceSec.Title = %q, want the Rothmaler/Schubert citation", res.SourceSec.Title)
	}
	if !strings.HasPrefix(res.TargetSec.Title, "Wisskirchen") {
		t.Errorf("TargetSec.Title = %q, want the Wisskirchen & Haeupler citation", res.TargetSec.Title)
	}
}

// TestTranslateFixtureMisappliedIsNotAConceptRelation: the fixture carries
// BOTH a misapplied and a congruent row on Pinus abies -> Abies alba. Task 3
// drops the misapplied one (it is a statement about name usage, not about
// circumscriptions), so /translate must show the congruent one alone.
func TestTranslateFixtureMisappliedIsNotAConceptRelation(t *testing.T) {
	const (
		pinusAbies = "cdm:concept:122053a6-abb7-4d4c-9f87-b7b8f6d1afef"
		wh98Sec    = "060afae5-76ef-44a7-921f-1202685ef351"
	)
	db, _ := openTempDB(t)
	concepts, relations := readCDMFixture(t)
	if _, err := application.IngestCDM(context.Background(), db, concepts, relations, cdmBackbone()); err != nil {
		t.Fatalf("IngestCDM: unexpected error: %v", err)
	}

	res, err := application.Translate(context.Background(), db, application.TranslateRequest{
		ConceptID: pinusAbies, TargetSec: wh98Sec,
	})
	if err != nil {
		t.Fatalf("Translate: unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(res.Candidates), res.Candidates)
	}
	if res.Candidates[0].Relation != domain.RelationCongruent {
		t.Errorf("relation = %q, want congruent — the misapplied row must not be served as a concept relation",
			res.Candidates[0].Relation)
	}
}

// readCDMFixture parses the committed harvest fixture (pipe-separated, one
// header line) without touching the network or the in-flight crawl cache.
func readCDMFixture(t *testing.T) ([]application.CDMConceptRow, []application.CDMRelationRow) {
	t.Helper()
	conceptCSV, err := os.ReadFile("../../../pipelines/cdm/fixtures/cdm-concepts-fixture.csv")
	if err != nil {
		t.Fatalf("reading concept fixture: %v", err)
	}
	relationCSV, err := os.ReadFile("../../../pipelines/cdm/fixtures/cdm-relations-fixture.csv")
	if err != nil {
		t.Fatalf("reading relation fixture: %v", err)
	}

	concepts := make([]application.CDMConceptRow, 0, 18)
	for _, f := range fixtureRows(t, string(conceptCSV), 9) {
		concepts = append(concepts, application.CDMConceptRow{
			ConceptUUID: f[0], ScientificName: f[1], Authorship: f[2], Rank: f[3],
			Status: f[4], SecUUID: f[5], SecTitle: f[6], ClassificationUUID: f[7], ParentUUID: f[8],
		})
	}
	relations := make([]application.CDMRelationRow, 0, 14)
	for _, f := range fixtureRows(t, string(relationCSV), 6) {
		isConcept := f[4] == "true"
		relations = append(relations, application.CDMRelationRow{
			FromUUID: f[0], ToUUID: f[1], RelationType: f[2], RelationSymbol: f[3],
			IsConceptRelation: &isConcept, RelationshipUUID: f[5],
		})
	}
	return concepts, relations
}

// fixtureRows splits a pipe-separated fixture (one header line) into its
// data rows, insisting on the exact field count so a reshaped fixture fails
// loudly instead of ingesting shifted columns.
func fixtureRows(t *testing.T, raw string, fields int) [][]string {
	t.Helper()
	var out [][]string
	for i, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if i == 0 {
			continue
		}
		f := strings.Split(line, "|")
		if len(f) != fields {
			t.Fatalf("fixture line %d: got %d fields, want %d", i+1, len(f), fields)
		}
		out = append(out, f)
	}
	return out
}
