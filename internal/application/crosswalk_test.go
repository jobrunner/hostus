package application

import (
	"reflect"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestPreferBackboneConcepts_DropsSecReferenceCandidateWhenBackboneExists
// pins the documented fallback: a sec.-reference-space candidate is dropped
// once a backbone candidate for the same name exists.
func TestPreferBackboneConcepts_DropsSecReferenceCandidateWhenBackboneExists(t *testing.T) {
	backbone := output.MatchCandidate{Concept: domain.Concept{ID: "wcvp:concept:1"}}
	secOnly := output.MatchCandidate{Concept: domain.Concept{ID: "sec:concept:1", SecReference: "sec-x"}}

	got := preferBackboneConcepts([]output.MatchCandidate{backbone, secOnly})

	if !reflect.DeepEqual(got, []output.MatchCandidate{backbone}) {
		t.Errorf("preferBackboneConcepts = %+v, want only the backbone candidate", got)
	}
}

// TestPreferBackboneConcepts_KeepsSecReferenceCandidatesWhenNoBackboneExists
// pins the other side of the fallback: a name that ONLY a sec. concept
// source carries keeps resolving exactly as before (candidates unchanged).
func TestPreferBackboneConcepts_KeepsSecReferenceCandidatesWhenNoBackboneExists(t *testing.T) {
	secOnly := []output.MatchCandidate{
		{Concept: domain.Concept{ID: "sec:concept:1", SecReference: "sec-x"}},
	}

	got := preferBackboneConcepts(secOnly)

	if !reflect.DeepEqual(got, secOnly) {
		t.Errorf("preferBackboneConcepts = %+v, want the input unchanged", got)
	}
}

// TestRuleCounts_SortsByRuleAcrossMultipleDistinctRules exercises
// ruleCounts with 2+ distinct rules, so sort.Slice's comparator actually
// runs at least once (with a single entry, sort.Slice never invokes it).
func TestRuleCounts_SortsByRuleAcrossMultipleDistinctRules(t *testing.T) {
	ruleRows := map[domain.NormalizationRule]int{
		domain.RuleAutonym: 3,
		domain.RuleExact:   5,
	}
	ruleTaxa := map[domain.NormalizationRule]map[string]bool{
		domain.RuleAutonym: {"Festuca ovina": true},
		domain.RuleExact:   {"Festuca rubra": true, "Festuca ovina": true},
	}

	got := ruleCounts(ruleRows, ruleTaxa)

	want := []RuleCount{
		{Rule: domain.RuleAutonym, Rows: 3, Taxa: 1, Flagged: domain.RuleAutonym.Flagged()},
		{Rule: domain.RuleExact, Rows: 5, Taxa: 2, Flagged: domain.RuleExact.Flagged()},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ruleCounts = %+v, want %+v (sorted by Rule)", got, want)
	}
}
