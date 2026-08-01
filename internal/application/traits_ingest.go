package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// unmatchedSampleCap bounds TraitIngestReport.UnmatchedSample so a huge
// vocabulary never balloons the report — the point is to show a reviewer
// WHICH taxa were lost, not to enumerate every one.
const unmatchedSampleCap = 20

// TraitRow is the minimal, vocabulary-agnostic shape of one trait
// observation IngestTraits needs: one (taxon, dim) value from one
// vocabulary/version. A concrete trait reader's row type (e.g.
// traits.Row) is adapted into this DTO by the caller, which is how
// application avoids importing internal/adapters/traits directly
// (depguard) — the same RowSource-bridge pattern Ingest/RowSource use for
// backbones.
type TraitRow struct {
	Taxon        string
	Vocab        string
	VocabVersion string
	Dim          string
	Value        float64
	NicheWidth   *float64
	NSystems     *int
}

// TraitRowSource streams one vocabulary's rows for IngestTraits. The caller
// adapts a concrete trait reader (e.g. traits.Read's *traits.Dataset) into
// this interface.
type TraitRowSource interface {
	Rows() []TraitRow
}

// TraitIngestReport summarizes one vocabulary's crosswalk run. The crosswalk
// from a trait table's bare taxon name string to a hostus taxon_concept is
// lossy by construction (PoC P6: the trait tables carry no external taxon
// id) — this report exists so that loss is VISIBLE rather than silent.
// Matched+Unmatched+Ambiguous always sums to Rows.
type TraitIngestReport struct {
	Vocab     string
	Rows      int
	Matched   int
	Unmatched int
	Ambiguous int
	// UnmatchedSample holds a bounded (unmatchedSampleCap), deterministic
	// (sorted, deduplicated) sample of the taxon names that failed to
	// resolve, so a reviewer/operator can see WHICH taxa were lost without
	// hostus dumping every unmatched row.
	UnmatchedSample []string
}

// traitResolution is one canonical taxon name's crosswalk outcome, decided
// in IngestTraits' resolve phase and consumed in its write phase.
type traitResolution struct {
	conceptID string
	matched   bool
	ambiguous bool
}

// IngestTraits resolves every row src provides against repo's name index
// and writes matched values as trait_value rows, then records meta as the
// vocabulary's provenance.
//
// It runs in two strictly separated phases — RESOLVE first, WRITE second:
//
//	Phase 1 (no transaction open): every DISTINCT canonical taxon name is
//	resolved through repo.MatchExact and cached in a map.
//	Phase 2: BeginIngest -> UpsertTraitVocabulary + AddTraitValue -> Commit,
//	consulting only that map. NO repository read happens while the ingest
//	transaction is open.
//
// The split is not a micro-optimization, it is a correctness requirement.
// The sqlite adapter deliberately runs with SetMaxOpenConns(1) (the
// foreign_keys pragma is per-connection and a ":memory:" database must not
// be split across connections), so an open IngestTx holds the ONLY pooled
// connection. A repo.MatchExact issued from inside the transaction would
// block forever waiting for a second connection — a real deadlock in
// "hostus ingest", not just in tests. The backbone ingest observes the same
// resolve-then-write discipline.
//
// Resolution per row, per PoC P6/task brief — never guess:
//
//  1. domain.Canonicalize(row.Taxon), then repo.MatchExact.
//  2. Zero candidates -> Unmatched (no trait_value written).
//  3. One or more candidates that ALL resolve to the SAME concept (e.g. a
//     synonym and its accepted name both matching) -> Matched, the value
//     lands on that one concept.
//  4. Candidates resolving to two or more DISTINCT concepts -> Ambiguous,
//     skipped entirely (never guessed which concept the row meant).
//
// The vocabulary metadata (meta) is recorded regardless of match outcome,
// via IngestTx.UpsertTraitVocabulary — even a vocabulary version that
// matches nothing at all should still show up in TraitVocabularies.
func IngestTraits(ctx context.Context, repo output.Repository, src TraitRowSource, meta domain.TraitVocabMeta) (TraitIngestReport, error) {
	report := TraitIngestReport{Vocab: string(meta.Vocab)}
	rows := src.Rows()
	report.Rows = len(rows)

	resolved, err := resolveTraitTaxa(ctx, repo, rows)
	if err != nil {
		return report, fmt.Errorf("application: resolving trait taxa for vocab %q: %w", meta.Vocab, err)
	}

	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{
		ID:      "trait:" + string(meta.Vocab),
		Version: meta.Version,
		License: meta.License,
	})
	if err != nil {
		return report, fmt.Errorf("application: starting trait ingest for vocab %q: %w", meta.Vocab, err)
	}

	if err := tx.UpsertTraitVocabulary(meta); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: recording trait vocabulary %q: %w", meta.Vocab, err)
	}

	unmatched := make(map[string]bool)
	for _, row := range rows {
		res := resolved[domain.Canonicalize(row.Taxon)]
		switch {
		case res.ambiguous:
			report.Ambiguous++
		case !res.matched:
			report.Unmatched++
			unmatched[row.Taxon] = true
		default:
			dim, err := domain.ParseTraitDim(row.Dim)
			if err != nil {
				_ = tx.Rollback()
				return report, fmt.Errorf("application: vocab %q, taxon %q: %w", meta.Vocab, row.Taxon, err)
			}
			tv := domain.TraitValue{
				Vocab:        meta.Vocab,
				VocabVersion: row.VocabVersion,
				Dim:          dim,
				Value:        row.Value,
				NicheWidth:   row.NicheWidth,
				NSystems:     row.NSystems,
			}
			if err := tx.AddTraitValue(res.conceptID, tv); err != nil {
				_ = tx.Rollback()
				return report, fmt.Errorf("application: writing trait value for concept %q, vocab %q: %w", res.conceptID, meta.Vocab, err)
			}
			report.Matched++
		}
	}

	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing trait ingest for vocab %q: %w", meta.Vocab, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing trait ingest for vocab %q: %w", meta.Vocab, err)
	}

	report.UnmatchedSample = sortedSample(unmatched, unmatchedSampleCap)
	return report, nil
}

