package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// fakeTranslateRepo is a minimal output.Repository stub for exercising
// application.Translate's own logic (request validation, hop boundary,
// direction handling, the no-relation answer, the opt-in name block)
// without a backing store. It embeds a nil output.Repository so an
// unanticipated method call panics rather than silently returning zeroes —
// the same discipline as fakeSuggestRepo.
type fakeTranslateRepo struct {
	output.Repository

	secs      map[string]domain.SecReference
	concepts  map[string]domain.Concept
	edges     map[string][]output.ConceptRelationEdge // keyed conceptID + "|" + targetSec
	exact     map[string][]output.MatchCandidate
	fuzzy     []output.MatchCandidate
	edgesErr  error
	gotTarget string
}

func (f *fakeTranslateRepo) SecReferenceByID(_ context.Context, id string) (domain.SecReference, error) {
	if s, ok := f.secs[id]; ok {
		return s, nil
	}
	return domain.SecReference{}, fmt.Errorf("sec %q: %w", id, domain.ErrNotFound)
}

func (f *fakeTranslateRepo) ConceptRelationsInSec(_ context.Context, conceptID, targetSec string) (output.ConceptRelations, error) {
	f.gotTarget = targetSec
	if f.edgesErr != nil {
		return output.ConceptRelations{}, f.edgesErr
	}
	c, ok := f.concepts[conceptID]
	if !ok {
		return output.ConceptRelations{}, fmt.Errorf("concept %q: %w", conceptID, domain.ErrNotFound)
	}
	return output.ConceptRelations{Source: c, Edges: f.edges[conceptID+"|"+targetSec]}, nil
}

func (f *fakeTranslateRepo) MatchExact(_ context.Context, canon string) ([]output.MatchCandidate, error) {
	return f.exact[canon], nil
}

func (f *fakeTranslateRepo) MatchFuzzyCandidates(context.Context, string, int, string, string) ([]output.MatchCandidate, error) {
	return f.fuzzy, nil
}

// --- fixtures --------------------------------------------------------------

const (
	secRothmaler = "sec-rothmaler"
	secWH98      = "sec-wh98"
)

func conceptIn(id, canonical, sec string) domain.Concept {
	return domain.Concept{
		ID:           id,
		BackboneID:   "cdm",
		Rank:         domain.RankSpecies,
		SecReference: sec,
		Status:       domain.StatusAccepted,
		AcceptedName: domain.Name{ID: id + ":name", Canonical: canonical, Authorship: "Mill.", Rank: domain.RankSpecies},
	}
}

// translateRepo wires the UC6 shape: "Abies alba sec. Rothmaler" and
// "Abies alba sec. Wisskirchen & Haeupler 1998" as two DISTINCT concepts.
func translateRepo() *fakeTranslateRepo {
	src := conceptIn("cdm:concept:roth", "Abies alba", secRothmaler)
	dst := conceptIn("cdm:concept:wh98", "Abies alba", secWH98)
	repo := &fakeTranslateRepo{
		secs: map[string]domain.SecReference{
			secRothmaler: {ID: secRothmaler, Title: "Rothmaler, Exkursionsflora, 8. Aufl."},
			secWH98:      {ID: secWH98, Title: "Wisskirchen & Haeupler 1998: Standardliste"},
		},
		concepts: map[string]domain.Concept{src.ID: src, dst.ID: dst},
		edges:    map[string][]output.ConceptRelationEdge{},
		exact: map[string][]output.MatchCandidate{
			domain.Canonicalize("Abies alba"): {
				{Concept: src, MatchedName: src.AcceptedName, Role: "accepted"},
				{Concept: dst, MatchedName: dst.AcceptedName, Role: "accepted"},
			},
		},
	}
	return repo
}

func edgeTo(rel domain.Relation, outgoing bool, repo *fakeTranslateRepo) output.ConceptRelationEdge {
	dst := repo.concepts["cdm:concept:wh98"]
	return output.ConceptRelationEdge{
		Partner:    dst,
		PartnerSec: repo.secs[secWH98],
		Relation:   rel,
		Outgoing:   outgoing,
		Source:     "cdm",
	}
}

