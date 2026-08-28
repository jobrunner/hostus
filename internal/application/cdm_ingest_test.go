package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// --- fakes -----------------------------------------------------------------

type cdmRelationWrite struct {
	from   string
	to     string
	rel    domain.Relation
	source string
}

type fakeCDMTx struct {
	names     []domain.Name
	concepts  []domain.Concept
	secs      []domain.SecReference
	relations []cdmRelationWrite
	links     [][3]string
	committed bool
	rolled    bool
	failOn    string
}

func (t *fakeCDMTx) UpsertName(n domain.Name) error {
	if t.failOn == "name" {
		return errors.New("boom")
	}
	t.names = append(t.names, n)
	return nil
}

func (t *fakeCDMTx) UpsertConcept(c domain.Concept) error {
	if t.failOn == "concept" {
		return errors.New("boom")
	}
	t.concepts = append(t.concepts, c)
	return nil
}

func (t *fakeCDMTx) LinkName(conceptID, nameID, role string, _ *bool) error {
	if t.failOn == "link" {
		return errors.New("boom")
	}
	t.links = append(t.links, [3]string{conceptID, nameID, role})
	return nil
}

func (t *fakeCDMTx) UpsertSecReference(s domain.SecReference) error {
	if t.failOn == "sec" {
		return errors.New("boom")
	}
	t.secs = append(t.secs, s)
	return nil
}

func (t *fakeCDMTx) AddConceptRelation(from, to string, rel domain.Relation, source string) error {
	if t.failOn == "relation" {
		return errors.New("boom")
	}
	t.relations = append(t.relations, cdmRelationWrite{from: from, to: to, rel: rel, source: source})
	return nil
}

func (t *fakeCDMTx) AddXref(string, domain.Xref, string) error         { return nil }
func (t *fakeCDMTx) AddDistribution(string, domain.Distribution) error { return nil }
func (t *fakeCDMTx) UpsertArea(domain.Area) error                      { return nil }
func (t *fakeCDMTx) AddTraitValue(string, domain.TraitValue) error     { return nil }
func (t *fakeCDMTx) UpsertTraitVocabulary(domain.TraitVocabMeta) error { return nil }
func (t *fakeCDMTx) UpsertXrefSource(domain.XrefSourceMeta) error      { return nil }
func (t *fakeCDMTx) UpsertNameSpace(domain.NameSpaceMeta) error        { return nil }
func (t *fakeCDMTx) AddAggregateMember(string, string) error           { return nil }
func (t *fakeCDMTx) ResolveNameSpaceMember(string, string) (string, error) {
	return "", nil
}
func (t *fakeCDMTx) AddNameSpaceEntry(string, domain.NameSpaceEntry) error {
	return nil
}
func (t *fakeCDMTx) UpsertClassification(string, string, string, string) error { return nil }
func (t *fakeCDMTx) AddVernacularName(string, domain.VernacularName) error     { return nil }
func (t *fakeCDMTx) Finalize() error {
	if t.failOn == "finalize" {
		return errors.New("boom")
	}
	return nil
}
func (t *fakeCDMTx) Commit() error   { t.committed = true; return nil }
func (t *fakeCDMTx) Rollback() error { t.rolled = true; return nil }

type fakeCDMRepo struct {
	tx *fakeCDMTx
	// existing is the set of concept ids already in the database.
	existing map[string]bool
	// readsAfterBegin counts repository reads that happened while an ingest
	// transaction was open — with the sqlite adapter's SetMaxOpenConns(1)
	// that is a real deadlock, so it must stay zero.
	readsAfterBegin int
	txOpen          bool
	beginErr        error
	existingErr     error
}

func (r *fakeCDMRepo) BeginIngest(context.Context, domain.BackboneVersion) (output.IngestTx, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	r.txOpen = true
	return r.tx, nil
}

