package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestResolveTargetSpace pins the tri-state at its source: the pure decision
// over a concept's name-space entries, before any I/O. The three UC4 states
// are distinct and none is a boolean fallback of another (see
// docs/superpowers/plans SP9 Task 2, and SP6's absent-vs-unclassified
// reasoning): a plain species carries NO policy, a known aggregate carries
// "known" with the aggregate spelling, and an aggregate the space does not
// carry as a taxon of its own is "unresolvable" with no ESy name to hand back.
func TestResolveTargetSpace(t *testing.T) {
	festuca := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina", Aggregate: false},
		{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true},
		{Space: "floraveg", ExtID: "5649", Name: "Festuca ovina s. l.", Aggregate: true},
	}

	tests := []struct {
		name        string
		isAggregate bool
		entries     []domain.NameSpaceEntry
		wantName    string
		wantPolicy  domain.AggregatePolicy
	}{
		{
			name:        "aggregate query, space carries the aggregate as its own taxon",
			isAggregate: true,
			entries:     festuca,
			wantName:    "Festuca ovina aggr.",
			wantPolicy:  domain.AggregatePolicyKnown,
		},
		{
			name:        "plain species query carries no policy and the nominate spelling",
			isAggregate: false,
			entries:     festuca,
			wantName:    "Festuca ovina",
			wantPolicy:  "",
		},
		{
			name:        "aggregate query, space knows only the microspecies -> unresolvable, no name",
			isAggregate: true,
			entries: []domain.NameSpaceEntry{
				{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina", Aggregate: false},
			},
			wantName:   "",
			wantPolicy: domain.AggregatePolicyUnresolvable,
		},
		{
			name:        "aggregate query, concept has no target-space entry at all -> unresolvable",
			isAggregate: true,
			entries:     nil,
			wantName:    "",
			wantPolicy:  domain.AggregatePolicyUnresolvable,
		},
		{
			name:        "plain species query, concept has no target-space entry -> no name, no policy",
			isAggregate: false,
			entries:     nil,
			wantName:    "",
			wantPolicy:  "",
		},
		{
			name:        "plain species query, only an aggregate spelling exists -> that spelling, no policy",
			isAggregate: false,
			entries: []domain.NameSpaceEntry{
				{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true},
			},
			wantName:   "Festuca ovina aggr.",
			wantPolicy: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotPolicy := domain.ResolveTargetSpace(tc.isAggregate, tc.entries)
			if gotName != tc.wantName {
				t.Errorf("name = %q, want %q", gotName, tc.wantName)
			}
			if gotPolicy != tc.wantPolicy {
				t.Errorf("policy = %q, want %q", gotPolicy, tc.wantPolicy)
			}
		})
	}
}
