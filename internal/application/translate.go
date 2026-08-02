package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// MaxTranslateHops is the number of concept_relation edges /v1/translate is
// allowed to traverse: exactly one, always, with no request parameter that
// can raise it.
//
// A transitive chain is not sound across the measured relation vocabulary.
// "congruent ∘ includes" is defensible; "overlaps ∘ overlaps" says nothing
// at all (two circumscriptions that each overlap a third may be disjoint),
// and "uncertain ∘ anything" is undefined by construction — the source
// explicitly refused to say which of ⊂/⊃/⊕ it meant. Since the sound
// composition rule is relation-pair-specific and no source in hostus states
// one, hostus does not compose. TranslateRequest.MaxHops exists only so a
// caller asking for more gets a NAMED refusal (ErrMultiHopUnsupported)
// rather than silently receiving a one-hop answer it believes is deeper.
const MaxTranslateHops = 1

// Errors /v1/translate distinguishes. They map onto the error contract's
// codes: ErrInvalidTranslateRequest/ErrMultiHopUnsupported -> INVALID_QUERY,
// ErrUnresolvableName -> UNRESOLVABLE, and a wrapped domain.ErrNotFound ->
// NOT_FOUND (unknown concept id or unknown target sec. space).
var (
	// ErrInvalidTranslateRequest reports a request that names neither or
	// both of ConceptID/Verbatim, or no target space.
	ErrInvalidTranslateRequest = errors.New("application: translate needs exactly one of concept_id/verbatim plus a target space")
	// ErrMultiHopUnsupported reports a request for more than
	// MaxTranslateHops hops.
	ErrMultiHopUnsupported = errors.New("application: translate follows exactly one relation hop")
	// ErrUnresolvableName reports a verbatim entry name that /v1/match's
	// resolution could not pin to a single concept.
	ErrUnresolvableName = errors.New("application: verbatim name is not resolvable to a concept")
)

// Entry modes echoed back so a response says how its source concept was
// found.
const (
	EntryModeConceptID = "concept_id"
	EntryModeVerbatim  = "verbatim"
)

// German notes carried on a TranslateResult / TranslationCandidate.
const (
	// noteNoRelation is the whole point of the empty answer: absence of a
	// recorded relation is not evidence that none exists.
	noteNoRelation = "Keine erfasste Relation in den Zielreferenzraum. Das bedeutet NICHT, dass keine Beziehung besteht — nur, dass keine Quelle eine erfasst hat."
	// noteNotEquality is attached to every candidate whose relation is not
	// congruent, in the response itself rather than only in the docs.
	noteNotEquality = "Keine Gleichsetzung: Diese Relation sagt nicht, dass es sich um dasselbe Taxon handelt."
	// noteUncertain sharpens noteNotEquality for ⊂⊃⊕, where the source
	// explicitly declined to say which of the three relations holds.
	noteUncertain = "Unbestimmt: Die Quelle legt sich nicht fest, ob enthalten in, enthält oder überlappt — keine Gleichsetzung."
	// noteNoInverse is attached when the stored statement points AT the
	// source concept and has no meaningful inverse (pro parte), so the
	// response must not phrase it source-first.
	noteNoInverse = "Gerichtete Aussage über den Namen der Gegenseite; keine Umkehrrichtung definiert."
	// noteNameCandidates labels the opt-in name-based block. It is not a
	// translation and says so.
	noteNameCandidates = "Namensgleiche Konzepte im Zielraum — KEINE Übersetzung: keine Quelle behauptet eine Beziehung zwischen ihnen. Nur zur manuellen Prüfung."
)

// TranslateRequest asks for the counterparts of one concept in another sec.
// reference space. Exactly one of ConceptID and Verbatim must be set.
type TranslateRequest struct {
	// ConceptID is a hostus concept id (e.g. "cdm:concept:<uuid>").
	ConceptID string
	// Verbatim is a name to resolve first, through the same resolution
	// /v1/match uses — including its discipline that a fuzzy hit ALWAYS
	// sets RequiresReview.
	Verbatim string
	// TargetSec is the id of the sec. reference space to translate into.
	TargetSec string
	// MaxHops, when > 0, must be MaxTranslateHops; anything else is
	// refused with ErrMultiHopUnsupported. 0 means "the default", which is
	// also MaxTranslateHops.
	MaxHops int
	// IncludeNameCandidates opts into the separately labeled, explicitly
	// NON-translation block of same-name concepts in the target space (see
	// TranslateResult.NameCandidates). Off by default, because a name match
	// across two sec. spaces is exactly the conflation UC6 exists to
	// prevent.
	IncludeNameCandidates bool
}

// TranslateEntry records how the source concept was reached, so a caller
// can see whether it is looking at an answer for the concept it meant.
type TranslateEntry struct {
	Mode       string
	Verbatim   string
	MatchType  domain.MatchType
	Confidence float64
	Note       string
}

