package application

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestRuleRank_CoversEveryNormalizationRuleAndRespectsFlagged pins two
// invariants selectTraitWinners depends on for its slot-contention
// precedence (see ruleRank's doc comment):
//
//  1. every domain.NormalizationRule domain.NameCandidates can emit has an
//     entry — a rule missing from ruleRank would silently rank as 0 (the Go
//     zero value), i.e. as if it were RuleExact, and quietly win every
//     contested slot it touches. This list is deliberately the same
//     enumeration domain.NormalizationRule.Flagged's switch uses, so the two
//     stay in lockstep if a rule is ever added.
//  2. every flagged rule ranks strictly above every unflagged rule, and
//     RuleExact ranks below all of them — the "exact > unflagged > flagged"
//     precedence the Hardening Task 6 fix (A1) makes explicit.
func TestRuleRank_CoversEveryNormalizationRuleAndRespectsFlagged(t *testing.T) {
	allRules := []domain.NormalizationRule{
		domain.RuleExact,
		domain.RuleHybridSpacing,
		domain.RuleHybridMarkerDropped,
		domain.RuleHybridMarkerAdded,
		domain.RuleAggregate,
		domain.RuleAggregateToNominate,
		domain.RuleAutonym,
		domain.RuleOrthographyGenitive,
	}
	if len(ruleRank) != len(allRules) {
		t.Fatalf("ruleRank has %d entries, want exactly %d (one per known NormalizationRule)", len(ruleRank), len(allRules))
	}

	maxUnflagged, minFlagged := -1, 1<<30
	for _, rule := range allRules {
		rank, ok := ruleRank[rule]
		if !ok {
			t.Fatalf("ruleRank has no entry for %q", rule)
		}
		if rule == domain.RuleExact && rank != 0 {
			t.Errorf("ruleRank[RuleExact] = %d, want 0 (must rank first)", rank)
		}
		if rule.Flagged() {
			if rank < minFlagged {
				minFlagged = rank
			}
			continue
		}
		if rank > maxUnflagged {
			maxUnflagged = rank
		}
	}
	if maxUnflagged >= minFlagged {
		t.Errorf("an unflagged rule ranks (%d) at or below a flagged rule (%d) — exact > unflagged > flagged is violated", maxUnflagged, minFlagged)
	}
}