// resolveTraitTaxa is IngestTraits' phase 1: it maps every DISTINCT
// canonical taxon name occurring in rows to its traitResolution, querying
// repo exactly once per distinct name (a trait table carries one row per
// (taxon, dim), so a vocabulary with five dims would otherwise issue five
// identical MatchExact queries per taxon). It must be called with no ingest
// transaction open — see IngestTraits' doc comment.
func resolveTraitTaxa(ctx context.Context, repo output.Repository, rows []TraitRow) (map[string]traitResolution, error) {
	resolved := make(map[string]traitResolution)
	for _, row := range rows {
		canon := domain.Canonicalize(row.Taxon)
		if _, seen := resolved[canon]; seen {
			continue
		}
		res, err := resolveTraitName(ctx, repo, canon)
		if err != nil {
			return nil, fmt.Errorf("taxon %q: %w", row.Taxon, err)
		}
		resolved[canon] = res
	}
	return resolved, nil
}

// resolveTraitName classifies repo.MatchExact's candidates for the already
// canonicalized name canon into exactly one of: matched to a single concept
// id, matched=false (zero candidates), or ambiguous=true (candidates
// resolving to two or more distinct concepts). It never picks a concept
// when ambiguous.
func resolveTraitName(ctx context.Context, repo output.Repository, canon string) (traitResolution, error) {
	candidates, err := repo.MatchExact(ctx, canon)
	if err != nil {
		return traitResolution{}, err
	}
	if len(candidates) == 0 {
		return traitResolution{}, nil
	}
	distinct := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		distinct[c.Concept.ID] = true
	}
	if len(distinct) > 1 {
		return traitResolution{ambiguous: true}, nil
	}
	return traitResolution{conceptID: candidates[0].Concept.ID, matched: true}, nil
}

// sortedSample returns a deterministic (sorted), bounded (at most cap)
// sample of set's keys, so TraitIngestReport.UnmatchedSample never varies
// across runs and never grows unbounded for a large lossy vocabulary.
func sortedSample(set map[string]bool, cap int) []string {
	if len(set) == 0 {
		return nil
	}
	all := make([]string, 0, len(set))
	for k := range set {
		all = append(all, k)
	}
	sort.Strings(all)
	// len(all) >= cap is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY: at len(all) == cap exactly, all[:cap] IS all,
	// so both branches produce the identical slice and no test can observe
	// the difference (same provable-equivalence class as the two documented
	// boundaries in suggest.go).
	if len(all) > cap {
		all = all[:cap]
	}
	return all
}