func (r *fakeCDMRepo) ExistingConceptIDs(_ context.Context, ids []string) (map[string]bool, error) {
	if r.txOpen {
		r.readsAfterBegin++
	}
	if r.existingErr != nil {
		return nil, r.existingErr
	}
	out := map[string]bool{}
	for _, id := range ids {
		if r.existing[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (r *fakeCDMRepo) SynonymCandidates(context.Context, string) ([]domain.SynonymCandidate, error) {
	return nil, nil
}

func (r *fakeCDMRepo) Areas(context.Context) ([]domain.Area, error) { return nil, nil }

func (r *fakeCDMRepo) SecReferences(context.Context) ([]domain.SecReference, error) {
	return nil, nil
}

func (r *fakeCDMRepo) Concept(context.Context, string) (*domain.Concept, []output.SynonymName, []domain.Xref, []domain.Distribution, error) {
	return nil, nil, nil, nil, nil
}

func (r *fakeCDMRepo) Classification(context.Context, string) ([]domain.ClassificationEntry, error) {
	return nil, nil
}

func (r *fakeCDMRepo) ConceptByXref(context.Context, string, string) (*domain.Concept, error) {
	return nil, nil
}

func (r *fakeCDMRepo) ConceptIDsByXref(context.Context, string, []string) (map[string]string, error) {
	return nil, nil
}

func (r *fakeCDMRepo) MatchExact(context.Context, string) ([]output.MatchCandidate, error) {
	return nil, nil
}

func (r *fakeCDMRepo) MatchFuzzyCandidates(context.Context, string, int, string, string) ([]output.MatchCandidate, error) {
	return nil, nil
}

func (r *fakeCDMRepo) BackboneVersions(context.Context) ([]domain.BackboneVersion, error) {
	return nil, nil
}

func (r *fakeCDMRepo) BuildDistributionClosure(context.Context) error {
	return nil
}

func (r *fakeCDMRepo) Traits(context.Context, string, []domain.TraitVocab) ([]domain.TraitSet, error) {
	return nil, nil
}

func (r *fakeCDMRepo) TraitVocabularies(context.Context) ([]domain.TraitVocabMeta, error) {
	return nil, nil
}

func (r *fakeCDMRepo) NameSpaceEntries(context.Context, string, []string) ([]domain.NameSpaceEntry, error) {
	return nil, nil
}

func (r *fakeCDMRepo) NameSpaces(context.Context) ([]domain.NameSpaceMeta, error) {
	return nil, nil
}

func (r *fakeCDMRepo) AggregateMembers(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *fakeCDMRepo) AggregatesByMember(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *fakeCDMRepo) VernacularNames(context.Context, string) ([]domain.VernacularName, error) {
	return nil, nil
}

func (r *fakeCDMRepo) AggregateConcepts(context.Context, string, []domain.Rank) ([]output.AggregateConceptSummary, error) {
	return nil, nil
}

func (r *fakeCDMRepo) WriteConceptAgreement(context.Context, []domain.ConceptAgreementPair) error {
	return nil
}

func (r *fakeCDMRepo) Suggest(context.Context, string, output.SuggestOpts) ([]domain.SuggestItem, error) {
	return nil, nil
}

func (r *fakeCDMRepo) BeginTraitIngest(context.Context) (output.IngestTx, error) { return r.tx, nil }

func (r *fakeCDMRepo) SecReferenceByID(context.Context, string) (domain.SecReference, error) {
	return domain.SecReference{}, nil
}

func (r *fakeCDMRepo) ConceptRelationsInSec(context.Context, string, string) (output.ConceptRelations, error) {
	return output.ConceptRelations{}, nil
}

// --- fixtures --------------------------------------------------------------

func cdmMeta() domain.BackboneVersion {
	return domain.BackboneVersion{
		ID:             "cdm",
		Version:        "2026-08-02",
		SourceURL:      "https://api.cybertaxonomy.org/rl_standardliste",
		ManifestSHA:    "deadbeef",
		Redistribution: domain.RedistributionUnknown,
	}
}

// twoSecsSameName is the SP5 case that no earlier milestone had: the same
// name in two different sec. reference spaces.
func twoSecsSameName() []application.CDMConceptRow {
	return []application.CDMConceptRow{
		{
			ConceptUUID: "aaa", ScientificName: "Abies alba", Authorship: "Mill.",
			Rank: "Species", Status: "Accepted",
			SecUUID: "sec-wh98", SecTitle: "Wisskirchen & Haeupler 1998",
		},
		{
			ConceptUUID: "bbb", ScientificName: "Abies alba", Authorship: "Mill.",
			Rank: "Species", Status: "Accepted",
			SecUUID: "sec-hegi", SecTitle: "HEGI: Illustrierte Flora von Mitteleuropa",
		},
	}
}

func newCDMRepo() *fakeCDMRepo {
	return &fakeCDMRepo{tx: &fakeCDMTx{}, existing: map[string]bool{}}
}

// --- tests -----------------------------------------------------------------

func TestIngestCDMRoundTripsSecReference(t *testing.T) {
	repo := newCDMRepo()
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), nil, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.ConceptsWritten != 2 {
		t.Fatalf("ConceptsWritten = %d, want 2", rep.ConceptsWritten)
	}
	if len(repo.tx.concepts) != 2 {
		t.Fatalf("wrote %d concepts, want 2", len(repo.tx.concepts))
	}
	for _, c := range repo.tx.concepts {
		if c.SecReference == "" {
			t.Errorf("concept %q has no sec_reference", c.ID)
		}
		if c.BackboneID != "cdm" {
			t.Errorf("concept %q backbone = %q, want cdm", c.ID, c.BackboneID)
		}
	}
	if rep.SecReferences != 2 {
		t.Errorf("SecReferences = %d, want 2", rep.SecReferences)
	}
	if len(repo.tx.secs) != 2 {
		t.Errorf("wrote %d sec_reference rows, want 2", len(repo.tx.secs))
	}
}

func TestIngestCDMKeepsSameNameInDifferentSecSpacesDistinct(t *testing.T) {
	// THE point of SP5: two rows for the same name must deliberately NOT be
	// merged.
	repo := newCDMRepo()
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), nil, cdmMeta()); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if len(repo.tx.concepts) != 2 {
		t.Fatalf("wrote %d concepts, want 2 distinct rows", len(repo.tx.concepts))
	}
	a, b := repo.tx.concepts[0], repo.tx.concepts[1]
	if a.ID == b.ID {
		t.Fatal("the two sec. spaces collapsed onto one concept id")
	}
	if a.AcceptedName.ID == b.AcceptedName.ID {
		t.Fatal("the two sec. spaces collapsed onto one name id")
	}
	if a.SecReference == b.SecReference {
		t.Fatal("the two concepts share a sec_reference")
	}
	if a.AcceptedName.Canonical != b.AcceptedName.Canonical {
		t.Fatal("fixture is wrong: the two concepts must share the NAME")
	}
}

