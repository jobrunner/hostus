package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"

	"github.com/jobrunner/hostus/internal/application"
)

// TestCrosswalk_InulaHirtaResolvesToPentanemaViaTier2Homonym is an
// end-to-end regression for the tier-2 homonym tie-break documented on
// genuineBearerWinner (internal/application/match.go): "Inula hirta" is a
// homotypic synonym of Pentanema hirtum and a heterotypic synonym of
// Pentanema britannicum, accepted in neither concept — so a plain "several
// distinct concepts share this spelling" check would flag it ambiguous, but
// tier 2 (homotypic beats heterotypic/unknown) resolves it deterministically
// to Pentanema hirtum. match_homotypic_internal_test.go already pins
// classify() directly at the unit level with hand-built candidates; this
// test exercises the SAME scenario through the full MatchNames path against
// a real seeded repository, so the wiring between Repository.MatchExact,
// domain.ClassifyMatch and genuineBearerWinner is proven end to end, not
// just the tie-break function in isolation.
func TestCrosswalk_InulaHirtaResolvesToPentanemaViaTier2Homonym(t *testing.T) {
	repo := openMemoryRepo(t)
	hirtumConceptID := seedInulaHirtaHomonymPair(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Inula hirta"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.RequiresReview {
		t.Error("RequiresReview = true, want false (tier-2 homotypic tie-break resolves this deterministically)")
	}
	if r.ConceptID != hirtumConceptID {
		t.Errorf("ConceptID = %q, want %q (Pentanema hirtum, the homotypic bearer)", r.ConceptID, hirtumConceptID)
	}
}

// seedInulaHirtaHomonymPair ingests the two concepts
// TestCrosswalk_InulaHirtaResolvesToPentanemaViaTier2Homonym needs: Pentanema
// hirtum, where "Inula hirta" is a HOMOTYPIC synonym (the genuine
// name-bearer — same nomenclatural type, name moved aside), and Pentanema
// britannicum, where "Inula hirta" is a HETEROTYPIC synonym (a
// misapplication — different type, weaker claim on the name). Neither
// concept carries "Inula hirta" as its accepted name. Returns the hirtum
// concept's id.
func seedInulaHirtaHomonymPair(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-tier2", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	homotypic := true
	heterotypic := false

	hirtumAccepted := domain.Name{ID: "test-tier2:name:hirtum-accepted", Canonical: "Pentanema hirtum", Rank: domain.RankSpecies}
	hirtumSyn := domain.Name{ID: "test-tier2:name:hirtum-syn-inula", Canonical: "Inula hirta", Rank: domain.RankSpecies}
	hirtumConcept := domain.Concept{ID: "test-tier2:concept:hirtum", BackboneID: "test-tier2", AcceptedName: hirtumAccepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	mustUpsertLinkedName(t, tx, hirtumConcept, hirtumAccepted, "accepted", nil)
	mustLinkExtraName(t, tx, hirtumConcept.ID, hirtumSyn, "synonym", &homotypic)

	britAccepted := domain.Name{ID: "test-tier2:name:brit-accepted", Canonical: "Pentanema britannicum", Rank: domain.RankSpecies}
	britSyn := domain.Name{ID: "test-tier2:name:brit-syn-inula", Canonical: "Inula hirta", Rank: domain.RankSpecies}
	britConcept := domain.Concept{ID: "test-tier2:concept:britannicum", BackboneID: "test-tier2", AcceptedName: britAccepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	mustUpsertLinkedName(t, tx, britConcept, britAccepted, "accepted", nil)
	mustLinkExtraName(t, tx, britConcept.ID, britSyn, "synonym", &heterotypic)

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return hirtumConcept.ID
}

// mustUpsertLinkedName writes concept and its accepted (or otherwise
// primary) name in one call, failing the test on any error — the
// three-statement sequence (UpsertName, UpsertConcept, LinkName) every
// concept fixture in this file repeats for its first name.
func mustUpsertLinkedName(t *testing.T, tx output.IngestTx, concept domain.Concept, name domain.Name, role string, homotypic *bool) {
	t.Helper()
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName(%q): unexpected error: %v", name.ID, err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept(%q): unexpected error: %v", concept.ID, err)
	}
	if err := tx.LinkName(concept.ID, name.ID, role, homotypic); err != nil {
		t.Fatalf("LinkName(%q): unexpected error: %v", name.ID, err)
	}
}

// mustLinkExtraName writes and links a SECOND name (e.g. a synonym) onto an
// already-upserted concept, failing the test on any error.
func mustLinkExtraName(t *testing.T, tx output.IngestTx, conceptID string, name domain.Name, role string, homotypic *bool) {
	t.Helper()
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName(%q): unexpected error: %v", name.ID, err)
	}
	if err := tx.LinkName(conceptID, name.ID, role, homotypic); err != nil {
		t.Fatalf("LinkName(%q): unexpected error: %v", name.ID, err)
	}
}