func translate(t *testing.T, repo *fakeTranslateRepo, req application.TranslateRequest) application.TranslateResult {
	t.Helper()
	res, err := application.Translate(context.Background(), repo, req)
	if err != nil {
		t.Fatalf("Translate: unexpected error: %v", err)
	}
	return res
}

func byConceptID(t *testing.T) application.TranslateRequest {
	t.Helper()
	return application.TranslateRequest{ConceptID: "cdm:concept:roth", TargetSec: secWH98}
}

// --- tests -----------------------------------------------------------------

// TestTranslateCongruentIsTheOnlyEquality is the endpoint's first
// non-negotiable, exercised through the whole use case rather than only on
// domain.Relation: a congruent edge may read as identity, and every other
// relation must arrive marked as not-identity, with the reason in the
// payload.
func TestTranslateCongruentIsTheOnlyEquality(t *testing.T) {
	cases := []struct {
		rel          domain.Relation
		wantEquality bool
	}{
		{domain.RelationCongruent, true},
		{domain.RelationNotCongruent, false},
		{domain.RelationIncludes, false},
		{domain.RelationIncludedIn, false},
		{domain.RelationOverlaps, false},
		{domain.RelationUncertain, false},
		{domain.RelationProParte, false},
	}
	for _, tc := range cases {
		repo := translateRepo()
		repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(tc.rel, true, repo)}

		res := translate(t, repo, byConceptID(t))
		if len(res.Candidates) != 1 {
			t.Fatalf("%s: got %d candidates, want 1", tc.rel, len(res.Candidates))
		}
		got := res.Candidates[0]
		if got.Relation != tc.rel {
			t.Errorf("%s: Relation = %q, want %q", tc.rel, got.Relation, tc.rel)
		}
		if got.IsEquality != tc.wantEquality {
			t.Errorf("%s: IsEquality = %v, want %v", tc.rel, got.IsEquality, tc.wantEquality)
		}
		if tc.wantEquality && got.Note != "" {
			t.Errorf("%s: Note = %q, want empty for an identity result", tc.rel, got.Note)
		}
		if !tc.wantEquality && got.Note == "" {
			t.Errorf("%s: Note is empty — a non-identity relation must say so in the payload", tc.rel)
		}
	}
}

// TestTranslateUncertainIsNotFlattenedToOverlaps guards the decision Task 1
// made and Task 3 stored: ⊂⊃⊕ keeps its own relation value all the way to
// the response, and its note names the undecidedness rather than the
// generic "not equal" text.
func TestTranslateUncertainIsNotFlattenedToOverlaps(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationUncertain, true, repo)}

	res := translate(t, repo, byConceptID(t))
	got := res.Candidates[0]
	if got.Relation != domain.RelationUncertain {
		t.Fatalf("Relation = %q, want %q", got.Relation, domain.RelationUncertain)
	}
	if got.RelationFromSource != domain.RelationUncertain {
		t.Errorf("RelationFromSource = %q, want %q", got.RelationFromSource, domain.RelationUncertain)
	}

	// The note must name the undecidedness itself, not merely differ from
	// the generic one: a swapped pair of notes would still "differ".
	if !strings.Contains(got.Note, "Unbestimmt") || !strings.Contains(got.Note, "legt sich nicht fest") {
		t.Errorf("uncertain note = %q, want the wording that the source does not commit", got.Note)
	}

	overlaps := translateRepo()
	overlaps.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationOverlaps, true, overlaps)}
	other := translate(t, overlaps, byConceptID(t)).Candidates[0]
	if strings.Contains(other.Note, "Unbestimmt") {
		t.Errorf("overlaps note = %q, want the plain non-identity wording, not the undecided one", other.Note)
	}
	if got.Note == other.Note {
		t.Errorf("uncertain and overlaps carry the same note %q — the undecided case must be distinguishable", got.Note)
	}
}