func TestIngestCDMKeysConceptsUnderTheCDMNamespace(t *testing.T) {
	repo := newCDMRepo()
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), nil, cdmMeta()); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if got := repo.tx.concepts[0].ID; got != "cdm:concept:aaa" {
		t.Errorf("concept id = %q, want cdm:concept:aaa", got)
	}
	if got := repo.tx.names[0].ID; got != "cdm:name:aaa" {
		t.Errorf("name id = %q, want cdm:name:aaa", got)
	}
	if len(repo.tx.links) != 2 || repo.tx.links[0][2] != "accepted" {
		t.Errorf("links = %v, want one accepted link per concept", repo.tx.links)
	}
}

func TestIngestCDMWritesMappedRelationBetweenTwoSecSpaces(t *testing.T) {
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolPtr(true)},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.RelationsWritten != 1 {
		t.Fatalf("RelationsWritten = %d, want 1", rep.RelationsWritten)
	}
	got := repo.tx.relations[0]
	if got.from != "cdm:concept:bbb" || got.to != "cdm:concept:aaa" {
		t.Errorf("relation ends = %q -> %q", got.from, got.to)
	}
	if got.rel != domain.RelationCongruent {
		t.Errorf("relation = %q, want congruent", got.rel)
	}
	if got.source != "cdm" {
		t.Errorf("relation source = %q, want cdm", got.source)
	}
	if rep.PerRelationType[string(domain.RelationCongruent)] != 1 {
		t.Errorf("PerRelationType = %v", rep.PerRelationType)
	}
}

func TestIngestCDMFailsLoudlyOnUnknownRelationType(t *testing.T) {
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "Sister of", IsConceptRelation: boolPtr(true)},
	}
	_, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err == nil {
		t.Fatal("want an error for an unmapped relation type")
	}
	if !strings.Contains(err.Error(), "Sister of") {
		t.Errorf("error %q does not name the offending value", err)
	}
	if len(repo.tx.relations) != 0 || len(repo.tx.concepts) != 0 {
		t.Error("nothing may be written for an unmapped relation type")
	}
	if repo.txOpen {
		t.Error("the failure must be detected in phase 1, before a transaction is opened")
	}
}