// TestCrosswalk_SalsolaKaliAggregateEndToEndYieldsSubsetAgreement is an
// end-to-end regression for the aggregate cross-space agreement mechanism
// (Task 6/7/10): eurosl's "Salsola kali agg." carries one member, germansl's
// "Salsola kali s.l." carries that SAME member plus one additional taxon
// (S. tragus subsp. tragus) germansl resolves separately — the real-world
// case the plan's Step 2 draft described. eurosl's member set is therefore a
// strict SUBSET of germansl's, which is exactly domain.AgreementSubset (a ⊆
// b, with eurosl as "a") — see domain/agreement.go's Compute and
// concept_agreement_test.go's TestComputeConceptAgreement_
// DifferingMembersYieldsSuperset, which already pins this at the
// ComputeConceptAgreement level. This test instead drives the SAME fixture
// through the full serving path (MatchNames -> matchAggregate ->
// buildAggregateResolution -> Repository.ConceptAgreement), the mechanism an
// actual "Salsola kali agg." query exercises.
func TestCrosswalk_SalsolaKaliAggregateEndToEndYieldsSubsetAgreement(t *testing.T) {
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:salsola-kali-agg", "Salsola kali agg.",
		[]string{"wcvp:concept:salsola-kali-1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:salsola-kali-agg", "Salsola kali s.l.",
		[]string{"wcvp:concept:salsola-kali-1", "wcvp:concept:salsola-tragus-subsp-tragus"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if err := repo.WriteConceptAgreement(context.Background(), report.Pairs); err != nil {
		t.Fatalf("WriteConceptAgreement: unexpected error: %v", err)
	}

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Salsola kali agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.AggregateResolution == nil {
		t.Fatal("AggregateResolution = nil, want non-nil for an aggregate match")
	}
	if r.AggregateResolution.Agreement != domain.AgreementSubset {
		t.Errorf("Agreement = %q, want %q (eurosl's one member is a strict subset of germansl's two)", r.AggregateResolution.Agreement, domain.AgreementSubset)
	}
	if r.RequiresReview {
		t.Error("RequiresReview = true, want false (a resolvable aggregate alias, agreement info is advisory)")
	}
}

// TestCrosswalk_RubusFruticosusAggregateIsOneSidedInGermanSL is an
// end-to-end regression for the "only one name space knows this aggregate"
// case (domain.AgreementOneSided, mirroring concept_agreement_test.go's
// TestComputeConceptAgreement_OneSidedAggregateInEuroslOnly but for germansl
// as the one-sided space) — driven through the live MatchNames path rather
// than the batch ComputeConceptAgreement report.
//
// FINDING (Task 13): unlike the batch report, MatchNames' live
// AggregateResolution never surfaces domain.AgreementOneSided in its
// Agreement field. buildAggregateResolution (internal/application/match.go)
// only calls Repository.ConceptAgreement — the lookup that could return a
// stored one_sided pair — when BOTH eurosl's and germansl's per-space Status
// are AggregatePolicyKnown; a genuinely one-sided aggregate has exactly one
// side Known and the other Unresolvable, so that gate is never satisfied and
// Agreement stays "". The one-sidedness is still fully visible — just via
// AggregateResolution.Options[i].Status per name space, not via Agreement —
// which is what this test actually asserts. This is a real gap between the
// batch report's vocabulary and the live match response, not a copy error;
// see the Task 13 report for the full writeup.
func TestCrosswalk_RubusFruticosusAggregateIsOneSidedInGermanSL(t *testing.T) {
	repo := openMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "germansl:concept:rubus-fruticosus-agg", "Rubus fruticosus agg.",
		[]string{"wcvp:concept:rubus-fruticosus-1"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if err := repo.WriteConceptAgreement(context.Background(), report.Pairs); err != nil {
		t.Fatalf("WriteConceptAgreement: unexpected error: %v", err)
	}
	// Sanity-check the fixture: the BATCH report does classify this pair
	// AgreementOneSided (this is not itself a new assertion — see
	// TestComputeConceptAgreement_OneSidedAggregateInEuroslOnly — kept here
	// only to make the live-path gap below unambiguous: the DATA supports
	// "one_sided", the live endpoint just doesn't surface it that way).
	if len(report.Pairs) != 1 || report.Pairs[0].Agreement != domain.AgreementOneSided {
		t.Fatalf("fixture invalid: report.Pairs = %+v, want exactly one AgreementOneSided pair", report.Pairs)
	}

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Rubus fruticosus agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.AggregateResolution == nil {
		t.Fatal("AggregateResolution = nil, want non-nil for an aggregate match")
	}
	if r.AggregateResolution.RequestedNameSpace != "germansl" {
		t.Errorf("RequestedNameSpace = %q, want %q", r.AggregateResolution.RequestedNameSpace, "germansl")
	}
	if r.AggregateResolution.Status != domain.AggregatePolicyKnown {
		t.Errorf("Status = %q, want %q (germansl, the requested space, does know this aggregate)", r.AggregateResolution.Status, domain.AggregatePolicyKnown)
	}
	assertAggregateOptionStatus(t, r.AggregateResolution.Options, "eurosl", domain.AggregatePolicyUnresolvable)
	assertAggregateOptionStatus(t, r.AggregateResolution.Options, "germansl", domain.AggregatePolicyKnown)
	// The documented finding: the live path's Agreement stays empty even
	// though the batch report (asserted above) computed AgreementOneSided
	// for the exact same data.
	if r.AggregateResolution.Agreement != "" {
		t.Errorf("Agreement = %q, want empty (buildAggregateResolution's ConceptAgreement lookup only fires when BOTH spaces resolved Known — see this test's doc comment)", r.AggregateResolution.Agreement)
	}
}

// assertAggregateOptionStatus fails the test unless options contains ns with
// exactly the given Status — the per-name-space lookup+assert pair
// TestCrosswalk_RubusFruticosusAggregateIsOneSidedInGermanSL needs twice.
func assertAggregateOptionStatus(t *testing.T, options []domain.AggregateResolutionOption, ns string, want domain.AggregatePolicy) {
	t.Helper()
	for _, opt := range options {
		if opt.NameSpace == ns {
			if opt.Status != want {
				t.Errorf("%s option = %+v, want Status %q", ns, opt, want)
			}
			return
		}
	}
	t.Errorf("no %q option in AggregateResolution.Options, want Status %q", ns, want)
}

// hasProvenance reports whether conceptID's classification (Family/
// OrderName/ClassName) can be traced to a source row: either a
// name_space_entry attached to it (Fall A — the crosswalk that actually
// writes classification today, see writeNameSpaceRow/UpsertClassification
// in namespace_ingest.go) or a Fall-B concept whose OWN backbone
// (eurosl/germansl) is the source of its classification.
func hasProvenance(t *testing.T, repo output.Repository, concept *domain.Concept) bool {
	t.Helper()
	if concept.BackboneID == "eurosl" || concept.BackboneID == "germansl" {
		return true
	}
	entries, err := repo.NameSpaceEntries(context.Background(), concept.ID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries(%q): unexpected error: %v", concept.ID, err)
	}
	return len(entries) > 0
}

// TestClassification_EveryValueTracesToASourceRow is the "no fabrication"
// sample (Task 13 Step 3, spec §11 correctness test 3): every concept this
// test finds with Family/OrderName/ClassName set must be traceable to an
// actual source row, never a value hostus invented. It samples concepts
// classified through the real Fall-A path (IngestNameSpace against
// seededMatchRepo's WCVP fixture, the same mechanism
// TestIngestNameSpace_WritesClassificationOntoMatchedConcept already pins
// for one concept) and asserts hasProvenance for each of TWO independently
// classified concepts, rather than just one, so the check is exercised as a
// genuine sample rather than a single coincidence.
//
// FINDING (Task 13): the brief's "OR a Fall-B concept (backbone_id in
// {eurosl,germansl})" branch is exercised by hasProvenance above for
// completeness, but it is CURRENTLY DEAD CODE against this codebase's real
// ingest paths: ingestTx.UpsertConcept's INSERT statement
// (internal/adapters/sqlite/db.go) does not even include the
// family/order_name/class_name columns, and nativespace_ingest.go (Fall B's
// ingest) never calls UpsertClassification — only namespace_ingest.go's
// Fall-A crosswalk does, always in the same call as AddNameSpaceEntry for
// the same concept ID (see writeNameSpaceRow). So today, EVERY concept with
// classification set is structurally guaranteed to also have a
// name_space_entry; a native eurosl/germansl concept cannot currently carry
// Family/OrderName/ClassName at all. This is a real, currently-true
// invariant (stronger than the brief assumed), not a test gap — see the
// Task 13 report.
func TestClassification_EveryValueTracesToASourceRow(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{
		{Taxon: "Festuca ovina", SourceID: "1408c0e8", Status: "accepted",
			Family: "Poaceae", OrderName: "Poales", ClassName: "Liliopsida"},
		{Taxon: "Senecio jacobaea", SourceID: "deadbeef", Status: "accepted",
			Family: "Asteraceae", OrderName: "Asterales", ClassName: "Magnoliopsida"},
	}
	report, err := application.IngestNameSpace(ctx, repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if report.Matched != 2 {
		t.Fatalf("report.Matched = %d, want 2", report.Matched)
	}

	jacobaeaVulgarisConceptID := "wcvp:concept:3082777"
	for _, id := range []string{festucaOvinaConceptID, jacobaeaVulgarisConceptID} {
		concept, _, _, _, err := repo.Concept(ctx, id)
		if err != nil {
			t.Fatalf("Concept(%q): unexpected error: %v", id, err)
		}
		if concept.Family == "" && concept.OrderName == "" && concept.ClassName == "" {
			t.Fatalf("concept %q has no classification set at all; test fixture is not exercising the invariant", id)
		}
		if !hasProvenance(t, repo, concept) {
			t.Errorf("concept %q has classification (family=%q order=%q class=%q) but no traceable source row", id, concept.Family, concept.OrderName, concept.ClassName)
		}
	}
}