// TranslationCandidate is one concept in the target space that a stored
// relation connects to the source concept.
//
// It never claims equality on its own: IsEquality is the single field a
// consumer may read as "same taxon", and it is true for exactly
// domain.RelationCongruent (see domain.Relation.IsEquality).
type TranslationCandidate struct {
	Concept domain.Concept
	Sec     domain.SecReference
	// Relation is the relation exactly as the source states it, together
	// with StatementFrom/StatementTo — the ordered pair it applies to.
	// It is never silently re-pointed.
	Relation      domain.Relation
	StatementFrom string
	StatementTo   string
	// Outgoing is true when the stored statement reads source -> candidate.
	Outgoing bool
	// RelationFromSource is the same edge read source-first. For an
	// outgoing edge it equals Relation; for an incoming one it is
	// Relation.Inverse() — and it is EMPTY when no inverse exists
	// (pro_parte), because naming one would invent a claim.
	RelationFromSource domain.Relation
	// IsEquality mirrors domain.Relation.IsEquality for the edge. It is
	// direction-independent (congruent is its own inverse).
	IsEquality bool
	Hops       int
	Source     string
	Note       string
}

// NameCandidate is a concept in the target space that merely SHARES the
// source concept's canonical name. It carries no relation, deliberately: no
// source asserts one. It is only ever produced on explicit opt-in and
// always requires review.
type NameCandidate struct {
	Concept        domain.Concept
	Sec            domain.SecReference
	RequiresReview bool
}

// TranslateResult is the derived sec_inference structure (architecture spec
// §4.3: derived, never a persisted table).
type TranslateResult struct {
	Source     domain.Concept
	SourceSec  domain.SecReference
	TargetSec  domain.SecReference
	Entry      TranslateEntry
	MaxHops    int
	Candidates []TranslationCandidate
	// NameCandidates is empty unless the request opted in AND no relation
	// was found. It is a separate field from Candidates so that no consumer
	// iterating Candidates can ever see a name guess.
	NameCandidates []NameCandidate
	RequiresReview bool
	Note           string
}

// HasRelation reports whether any relation-backed candidate was found. It
// is what the HTTP layer renders as the explicit "no relation recorded"
// outcome — an empty Candidates slice is an ANSWER here, never an error and
// never something to paper over with a name guess.
func (r TranslateResult) HasRelation() bool { return len(r.Candidates) > 0 }

// Translate resolves req's source concept, then returns every concept in
// req.TargetSec that a stored concept_relation connects it to in exactly
// one hop.
//
// The guarantees it owes its callers, in order of how easy they are to
// break:
//
//  1. A result is presented as identity only for domain.RelationCongruent
//     (TranslationCandidate.IsEquality).
//  2. No relation found is an explicit empty answer (Candidates empty,
//     Note set), never a name match dressed up as a translation. A
//     name-based block exists only on explicit opt-in, in its own field,
//     always RequiresReview.
//  3. Exactly MaxTranslateHops hop; a request for more is refused by name.
//  4. Unknown concept id and unknown target space both surface
//     domain.ErrNotFound; an unresolvable verbatim name surfaces
//     ErrUnresolvableName.
//  5. A fuzzy verbatim entry sets RequiresReview on the whole result.
func Translate(ctx context.Context, repo output.Repository, req TranslateRequest) (TranslateResult, error) {
	if req.TargetSec == "" || (req.ConceptID == "") == (req.Verbatim == "") {
		return TranslateResult{}, ErrInvalidTranslateRequest
	}
	if req.MaxHops != 0 && req.MaxHops != MaxTranslateHops {
		return TranslateResult{}, fmt.Errorf("%w (requested %d)", ErrMultiHopUnsupported, req.MaxHops)
	}

	target, err := repo.SecReferenceByID(ctx, req.TargetSec)
	if err != nil {
		return TranslateResult{}, err
	}

	conceptID, entry, err := resolveTranslateEntry(ctx, repo, req)
	if err != nil {
		return TranslateResult{}, err
	}

	// One call resolves both the source concept and its one-hop edges, so
	// an unknown id (domain.ErrNotFound) and a known id with no relations
	// (empty Edges) are decided together rather than by two reads that
	// could disagree.
	rel, err := repo.ConceptRelationsInSec(ctx, conceptID, req.TargetSec)
	if err != nil {
		return TranslateResult{}, err
	}
	source := rel.Source
	sourceSec, err := sourceSecReference(ctx, repo, source.SecReference)
	if err != nil {
		return TranslateResult{}, err
	}

	res := TranslateResult{
		Source:         source,
		SourceSec:      sourceSec,
		TargetSec:      target,
		Entry:          entry,
		MaxHops:        MaxTranslateHops,
		RequiresReview: entry.MatchType == domain.MatchFuzzy,
	}
	for _, e := range rel.Edges {
		res.Candidates = append(res.Candidates, toCandidate(conceptID, e))
	}
	if res.HasRelation() {
		return res, nil
	}

	res.Note = noteNoRelation
	if req.IncludeNameCandidates {
		names, err := nameCandidates(ctx, repo, source, req.TargetSec)
		if err != nil {
			return TranslateResult{}, err
		}
		res.NameCandidates = names
		if len(names) > 0 {
			// A response carrying name candidates always requires review,
			// even when the entry itself was an exact concept id: the block
			// is a human-checkable hint, not an answer. The note says the
			// same thing in prose, in the payload.
			res.RequiresReview = true
			res.Note = noteNoRelation + " " + noteNameCandidates
		}
	}
	return res, nil
}