// TestTranslateIncomingEdgeIsInverted checks the directionality contract:
// hostus stores only the direction the source states, so an incoming
// "includes" must arrive as the stored statement PLUS a source-first
// reading of "included_in" — and the two must be distinguishable.
func TestTranslateIncomingEdgeIsInverted(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationIncludes, false, repo)}

	got := translate(t, repo, byConceptID(t)).Candidates[0]
	if got.Outgoing {
		t.Errorf("Outgoing = true, want false for a stored partner -> source row")
	}
	if got.Relation != domain.RelationIncludes {
		t.Errorf("Relation = %q, want the stored %q", got.Relation, domain.RelationIncludes)
	}
	if got.RelationFromSource != domain.RelationIncludedIn {
		t.Errorf("RelationFromSource = %q, want %q", got.RelationFromSource, domain.RelationIncludedIn)
	}
	if got.StatementFrom != "cdm:concept:wh98" || got.StatementTo != "cdm:concept:roth" {
		t.Errorf("statement = %q -> %q, want wh98 -> roth (the stored order)", got.StatementFrom, got.StatementTo)
	}
}

// TestTranslateOutgoingEdgeKeepsItsOrder is the counterpart: an outgoing
// row reads source -> partner and is not inverted.
func TestTranslateOutgoingEdgeKeepsItsOrder(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationIncludes, true, repo)}

	got := translate(t, repo, byConceptID(t)).Candidates[0]
	if !got.Outgoing {
		t.Errorf("Outgoing = false, want true")
	}
	if got.RelationFromSource != domain.RelationIncludes {
		t.Errorf("RelationFromSource = %q, want %q", got.RelationFromSource, domain.RelationIncludes)
	}
	if got.StatementFrom != "cdm:concept:roth" || got.StatementTo != "cdm:concept:wh98" {
		t.Errorf("statement = %q -> %q, want roth -> wh98", got.StatementFrom, got.StatementTo)
	}
}

// TestTranslateIncomingProParteHasNoSourceFirstReading pins the case where
// inverting would invent a claim: pro parte is a directed assertion about
// the from-side NAME, so an incoming one gets no RelationFromSource at all,
// and the response says why instead of leaving the gap unexplained.
func TestTranslateIncomingProParteHasNoSourceFirstReading(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationProParte, false, repo)}

	got := translate(t, repo, byConceptID(t)).Candidates[0]
	if got.RelationFromSource != "" {
		t.Errorf("RelationFromSource = %q, want empty — pro parte has no inverse", got.RelationFromSource)
	}
	if got.Relation != domain.RelationProParte {
		t.Errorf("Relation = %q, want the stored %q", got.Relation, domain.RelationProParte)
	}
	// Non-empty is not enough: candidateNote(pro_parte) already returns the
	// generic non-identity text, so dropping the noteNoInverse assignment
	// would leave a plausible-but-wrong note. The note must NAME the
	// missing inverse.
	if !strings.Contains(got.Note, "keine Umkehrrichtung definiert") {
		t.Errorf("Note = %q, want it to name the missing inverse", got.Note)
	}
	if got.IsEquality {
		t.Errorf("IsEquality = true for pro parte")
	}
}

// TestTranslateTwoRelationsOnOnePairAreBothReturned: the concept_relation PK
// is (from, to, relation, source), so one pair can legitimately carry two
// relation types. Collapsing them to one candidate would throw away exactly
// the information UC6 asks for.
func TestTranslateTwoRelationsOnOnePairAreBothReturned(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{
		edgeTo(domain.RelationCongruent, true, repo),
		edgeTo(domain.RelationOverlaps, true, repo),
	}

	res := translate(t, repo, byConceptID(t))
	if len(res.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2 (both relation types on the same pair)", len(res.Candidates))
	}
	// Asserting the two VALUES, not merely that they differ: a swap would
	// satisfy a difference check while mislabelling both edges.
	if res.Candidates[0].Relation != domain.RelationCongruent {
		t.Errorf("first candidate = %q, want congruent", res.Candidates[0].Relation)
	}
	if res.Candidates[1].Relation != domain.RelationOverlaps {
		t.Errorf("second candidate = %q, want overlaps", res.Candidates[1].Relation)
	}
	if res.Candidates[0].IsEquality == res.Candidates[1].IsEquality {
		t.Errorf("both candidates report is_equality %v — the congruent/overlaps distinction was lost",
			res.Candidates[0].IsEquality)
	}
}

