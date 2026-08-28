package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// aggregateRanks are the two Fall-B ranks that ever carry a WCVP-member
// aggregation (schema.sql's concept_aggregate, Task 5/6): SPECIES_AGGREGATE
// and GENUS_AGGREGATE.
var aggregateRanks = []domain.Rank{domain.RankSpeciesAggregate, domain.RankGenusAggregate}

// ConceptAgreementReport is ComputeConceptAgreement's result: one
// domain.ConceptAgreementPair per eurosl/germansl aggregate (or lone
// one-sided aggregate) found.
type ConceptAgreementReport struct {
	Pairs []domain.ConceptAgreementPair
}

// ComputeConceptAgreement pairs up every eurosl aggregate concept (rank
// SPECIES_AGGREGATE/GENUS_AGGREGATE) with its germansl counterpart, matched
// by name — domain.Canonicalize(domain.StripAggregateMarkers(canonical)) —
// and compares their WCVP member sets via domain.CompareAggregateMembers.
// An aggregate with no name-matched counterpart in the other space is
// reported as domain.AgreementOneSided, its own side's OnlyIn* left empty
// (there is nothing to compare a missing side against).
//
// If two aggregates on the SAME side share a match key (a name collision —
// not specified by the design), the one with the lexicographically first
// concept id is used and the rest are silently dropped from matching; this
// is a deliberate, simple tie-break rather than an attempt at a fuller
// dedup policy the design does not call for.
//
// The result is not persisted here — callers pass report.Pairs to
// Repository.WriteConceptAgreement themselves, since this function only
// computes the comparison (application.ComputeConceptAgreement's own name
// says "Compute", not "Write").
func ComputeConceptAgreement(ctx context.Context, repo output.Repository) (ConceptAgreementReport, error) {
	eurosl, err := repo.AggregateConcepts(ctx, "eurosl", aggregateRanks)
	if err != nil {
		return ConceptAgreementReport{}, fmt.Errorf("application: listing eurosl aggregate concepts: %w", err)
	}
	germansl, err := repo.AggregateConcepts(ctx, "germansl", aggregateRanks)
	if err != nil {
		return ConceptAgreementReport{}, fmt.Errorf("application: listing germansl aggregate concepts: %w", err)
	}

	germanslByKey := indexAggregatesByMatchKey(germansl)
	matchedGermansl := map[string]bool{}

	report := ConceptAgreementReport{}
	for _, e := range indexAggregatesByMatchKey(eurosl) {
		key := e.matchKey
		g, ok := germanslByKey[key]
		pair, err := buildAgreementPair(ctx, repo, e.summary, g, ok)
		if err != nil {
			return ConceptAgreementReport{}, err
		}
		if ok {
			matchedGermansl[g.summary.ConceptID] = true
		}
		report.Pairs = append(report.Pairs, pair)
	}

	// Every germansl aggregate that no eurosl aggregate matched is its own
	// one-sided pair.
	for _, key := range sortedKeys(germanslByKey) {
		g := germanslByKey[key]
		if matchedGermansl[g.summary.ConceptID] {
			continue
		}
		pair := domain.ConceptAgreementPair{
			GermanslConceptID: g.summary.ConceptID,
			Agreement:         domain.AgreementOneSided,
			AgreementText:     agreementText(domain.AgreementOneSided),
		}
		report.Pairs = append(report.Pairs, pair)
	}

	return report, nil
}

// keyedAggregate pairs one output.AggregateConceptSummary with its
// precomputed match key.
type keyedAggregate struct {
	summary  output.AggregateConceptSummary
	matchKey string
}

// indexAggregatesByMatchKey groups summaries by
// domain.Canonicalize(domain.StripAggregateMarkers(...)), keeping only the
// lexicographically first concept id per key on a collision (see
// ComputeConceptAgreement's doc comment).
func indexAggregatesByMatchKey(summaries []output.AggregateConceptSummary) map[string]keyedAggregate {
	out := make(map[string]keyedAggregate, len(summaries))
	for _, s := range summaries {
		key := domain.Canonicalize(domain.StripAggregateMarkers(s.Canonical))
		existing, ok := out[key]
		if ok && existing.summary.ConceptID <= s.ConceptID {
			continue
		}
		out[key] = keyedAggregate{summary: s, matchKey: key}
	}
	return out
}

func sortedKeys(m map[string]keyedAggregate) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// buildAgreementPair builds one domain.ConceptAgreementPair for eurosl
// aggregate e, matched (or not, per hasMatch) against germansl aggregate g.
func buildAgreementPair(ctx context.Context, repo output.Repository, e output.AggregateConceptSummary, g keyedAggregate, hasMatch bool) (domain.ConceptAgreementPair, error) {
	if !hasMatch {
		return domain.ConceptAgreementPair{
			EuroslConceptID: e.ConceptID,
			Agreement:       domain.AgreementOneSided,
			AgreementText:   agreementText(domain.AgreementOneSided),
		}, nil
	}

	euroslMembers, err := repo.AggregateMembers(ctx, e.ConceptID)
	if err != nil {
		return domain.ConceptAgreementPair{}, fmt.Errorf("application: reading aggregate members for %q: %w", e.ConceptID, err)
	}
	germanslMembers, err := repo.AggregateMembers(ctx, g.summary.ConceptID)
	if err != nil {
		return domain.ConceptAgreementPair{}, fmt.Errorf("application: reading aggregate members for %q: %w", g.summary.ConceptID, err)
	}

	agreement, onlyEurosl, onlyGermansl := domain.CompareAggregateMembers(euroslMembers, germanslMembers)
	return domain.ConceptAgreementPair{
		EuroslConceptID:   e.ConceptID,
		GermanslConceptID: g.summary.ConceptID,
		Agreement:         agreement,
		AgreementText:     agreementText(agreement),
		OnlyInEurosl:      onlyEurosl,
		OnlyInGermansl:    onlyGermansl,
	}, nil
}

// agreementText renders a generic, German, human-readable sentence per
// domain.Agreement value — a fixed template, not a per-pair narrative (the
// design's spec §5 example sentence names a concrete species and is
// illustrative only, not a literal template).
func agreementText(a domain.Agreement) string {
	switch a {
	case domain.AgreementIdentical:
		return "eurosl und germansl führen exakt dieselben Sippen unter diesem Aggregat."
	case domain.AgreementSubset:
		return "eurosl führt eine echte Teilmenge der Sippen, die germansl unter diesem Aggregat führt."
	case domain.AgreementSuperset:
		return "eurosl führt zusätzliche Sippen unter diesem Aggregat, die germansl nicht (oder anders) fasst."
	case domain.AgreementOverlap:
		return "eurosl und germansl fassen unter diesem Aggregat teilweise unterschiedliche Sippen — beide Seiten führen Sippen, die die jeweils andere nicht kennt."
	case domain.AgreementDisjoint:
		return "eurosl und germansl führen unter diesem Aggregat keine gemeinsame Sippe — trotz gleichlautendem Namen decken sich die Mitgliederlisten nicht."
	case domain.AgreementOneSided:
		return "nur einer der beiden Namensräume (eurosl oder germansl) kennt dieses Aggregat; im anderen gibt es kein gleichnamiges Gegenstück."
	default:
		return "unbekannter Vergleichsstatus."
	}
}