func TestIngestCDMPreservesUncertaintyOfSubsetSupersetOverlap(t *testing.T) {
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "Included in or Includes or Overlaps", IsConceptRelation: boolPtr(true)},
	}
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta()); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if got := repo.tx.relations[0].rel; got != domain.RelationUncertain {
		t.Errorf("relation = %q, want the uncertain value (never overlaps)", got)
	}
}

func TestIngestCDMDropsMisappliedNameRowsAndSamplesThem(t *testing.T) {
	// Documented rule: conceptRelationship=false rows are NOT concept
	// relations. They are dropped from concept_relation — counted and
	// sampled, never silently.
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "is misapplied name for", IsConceptRelation: boolPtr(false), RelationshipUUID: "r-mis"},
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolPtr(true)},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.NonConcept != 1 {
		t.Errorf("NonConcept = %d, want 1", rep.NonConcept)
	}
	if len(rep.NonConceptSample) != 1 || !strings.Contains(rep.NonConceptSample[0], "r-mis") {
		t.Errorf("NonConceptSample = %v", rep.NonConceptSample)
	}
	if rep.RelationsWritten != 1 || len(repo.tx.relations) != 1 {
		t.Fatalf("wrote %d relations, want only the concept relation", len(repo.tx.relations))
	}
	if repo.tx.relations[0].rel == domain.RelationMisapplied {
		t.Error("a misapplied-name row must never reach concept_relation")
	}
}

func TestIngestCDMDropsMisappliedTypeEvenWhenFlagIsUnknown(t *testing.T) {
	// An empty is_concept_relation is UNKNOWN, not false — but a
	// misapplied-name TYPE is never a concept relation regardless of the
	// flag, so the type decides here.
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "is misapplied name for", RelationshipUUID: "r-u"},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.NonConcept != 1 || rep.RelationsWritten != 0 {
		t.Errorf("NonConcept=%d RelationsWritten=%d, want 1/0", rep.NonConcept, rep.RelationsWritten)
	}
}

func TestIngestCDMKeepsUnknownFlagRowsThatAreConceptRelations(t *testing.T) {
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "Includes", RelationshipUUID: "r-u"},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.RelationsWritten != 1 {
		t.Fatalf("RelationsWritten = %d, want 1 (unknown flag is not false)", rep.RelationsWritten)
	}
	if rep.UnknownFlag != 1 {
		t.Errorf("UnknownFlag = %d, want 1", rep.UnknownFlag)
	}
}

func TestIngestCDMKeepsProParteAsConceptRelation(t *testing.T) {
	// CDM flags pro-parte rows conceptRelationship=true: "A applies partly
	// to B" IS a statement about circumscriptions.
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "is pro parte synonym for", IsConceptRelation: boolPtr(true)},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.RelationsWritten != 1 || repo.tx.relations[0].rel != domain.RelationProParte {
		t.Fatalf("pro parte must be stored as a concept relation, got %+v", repo.tx.relations)
	}
}

func TestIngestCDMCountsAndSamplesUnresolvableRelationEndsWithoutAborting(t *testing.T) {
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "ghost", RelationType: "Congruent to", IsConceptRelation: boolPtr(true), RelationshipUUID: "r-1"},
		{FromUUID: "ghost", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolPtr(true), RelationshipUUID: "r-2"},
		{FromUUID: "bbb", ToUUID: "aaa", RelationType: "Congruent to", IsConceptRelation: boolPtr(true), RelationshipUUID: "r-3"},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("an unresolvable end must never abort the ingest: %v", err)
	}
	if rep.UnresolvedEnds != 2 {
		t.Errorf("UnresolvedEnds = %d, want 2", rep.UnresolvedEnds)
	}
	if len(rep.UnresolvedSample) == 0 {
		t.Error("unresolvable ends must be sampled, not just counted")
	}
	if rep.RelationsWritten != 1 || len(repo.tx.relations) != 1 {
		t.Errorf("wrote %d relations, want 1", len(repo.tx.relations))
	}
	if !repo.tx.committed {
		t.Error("the ingest must still commit")
	}
}