// toCandidate renders one stored edge as a candidate WITHOUT re-pointing
// it: Relation and StatementFrom/StatementTo always describe the row as
// stored, and RelationFromSource is added only where domain.Relation.Inverse
// says an inverse exists.
func toCandidate(sourceID string, e output.ConceptRelationEdge) TranslationCandidate {
	c := TranslationCandidate{
		Concept:            e.Partner,
		Sec:                e.PartnerSec,
		Relation:           e.Relation,
		StatementFrom:      sourceID,
		StatementTo:        e.Partner.ID,
		Outgoing:           e.Outgoing,
		RelationFromSource: e.Relation,
		IsEquality:         e.Relation.IsEquality(),
		Hops:               MaxTranslateHops,
		Source:             e.Source,
		Note:               candidateNote(e.Relation),
	}
	if e.Outgoing {
		return c
	}
	c.StatementFrom, c.StatementTo = e.Partner.ID, sourceID
	inverse, ok := e.Relation.Inverse()
	if !ok {
		c.RelationFromSource = ""
		c.Note = noteNoInverse
		return c
	}
	c.RelationFromSource = inverse
	return c
}

// candidateNote spells out in the payload itself what a non-congruent
// relation does not say, so a consumer reading only the JSON cannot mistake
// it for identity.
func candidateNote(r domain.Relation) string {
	if r.IsEquality() {
		return ""
	}
	if r == domain.RelationUncertain {
		return noteUncertain
	}
	return noteNotEquality
}

// resolveTranslateEntry returns the source concept id plus a record of how
// it was reached. The verbatim path goes through MatchNames, so /translate
// inherits /v1/match's classification and its rule that a fuzzy hit always
// requires review; an unresolved or ambiguous name is ErrUnresolvableName
// rather than a guess.
func resolveTranslateEntry(ctx context.Context, repo output.Repository, req TranslateRequest) (string, TranslateEntry, error) {
	if req.ConceptID != "" {
		return req.ConceptID, TranslateEntry{Mode: EntryModeConceptID}, nil
	}
	results, err := MatchNames(ctx, repo, []MatchRequest{{ID: "source", Verbatim: req.Verbatim}})
	if err != nil {
		return "", TranslateEntry{}, err
	}
	m := results[0]
	if m.ConceptID == "" {
		return "", TranslateEntry{}, fmt.Errorf("%w: %q", ErrUnresolvableName, req.Verbatim)
	}
	return m.ConceptID, TranslateEntry{
		Mode:       EntryModeVerbatim,
		Verbatim:   req.Verbatim,
		MatchType:  m.MatchType,
		Confidence: m.Confidence,
		Note:       m.Note,
	}, nil
}

// sourceSecReference resolves the source concept's own sec. space for the
// response. An empty id (every pre-SP5 backbone wrote "") and an id with no
// sec_reference row both yield the zero SecReference rather than an error:
// the source's own citation is context, not the answer, and a missing one
// must not fail a translation that otherwise succeeded.
func sourceSecReference(ctx context.Context, repo output.Repository, id string) (domain.SecReference, error) {
	if id == "" {
		return domain.SecReference{}, nil
	}
	sec, err := repo.SecReferenceByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.SecReference{ID: id}, nil
	}
	if err != nil {
		return domain.SecReference{}, err
	}
	return sec, nil
}

// nameCandidates finds concepts in the target space sharing the source
// concept's canonical name. This is the opt-in, explicitly NON-relational
// block: every entry is marked RequiresReview, it lives in its own result
// field, and it carries no relation to omit or misread.
func nameCandidates(ctx context.Context, repo output.Repository, source domain.Concept, targetSec string) ([]NameCandidate, error) {
	matches, err := repo.MatchExact(ctx, domain.Canonicalize(source.AcceptedName.Canonical))
	if err != nil {
		return nil, err
	}
	var out []NameCandidate
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		if m.Concept.ID == source.ID || m.Concept.SecReference != targetSec || seen[m.Concept.ID] {
			continue
		}
		seen[m.Concept.ID] = true
		sec, err := sourceSecReference(ctx, repo, m.Concept.SecReference)
		if err != nil {
			return nil, err
		}
		out = append(out, NameCandidate{Concept: m.Concept, Sec: sec, RequiresReview: true})
	}
	return out, nil
}