// TestTranslateNoRelationIsAnExplicitEmptyAnswer is the second
// non-negotiable. The repo deliberately DOES hold a same-named concept in
// the target space (translateRepo's MatchExact returns it) — and the
// default answer must still be empty, with an explanatory note, and with no
// name guess anywhere in it.
func TestTranslateNoRelationIsAnExplicitEmptyAnswer(t *testing.T) {
	repo := translateRepo()

	res := translate(t, repo, byConceptID(t))
	if res.HasRelation() {
		t.Fatalf("HasRelation() = true, want false")
	}
	if len(res.Candidates) != 0 {
		t.Errorf("got %d candidates, want 0", len(res.Candidates))
	}
	if len(res.NameCandidates) != 0 {
		t.Errorf("got %d name candidates without opting in, want 0 — a name match is not a translation", len(res.NameCandidates))
	}
	if res.Note == "" {
		t.Errorf("Note is empty — an empty answer must say that absence of a record is not absence of a relation")
	}
	if res.TargetSec.Title == "" {
		t.Errorf("TargetSec.Title is empty — an empty answer must still name the space it looked in")
	}
}

// TestTranslateNameCandidatesAreOptInAndLabelled: when the caller
// explicitly asks, same-name concepts come back in their OWN field, each
// requiring review, and the result as a whole requires review. They are
// never merged into Candidates.
func TestTranslateNameCandidatesAreOptInAndLabelled(t *testing.T) {
	repo := translateRepo()
	req := byConceptID(t)
	req.IncludeNameCandidates = true

	res := translate(t, repo, req)
	if len(res.Candidates) != 0 {
		t.Fatalf("got %d relation candidates, want 0", len(res.Candidates))
	}
	if len(res.NameCandidates) != 1 {
		t.Fatalf("got %d name candidates, want 1", len(res.NameCandidates))
	}
	if got := res.NameCandidates[0].Concept.ID; got != "cdm:concept:wh98" {
		t.Errorf("name candidate = %q, want cdm:concept:wh98", got)
	}
	if !res.NameCandidates[0].RequiresReview {
		t.Errorf("name candidate RequiresReview = false, want true")
	}
	if !res.RequiresReview {
		t.Errorf("result RequiresReview = false, want true when a name block is present")
	}
}

// TestTranslateNameCandidatesOptInWithNothingToShow: opting in when the
// target space holds no same-name concept must NOT flip the result to
// "requires review" or attach the name-block note. An empty hint is not a
// hint, and the two no-relation notes must stay distinguishable.
func TestTranslateNameCandidatesOptInWithNothingToShow(t *testing.T) {
	repo := translateRepo()
	// The source's OWN space holds no other same-name concept.
	req := application.TranslateRequest{
		ConceptID: "cdm:concept:roth", TargetSec: secRothmaler, IncludeNameCandidates: true,
	}

	res := translate(t, repo, req)
	if len(res.NameCandidates) != 0 {
		t.Fatalf("got %d name candidates, want 0", len(res.NameCandidates))
	}
	if res.RequiresReview {
		t.Errorf("RequiresReview = true with an empty name block, want false")
	}

	withNames := translate(t, translateRepo(), application.TranslateRequest{
		ConceptID: "cdm:concept:roth", TargetSec: secWH98, IncludeNameCandidates: true,
	})
	if !strings.Contains(withNames.Note, "KEINE Übersetzung") {
		t.Errorf("name-block note = %q, want it to say plainly that this is not a translation", withNames.Note)
	}
	if strings.Contains(res.Note, "KEINE Übersetzung") {
		t.Errorf("empty-block note = %q, want the plain no-relation wording only", res.Note)
	}
	if res.Note == withNames.Note {
		t.Errorf("note %q is the same with and without name candidates", res.Note)
	}
	if res.Note == "" {
		t.Errorf("the plain no-relation note is missing")
	}
}