func TestIngestCDMResolvesEndsAgainstAlreadyIngestedConcepts(t *testing.T) {
	repo := newCDMRepo()
	repo.existing["cdm:concept:ccc"] = true
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "ccc", RelationType: "Congruent to", IsConceptRelation: boolPtr(true)},
	}
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.RelationsWritten != 1 {
		t.Errorf("RelationsWritten = %d, want 1 (end already in the database)", rep.RelationsWritten)
	}
}

func TestIngestCDMNeverReadsWhileTheIngestTransactionIsOpen(t *testing.T) {
	// SetMaxOpenConns(1): a read inside the write transaction is a real
	// deadlock. This already cost SP3 an escalation.
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "Congruent to", IsConceptRelation: boolPtr(true)},
	}
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta()); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if repo.readsAfterBegin != 0 {
		t.Fatalf("%d repository read(s) happened with the ingest transaction open", repo.readsAfterBegin)
	}
}

func TestIngestCDMStoresOnlyTheSourceDirectionOfIncludes(t *testing.T) {
	// Directionality decision: one canonical direction (the one the source
	// states), inverted at query time via domain.Relation.Inverse — never a
	// second, synthesized mirror row.
	repo := newCDMRepo()
	rels := []application.CDMRelationRow{
		{FromUUID: "aaa", ToUUID: "bbb", RelationType: "Includes", IsConceptRelation: boolPtr(true)},
	}
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta()); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if len(repo.tx.relations) != 1 {
		t.Fatalf("wrote %d relations, want exactly 1 (no mirror row)", len(repo.tx.relations))
	}
	w := repo.tx.relations[0]
	if w.rel != domain.RelationIncludes || w.from != "cdm:concept:aaa" {
		t.Errorf("stored %+v, want the source direction as includes", w)
	}
}

