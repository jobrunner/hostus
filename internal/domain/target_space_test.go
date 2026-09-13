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
		wantChoice  domain.TargetSpaceChoice
		wantPolicy  domain.AggregatePolicy
	}{
		{
			name:        "aggregate query, space carries the aggregate as its own taxon",
			isAggregate: true,
			entries:     festuca,
			wantChoice:  domain.TargetSpaceChoice{Name: "Festuca ovina aggr.", ExtID: "5648"},
			wantPolicy:  domain.AggregatePolicyKnown,
		},
		{
			name:        "plain species query carries no policy and the nominate spelling",
			isAggregate: false,
			entries:     festuca,
			wantChoice:  domain.TargetSpaceChoice{Name: "Festuca ovina", ExtID: "5647"},
			wantPolicy:  "",
		},
		{
			name:        "aggregate query, space knows only the microspecies -> unresolvable, no name",
			isAggregate: true,
			entries: []domain.NameSpaceEntry{
				{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina", Aggregate: false},
			},
			wantChoice: domain.TargetSpaceChoice{},
			wantPolicy: domain.AggregatePolicyUnresolvable,
		},
		{
			name:        "aggregate query, concept has no target-space entry at all -> unresolvable",
			isAggregate: true,
			entries:     nil,
			wantChoice:  domain.TargetSpaceChoice{},
			wantPolicy:  domain.AggregatePolicyUnresolvable,
		},
		{
			name:        "plain species query, concept has no target-space entry -> no name, no policy",
			isAggregate: false,
			entries:     nil,
			wantChoice:  domain.TargetSpaceChoice{},
			wantPolicy:  "",
		},
		{
			name:        "plain species query, only an aggregate spelling exists -> that spelling, no policy",
			isAggregate: false,
			entries: []domain.NameSpaceEntry{
				{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true},
			},
			wantChoice: domain.TargetSpaceChoice{Name: "Festuca ovina aggr.", ExtID: "5648"},
			wantPolicy: "",
		},
		{
			// Pins the nominate PREFERENCE, not merely entries[0]: an
			// aggregate spelling is listed FIRST, yet a plain-species query
			// must still hand back the nominate. The repository orders entries
			// by (space, ext_id), which is independent of the aggregate flag,
			// so nothing but this branch guarantees the nominate wins — a
			// concept whose aggregate spelling has the lower ext_id would
			// otherwise leak the aggregate name as the ESy name for a plain
			// species, exactly the confusion ResolveTargetSpace exists to avoid.
			name:        "plain species query, aggregate spelling sorts first -> still the nominate",
			isAggregate: false,
			entries: []domain.NameSpaceEntry{
				{Space: "floraveg", ExtID: "1", Name: "Festuca ovina aggr.", Aggregate: true},
				{Space: "floraveg", ExtID: "2", Name: "Festuca ovina", Aggregate: false},
			},
			wantChoice: domain.TargetSpaceChoice{Name: "Festuca ovina", ExtID: "2"},
			wantPolicy: "",
		},
		{
			// Pins that the CHOSEN entry's own ext_id/status ride along, not
			// just its name — the whole point of TargetSpaceChoice. The
			// accepted-in-space entry is picked (a08253f0-... models a real
			// Euro+Med TaxonUsageID shape), so Status reports "accepted".
			name:        "accepted entry carries its ext_id and accepted status",
			isAggregate: false,
			entries: []domain.NameSpaceEntry{
				{Space: "eurosl", ExtID: "a08253f0-0000-0000-0000-000000000001", Name: "Inula hirta", Status: domain.NameSpaceStatusAccepted},
			},
			wantChoice: domain.TargetSpaceChoice{Name: "Inula hirta", ExtID: "a08253f0-0000-0000-0000-000000000001", Status: domain.NameSpaceStatusAccepted},
			wantPolicy: "",
		},
		{
			// The Inula-hirta class: no accepted-in-space entry exists (WCVP's
			// side of the concept carries only Euro+Med SYNONYMS), so
			// pickSpelling falls back to the first spelling and its Status is
			// the source's own synonym status verbatim — this is exactly how
			// Habitatus is meant to tell "this is an E+M synonym, not the
			// accepted E+M name" from the response alone.
			name:        "no accepted-in-space entry -> fallback spelling's own synonym status",
			isAggregate: false,
			entries: []domain.NameSpaceEntry{
				{Space: "eurosl", ExtID: "b19364e1-0000-0000-0000-000000000002", Name: "Inula hirta subsp. hirta", Status: "synonymobjective"},
			},
			wantChoice: domain.TargetSpaceChoice{Name: "Inula hirta subsp. hirta", ExtID: "b19364e1-0000-0000-0000-000000000002", Status: "synonymobjective"},
			wantPolicy: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotChoice, gotPolicy := domain.ResolveTargetSpace(tc.isAggregate, tc.entries)
			if gotChoice != tc.wantChoice {
				t.Errorf("choice = %+v, want %+v", gotChoice, tc.wantChoice)
			}
			if gotPolicy != tc.wantPolicy {
				t.Errorf("policy = %q, want %q", gotPolicy, tc.wantPolicy)
			}
		})
	}
}