// TestTranslateNameCandidatesAreSuppressedWhenARelationExists: the block is
// a last resort for the empty case only. If a real relation was found, no
// name guess may ride along beside it.
func TestTranslateNameCandidatesAreSuppressedWhenARelationExists(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationCongruent, true, repo)}
	req := byConceptID(t)
	req.IncludeNameCandidates = true

	res := translate(t, repo, req)
	if len(res.NameCandidates) != 0 {
		t.Errorf("got %d name candidates alongside a real relation, want 0", len(res.NameCandidates))
	}
	if res.RequiresReview {
		t.Errorf("RequiresReview = true for a plain congruent answer")
	}
}

// TestTranslateRefusesMoreThanOneHop enforces the third non-negotiable at
// the request boundary: a caller asking for a chain gets a named refusal
// rather than a one-hop answer it might read as deeper.
func TestTranslateRefusesMoreThanOneHop(t *testing.T) {
	for _, hops := range []int{2, 3, -1} {
		repo := translateRepo()
		req := byConceptID(t)
		req.MaxHops = hops
		_, err := application.Translate(context.Background(), repo, req)
		if !errors.Is(err, application.ErrMultiHopUnsupported) {
			t.Errorf("MaxHops=%d: error = %v, want ErrMultiHopUnsupported", hops, err)
		}
	}
	repo := translateRepo()
	req := byConceptID(t)
	req.MaxHops = application.MaxTranslateHops
	if _, err := application.Translate(context.Background(), repo, req); err != nil {
		t.Errorf("MaxHops=%d: unexpected error: %v", application.MaxTranslateHops, err)
	}
}

// TestTranslateReportsOneHopOnEveryCandidate: the boundary is not only
// enforced, it is stated — both on the result and on each candidate, so a
// consumer never has to assume the depth it got.
func TestTranslateReportsOneHopOnEveryCandidate(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationCongruent, true, repo)}

	res := translate(t, repo, byConceptID(t))
	if res.MaxHops != 1 {
		t.Errorf("MaxHops = %d, want 1", res.MaxHops)
	}
	if res.Candidates[0].Hops != 1 {
		t.Errorf("candidate Hops = %d, want 1", res.Candidates[0].Hops)
	}
}

func TestTranslateUnknownConceptIDIsNotFound(t *testing.T) {
	repo := translateRepo()
	_, err := application.Translate(context.Background(), repo, application.TranslateRequest{
		ConceptID: "cdm:concept:nope", TargetSec: secWH98,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want domain.ErrNotFound", err)
	}
}

// TestTranslateUnknownTargetSpaceIsNotFound keeps a typo'd target space
// distinguishable from a real space with nothing in it — the empty answer
// must never absorb a client error.
func TestTranslateUnknownTargetSpaceIsNotFound(t *testing.T) {
	repo := translateRepo()
	_, err := application.Translate(context.Background(), repo, application.TranslateRequest{
		ConceptID: "cdm:concept:roth", TargetSec: "sec-does-not-exist",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want domain.ErrNotFound", err)
	}
}

func TestTranslateUnresolvableVerbatimName(t *testing.T) {
	repo := translateRepo()
	_, err := application.Translate(context.Background(), repo, application.TranslateRequest{
		Verbatim: "Quercus nonexistens", TargetSec: secWH98,
	})
	if !errors.Is(err, application.ErrUnresolvableName) {
		t.Errorf("error = %v, want ErrUnresolvableName", err)
	}
}

// TestTranslateAmbiguousVerbatimNameIsUnresolvable: "Abies alba" exists in
// BOTH spaces, so an exact match is ambiguous across two distinct concepts.
// MatchNames refuses to pick one, and Translate must not pick one either.
func TestTranslateAmbiguousVerbatimNameIsUnresolvable(t *testing.T) {
	repo := translateRepo()
	_, err := application.Translate(context.Background(), repo, application.TranslateRequest{
		Verbatim: "Abies alba", TargetSec: secWH98,
	})
	if !errors.Is(err, application.ErrUnresolvableName) {
		t.Errorf("error = %v, want ErrUnresolvableName for a name in two sec. spaces", err)
	}
}

// TestTranslateFuzzyVerbatimEntryRequiresReview: entry by name reuses
// /v1/match's resolution with the same discipline — a fuzzy hit ALWAYS
// requires review, and the entry record says it was fuzzy.
func TestTranslateFuzzyVerbatimEntryRequiresReview(t *testing.T) {
	repo := translateRepo()
	src := repo.concepts["cdm:concept:roth"]
	// Only the source concept is fuzzy-reachable, so the fuzzy hit resolves
	// to exactly one concept rather than tying across the two spaces.
	repo.fuzzy = []output.MatchCandidate{{Concept: src, MatchedName: src.AcceptedName, Role: "accepted"}}
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationCongruent, true, repo)}

	res := translate(t, repo, application.TranslateRequest{Verbatim: "Abies albba", TargetSec: secWH98})
	if !res.RequiresReview {
		t.Errorf("RequiresReview = false, want true for a fuzzy entry")
	}
	if res.Entry.Mode != application.EntryModeVerbatim {
		t.Errorf("Entry.Mode = %q, want %q", res.Entry.Mode, application.EntryModeVerbatim)
	}
	if res.Entry.MatchType != domain.MatchFuzzy {
		t.Errorf("Entry.MatchType = %q, want %q", res.Entry.MatchType, domain.MatchFuzzy)
	}
	if res.Entry.Verbatim != "Abies albba" {
		t.Errorf("Entry.Verbatim = %q, want the query as sent", res.Entry.Verbatim)
	}
	// The result-level flag is PROPAGATED from the match result, not
	// re-derived from MatchType — the two must agree.
	if !res.Entry.RequiresReview {
		t.Errorf("Entry.RequiresReview = false, want the match result's own flag")
	}
	if res.Entry.RequiresReview != res.RequiresReview {
		t.Errorf("Entry.RequiresReview = %v but result RequiresReview = %v", res.Entry.RequiresReview, res.RequiresReview)
	}
	// A fuzzy ENTRY must not turn the relation itself into a guess.
	if len(res.Candidates) != 1 || !res.Candidates[0].IsEquality {
		t.Errorf("candidates = %+v, want the one congruent relation", res.Candidates)
	}
}