func TestIngestCDMResolvesParentOnlyWhenTheParentConceptExists(t *testing.T) {
	rows := twoSecsSameName()
	rows[1].ParentUUID = "aaa"
	rows = append(rows, application.CDMConceptRow{
		ConceptUUID: "ccc", ScientificName: "Abies", Rank: "Genus", Status: "Accepted",
		SecUUID: "sec-wh98", SecTitle: "Wisskirchen & Haeupler 1998", ParentUUID: "ghost",
	})
	repo := newCDMRepo()
	rep, err := application.IngestCDM(context.Background(), repo, rows, nil, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	byID := map[string]domain.Concept{}
	for _, c := range repo.tx.concepts {
		byID[c.ID] = c
	}
	if got := byID["cdm:concept:bbb"].ParentID; got != "cdm:concept:aaa" {
		t.Errorf("resolvable parent = %q, want cdm:concept:aaa", got)
	}
	if got := byID["cdm:concept:ccc"].ParentID; got != "" {
		t.Errorf("unresolvable parent = %q, want empty (FK would fail)", got)
	}
	if rep.UnresolvedParents != 1 {
		t.Errorf("UnresolvedParents = %d, want 1", rep.UnresolvedParents)
	}
}

func TestIngestCDMHandlesEmptyStatusAndExoticRanksExplicitly(t *testing.T) {
	rows := []application.CDMConceptRow{
		{ConceptUUID: "aaa", ScientificName: "Abies alba agg.", Rank: "Species Aggregate", Status: "", SecUUID: "s1", SecTitle: "One"},
		{ConceptUUID: "bbb", ScientificName: "Abies alba", Rank: "Species", Status: "Accepted", SecUUID: "s1", SecTitle: "One"},
		// A second exotic-rank row with the SAME verbatim spelling, so the
		// per-spelling tally is asserted as a COUNT and not merely as
		// presence, and a second non-empty status so EmptyStatus can tell
		// "empty" from "not empty".
		{ConceptUUID: "ccc", ScientificName: "Pinus abies agg.", Rank: "Species Aggregate", Status: "Accepted", SecUUID: "s1", SecTitle: "One"},
	}
	repo := newCDMRepo()
	rep, err := application.IngestCDM(context.Background(), repo, rows, nil, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	byID := map[string]domain.Concept{}
	for _, c := range repo.tx.concepts {
		byID[c.ID] = c
	}
	agg := byID["cdm:concept:aaa"]
	if agg.Rank != domain.RankOther {
		t.Errorf("exotic rank = %q, want OTHER", agg.Rank)
	}
	if agg.RankVerbatim != "Species Aggregate" {
		t.Errorf("rank verbatim = %q, want the raw spelling preserved", agg.RankVerbatim)
	}
	if agg.Status != domain.StatusUnknown {
		t.Errorf("empty status = %q, want UNKNOWN", agg.Status)
	}
	if rep.EmptyStatus != 1 {
		t.Errorf("EmptyStatus = %d, want 1", rep.EmptyStatus)
	}
	if rep.OtherRanks != 2 {
		t.Errorf("OtherRanks = %d, want 2", rep.OtherRanks)
	}
	if len(rep.OtherRankSample) != 1 || rep.OtherRankSample[0].Verbatim != "Species Aggregate" || rep.OtherRankSample[0].Count != 2 {
		t.Errorf("OtherRankSample = %+v, want one entry {Species Aggregate 2}", rep.OtherRankSample)
	}
}

func TestIngestCDMSkipsRowsWithoutAConceptUUID(t *testing.T) {
	rows := []application.CDMConceptRow{{ConceptUUID: "", ScientificName: "Nameless", Rank: "Species"}}
	repo := newCDMRepo()
	rep, err := application.IngestCDM(context.Background(), repo, rows, nil, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.ConceptsWritten != 0 || rep.SkippedConcepts != 1 {
		t.Errorf("written=%d skipped=%d, want 0/1", rep.ConceptsWritten, rep.SkippedConcepts)
	}
}

func TestIngestCDMReportsRedistribution(t *testing.T) {
	repo := newCDMRepo()
	rep, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), nil, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.Redistribution != string(domain.RedistributionUnknown) {
		t.Errorf("Redistribution = %q, want unknown", rep.Redistribution)
	}
	if rep.Backbone != "cdm" {
		t.Errorf("Backbone = %q", rep.Backbone)
	}
}

func TestIngestCDMDeduplicatesSecReferences(t *testing.T) {
	rows := []application.CDMConceptRow{
		{ConceptUUID: "aaa", ScientificName: "A a", Rank: "Species", Status: "Accepted", SecUUID: "s1", SecTitle: "One"},
		{ConceptUUID: "bbb", ScientificName: "B b", Rank: "Species", Status: "Accepted", SecUUID: "s1", SecTitle: "One"},
		{ConceptUUID: "ccc", ScientificName: "C c", Rank: "Species", Status: "Accepted", SecUUID: "", SecTitle: ""},
	}
	repo := newCDMRepo()
	rep, err := application.IngestCDM(context.Background(), repo, rows, nil, cdmMeta())
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if rep.SecReferences != 1 || len(repo.tx.secs) != 1 {
		t.Errorf("SecReferences=%d rows=%d, want 1/1", rep.SecReferences, len(repo.tx.secs))
	}
	if rep.ConceptsWithoutSec != 1 {
		t.Errorf("ConceptsWithoutSec = %d, want 1", rep.ConceptsWithoutSec)
	}
}

func TestIngestCDMPropagatesRepositoryFailures(t *testing.T) {
	for _, failOn := range []string{"sec", "name", "concept", "link", "relation", "finalize"} {
		repo := newCDMRepo()
		repo.tx.failOn = failOn
		rels := []application.CDMRelationRow{
			{FromUUID: "aaa", ToUUID: "bbb", RelationType: "Congruent to", IsConceptRelation: boolPtr(true)},
		}
		_, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta())
		if err == nil {
			t.Errorf("failOn=%s: want error", failOn)
			continue
		}
		if !repo.tx.rolled {
			t.Errorf("failOn=%s: transaction was not rolled back", failOn)
		}
	}
}

func TestIngestCDMPropagatesBeginAndResolveFailures(t *testing.T) {
	repo := newCDMRepo()
	repo.beginErr = errors.New("no tx")
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), nil, cdmMeta()); err == nil {
		t.Error("want error when BeginIngest fails")
	}

	repo = newCDMRepo()
	repo.existingErr = errors.New("no read")
	rels := []application.CDMRelationRow{{FromUUID: "aaa", ToUUID: "zzz", RelationType: "Congruent to"}}
	if _, err := application.IngestCDM(context.Background(), repo, twoSecsSameName(), rels, cdmMeta()); err == nil {
		t.Error("want error when the resolve phase fails")
	}
}

func boolPtr(b bool) *bool { return &b }
