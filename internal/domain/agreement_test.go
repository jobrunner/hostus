package domain_test

import (
	"reflect"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestCompareAggregateMembers_IdenticalSetsYieldIdentical(t *testing.T) {
	agreement, onlyA, onlyB := domain.CompareAggregateMembers(
		[]string{"wcvp:concept:1"}, []string{"wcvp:concept:1"})
	if agreement != domain.AgreementIdentical {
		t.Errorf("agreement = %q, want %q", agreement, domain.AgreementIdentical)
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("onlyA=%v onlyB=%v, want both empty", onlyA, onlyB)
	}
}

func TestCompareAggregateMembers_BSupersetOfA(t *testing.T) {
	agreement, onlyA, onlyB := domain.CompareAggregateMembers(
		[]string{"wcvp:concept:1"}, []string{"wcvp:concept:1", "wcvp:concept:2"})
	if agreement != domain.AgreementSubset {
		t.Errorf("agreement = %q, want %q (a ⊆ b)", agreement, domain.AgreementSubset)
	}
	if len(onlyA) != 0 {
		t.Errorf("onlyA = %v, want empty", onlyA)
	}
	if !reflect.DeepEqual(onlyB, []string{"wcvp:concept:2"}) {
		t.Errorf("onlyB = %v, want [wcvp:concept:2]", onlyB)
	}
}

func TestCompareAggregateMembers_ASupersetOfB(t *testing.T) {
	agreement, onlyA, _ := domain.CompareAggregateMembers(
		[]string{"wcvp:concept:1", "wcvp:concept:2"}, []string{"wcvp:concept:1"})
	if agreement != domain.AgreementSuperset {
		t.Errorf("agreement = %q, want %q (a ⊇ b)", agreement, domain.AgreementSuperset)
	}
	if !reflect.DeepEqual(onlyA, []string{"wcvp:concept:2"}) {
		t.Errorf("onlyA = %v, want [wcvp:concept:2]", onlyA)
	}
}

func TestCompareAggregateMembers_DisjointSets(t *testing.T) {
	agreement, onlyA, onlyB := domain.CompareAggregateMembers(
		[]string{"wcvp:concept:1"}, []string{"wcvp:concept:2"})
	if agreement != domain.AgreementDisjoint {
		t.Errorf("agreement = %q, want %q", agreement, domain.AgreementDisjoint)
	}
	if !reflect.DeepEqual(onlyA, []string{"wcvp:concept:1"}) || !reflect.DeepEqual(onlyB, []string{"wcvp:concept:2"}) {
		t.Errorf("onlyA=%v onlyB=%v, want [wcvp:concept:1] and [wcvp:concept:2]", onlyA, onlyB)
	}
}

func TestCompareAggregateMembers_PartialOverlap(t *testing.T) {
	agreement, onlyA, onlyB := domain.CompareAggregateMembers(
		[]string{"wcvp:concept:1", "wcvp:concept:2"}, []string{"wcvp:concept:2", "wcvp:concept:3"})
	if agreement != domain.AgreementOverlap {
		t.Errorf("agreement = %q, want %q", agreement, domain.AgreementOverlap)
	}
	if !reflect.DeepEqual(onlyA, []string{"wcvp:concept:1"}) || !reflect.DeepEqual(onlyB, []string{"wcvp:concept:3"}) {
		t.Errorf("onlyA=%v onlyB=%v, want [wcvp:concept:1] and [wcvp:concept:3]", onlyA, onlyB)
	}
}
