package domain

import "sort"

// Agreement classifies how one eurosl aggregate concept's member set
// compares to its germansl counterpart's member set (or the absence of
// one) — the precomputed result schema.sql's concept_agreement table
// stores per pair (see docs/superpowers/specs/2026-08-27-hostus-namensraum-
// redesign-design.md section 5).
type Agreement string

const (
	AgreementIdentical Agreement = "identical"
	AgreementSubset    Agreement = "subset"
	AgreementSuperset  Agreement = "superset"
	AgreementOverlap   Agreement = "overlap"
	AgreementDisjoint  Agreement = "disjoint"
	AgreementOneSided  Agreement = "one_sided"
)

// ConceptAgreementPair is one row of the precomputed comparison between an
// eurosl aggregate concept and its name-matched germansl counterpart (see
// application.ComputeConceptAgreement, which produces these). Either
// EuroslConceptID or GermanslConceptID is empty when Agreement is
// AgreementOneSided and only the other name space knows this aggregate.
type ConceptAgreementPair struct {
	EuroslConceptID   string
	GermanslConceptID string
	Agreement         Agreement
	AgreementText     string
	// OnlyInEurosl/OnlyInGermansl are the sorted WCVP concept ids each side
	// includes that the other does not — always empty for AgreementOneSided
	// (there is nothing to compare a missing side against).
	OnlyInEurosl   []string
	OnlyInGermansl []string
}

// CompareAggregateMembers computes the Agreement between two member sets
// (a-side, b-side), given as concept-id slices. It is a pure set
// comparison — the caller resolves which side is "eurosl" and which is
// "germansl". onlyA/onlyB are returned sorted and deduplicated.
func CompareAggregateMembers(a, b []string) (agreement Agreement, onlyA, onlyB []string) {
	setA, setB := toSet(a), toSet(b)
	for id := range setA {
		if !setB[id] {
			onlyA = append(onlyA, id)
		}
	}
	for id := range setB {
		if !setA[id] {
			onlyB = append(onlyB, id)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)

	// Hoisted for the same coverage reason as ClassifyNomStatus'/
	// judgeSynonym's switch (see internal/domain/synonym.go): a condition
	// written inside a case arm sits in no counted block in Go's coverage
	// model, so `make mutation` reports it as NOT COVERED regardless of
	// how thoroughly the branch is actually tested. As plain assignments
	// they are covered, mutated and killed.
	aEmpty := len(onlyA) == 0
	bEmpty := len(onlyB) == 0
	disjoint := len(setA)+len(setB)-len(onlyA)-len(onlyB) == 0 // kein gemeinsames Element

	switch {
	case aEmpty && bEmpty:
		return AgreementIdentical, onlyA, onlyB
	case aEmpty:
		return AgreementSubset, onlyA, onlyB // a ⊆ b
	case bEmpty:
		return AgreementSuperset, onlyA, onlyB // a ⊇ b
	case disjoint:
		return AgreementDisjoint, onlyA, onlyB
	default:
		return AgreementOverlap, onlyA, onlyB
	}
}

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