// TestTranslateConceptIDEntryDoesNotRequireReview is the counterpart: an
// exact entry plus a congruent relation is the one case that may be read
// as a plain answer.
func TestTranslateConceptIDEntryDoesNotRequireReview(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationCongruent, true, repo)}

	res := translate(t, repo, byConceptID(t))
	if res.RequiresReview {
		t.Errorf("RequiresReview = true, want false")
	}
	if res.Entry.Mode != application.EntryModeConceptID {
		t.Errorf("Entry.Mode = %q, want %q", res.Entry.Mode, application.EntryModeConceptID)
	}
	if res.SourceSec.Title == "" {
		t.Errorf("SourceSec.Title is empty — the answer must name the space it translated FROM")
	}
}

func TestTranslateRejectsMalformedRequests(t *testing.T) {
	cases := map[string]application.TranslateRequest{
		"neither concept_id nor verbatim": {TargetSec: secWH98},
		"both concept_id and verbatim":    {ConceptID: "cdm:concept:roth", Verbatim: "Abies alba", TargetSec: secWH98},
		"no target space":                 {ConceptID: "cdm:concept:roth"},
	}
	for name, req := range cases {
		_, err := application.Translate(context.Background(), translateRepo(), req)
		if !errors.Is(err, application.ErrInvalidTranslateRequest) {
			t.Errorf("%s: error = %v, want ErrInvalidTranslateRequest", name, err)
		}
	}
}

// TestTranslateQueriesTheRequestedTargetSpace pins that the target space
// actually reaches the repository — an implementation that ignored it
// would return the source's relations into every space.
func TestTranslateQueriesTheRequestedTargetSpace(t *testing.T) {
	repo := translateRepo()
	translate(t, repo, byConceptID(t))
	if repo.gotTarget != secWH98 {
		t.Errorf("repo queried target %q, want %q", repo.gotTarget, secWH98)
	}
}

func TestTranslateSurfacesRepositoryErrors(t *testing.T) {
	repo := translateRepo()
	sentinel := errors.New("boom")
	repo.edgesErr = sentinel
	if _, err := application.Translate(context.Background(), repo, byConceptID(t)); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want the repository error", err)
	}
}
