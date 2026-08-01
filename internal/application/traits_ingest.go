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
	// Normalized breaks down, per deterministic normalisation rule (see
	// domain.NameCandidates), the rows whose value was actually STORED
	// under that rule. It is sorted by rule name and lists only rules that
	// fired — a vocabulary resolving purely on exact keys reports an empty
	// slice. This is what keeps a normalised match from being
	// indistinguishable from an exact one in the ingest report.
	//
	// "Stored" is the operative word: a matched row that lost its
	// (concept, dim) slot to another row (see selectTraitWinners) still
	// counts in Matched but is credited to no rule here, so these counts
	// agree with what a "SELECT resolution, COUNT(*) FROM trait_value"
	// against the resulting database reports. Sum(Normalized.Rows) is
	// therefore <= Matched, not equal to it.
	Normalized []RuleCount
	// FlaggedSample holds a bounded (unmatchedSampleCap), deterministic
	// sample of the taxon names that matched only through a FLAGGED rule
	// (domain.NormalizationRule.Flagged — the aggregate-to-nominate-species
	// and autonym-to-species judgement calls). Those two rules equate two
	// circumscriptions that are not strictly identical; listing the names
	// here is what makes the judgement auditable instead of silent.
	FlaggedSample []string
	// Redistribution is this vocabulary's manifest-pinned redistribution
	// value (see domain.Redistribution), surfaced here so "hostus ingest"
	// can print a notice for anything that is not "allowed" — the local
	// ingest itself is never gated by it.
	Redistribution string
}

// RuleCount is one normalisation rule's contribution to a vocabulary's
// crosswalk: how many trait ROWS and how many distinct TAXON names it
// resolved that the plain exact key would have lost. Flagged mirrors
// domain.NormalizationRule.Flagged so a reader of the report does not have
// to know which rules are judgement calls.
type RuleCount struct {
	Rule    domain.NormalizationRule
	Rows    int
	Taxa    int
	Flagged bool
}

// traitResolution is one canonical taxon name's crosswalk outcome, decided
// in IngestTraits' resolve phase and consumed in its write phase.
type traitResolution struct {
	conceptID string
	matched   bool
	ambiguous bool
	// rule is the normalisation rule whose key produced this outcome (both
	// for matched and for ambiguous). domain.RuleExact means the plain
	// Canonicalize key answered — the pre-normalisation behavior.
	rule domain.NormalizationRule
}

// IngestTraits resolves every row src provides against repo's name index
// and writes matched values as trait_value rows, then records meta as the
// vocabulary's provenance.
//
// Before either phase, checkVocabIdentity reconciles the manifest's
// (vocab, version) against the one every CSV row declares, so a manifest
// pointed at the wrong file — or pinning a version the pipeline does not
// emit — fails loudly instead of writing data under a foreign identity.
//
// It runs in two strictly separated phases — RESOLVE first, WRITE second:
//
//	Phase 1 (no transaction open): every DISTINCT canonical taxon name is
//	resolved through repo.MatchExact and cached in a map.
//	Phase 2: BeginTraitIngest -> UpsertTraitVocabulary + AddTraitValue ->
//	Commit, consulting only that map. NO repository read happens while the
//	ingest transaction is open.
//
// Phase 2 uses repo.BeginTraitIngest, NOT repo.BeginIngest: a trait
// vocabulary is not a taxonomic backbone and must never leave a row in
// backbone_version (see output.Repository.BeginTraitIngest).
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
//  1. domain.NameCandidates(row.Taxon) produces an ordered, deterministic
//     ladder of lookup keys: the plain domain.Canonicalize key first, then
//     one key per applicable normalisation rule (hybrid marker, aggregate,
//     autonym, -ii/-i genitive — Hardening Task 5). Each is tried through
//     repo.MatchExact in order, and the FIRST key the index answers decides
//     the outcome; see resolveTraitName.
//  2. No key answered -> Unmatched (no trait_value written).
//  3. One or more candidates that ALL resolve to the SAME concept (e.g. a
//     synonym and its accepted name both matching) -> Matched, the value
//     lands on that one concept.
//  4. Candidates resolving to two or more DISTINCT concepts -> Ambiguous,
//     skipped entirely (never guessed which concept the row meant).
//
// Every match that needed more than the plain exact key is counted in
// TraitIngestReport.Normalized, and the two rules that rest on a botanical
// judgement (aggregate-to-nominate-species, autonym-to-species) additionally
// name their taxa in TraitIngestReport.FlaggedSample — a normalised match
// is never reported as if it had been exact.
//
// The vocabulary metadata (meta) is recorded regardless of match outcome,
// via IngestTx.UpsertTraitVocabulary — even a vocabulary version that
// matches nothing at all should still show up in TraitVocabularies.
func IngestTraits(ctx context.Context, repo output.Repository, src TraitRowSource, meta domain.TraitVocabMeta) (TraitIngestReport, error) {
	report := TraitIngestReport{Vocab: string(meta.Vocab), Redistribution: string(meta.Redistribution)}
	rows := src.Rows()
	report.Rows = len(rows)

	if err := checkVocabIdentity(rows, meta); err != nil {
		return report, err
	}

	resolved, err := resolveTraitTaxa(ctx, repo, rows)
	if err != nil {
		return report, fmt.Errorf("application: resolving trait taxa for vocab %q: %w", meta.Vocab, err)
	}

	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		return report, fmt.Errorf("application: starting trait ingest for vocab %q: %w", meta.Vocab, err)
	}

	if err := tx.UpsertTraitVocabulary(meta); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: recording trait vocabulary %q: %w", meta.Vocab, err)
	}

	winners := selectTraitWinners(rows, resolved)
	tally := newTraitTally()
	for i, row := range rows {
		res := resolved[domain.Canonicalize(row.Taxon)]
		switch {
		case res.ambiguous:
			report.Ambiguous++
		case !res.matched:
			report.Unmatched++
			tally.countUnmatched(row.Taxon)
		default:
			report.Matched++
			if !winners[i] {
				// Another row of this same vocabulary already owns this
				// (concept, dim) slot and outranks this one — see
				// selectTraitWinners. The row still counts as matched (it DID
				// resolve), but it stores nothing, so it must not be credited
				// to a normalisation rule either.
				continue
			}
			tv, err := traitValueFor(row, meta, res.rule)
			if err != nil {
				_ = tx.Rollback()
				return report, fmt.Errorf("application: vocab %q, taxon %q: %w", meta.Vocab, row.Taxon, err)
			}
			if err := tx.AddTraitValue(res.conceptID, tv); err != nil {
				_ = tx.Rollback()
				return report, fmt.Errorf("application: writing trait value for concept %q, vocab %q: %w", res.conceptID, meta.Vocab, err)
			}
			tally.countMatched(row.Taxon, res.rule)
		}
	}

	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing trait ingest for vocab %q: %w", meta.Vocab, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing trait ingest for vocab %q: %w", meta.Vocab, err)
	}

	tally.report(&report)
	return report, nil
}

// traitSlot is the identity trait_value's primary key reduces a written row
// to WITHIN one IngestTraits call (vocab and vocab_version are fixed by meta
// for the whole call, so only the concept and the dimension vary).
type traitSlot struct {
	conceptID string
	dim       string
}

// ruleRank orders every domain.NormalizationRule for selectTraitWinners'
// slot-contention decision. It mirrors domain.NameCandidates' emission
// order exactly (see that function's doc comment): RuleExact first, then
// the pure-spelling rewrites in the order NameCandidates tries them, then
// the two flagged circumscription judgements last. A lower rank always
// wins a contested slot.
//
// This is the fix for the defect one level below the one selectTraitWinners
// already guards: an exact match beating a normalised one was fixed first
// (see the Hardening Task 5 walk-back), but among normalised rules the
// code fell back to plain row order — meaning a FLAGGED rule
// (aggregate_to_nominate, autonym: these equate two circumscriptions that
// are not identical) could beat an UNFLAGGED, pure-spelling rule
// (hybrid_spacing, hybrid_marker_*, orthography_genitive: circumscription
// unchanged) purely because the flagged row happened to appear first in
// the CSV. ruleRank makes the precedence explicit: exact > unflagged
// normalised > flagged normalised, with row order only breaking a tie
// between two rows that resolved via the IDENTICAL rule.
//
// If domain.NameCandidates' rule set ever grows, this map must grow with
// it — TestRuleRank_CoversEveryNormalizationRule pins that as a compile
// -time-adjacent invariant (a table-driven test over every known rule).
var ruleRank = map[domain.NormalizationRule]int{
	domain.RuleExact:               0,
	domain.RuleHybridSpacing:       1,
	domain.RuleHybridMarkerDropped: 2,
	domain.RuleHybridMarkerAdded:   3,
	domain.RuleOrthographyGenitive: 4,
	domain.RuleAggregate:           5,
	domain.RuleAggregateToNominate: 6,
	domain.RuleAutonym:             7,
}

// selectTraitWinners decides, for every (concept, dim) slot, WHICH of the
// rows resolving to it actually gets stored — returning the set of winning
// row indices.
//
// It exists because the crosswalk is many-to-one: several vocabulary taxon
// names can resolve to the same concept ("Acer opalus" exactly, and
// "Acer opalus aggr." via the aggregate fallback), and trait_value's primary
// key is (concept_id, vocab, vocab_version, dim). Without this, the winner
// was simply whichever row IngestTraits happened to reach last, because
// AddTraitValue is an INSERT OR REPLACE. That is bad two ways: an
// exactly-matched value could be silently overwritten by an aggregate's
// collective mean, and — since Hardening Task 5 — the stored
// trait_value.resolution flag would then describe the slot by CSV row order
// rather than by what is actually true of it. A flag whose value depends on
// input ordering is not a flag a consumer can filter on.
//
// The rule is therefore explicit and order-independent in the only respect
// that matters: an EXACT match always outranks a normalised one. Among rows
// of equal rank the first in row order wins, which keeps the previous
// behavior for the ordinary case (a vocabulary listing the same taxon twice)
// and keeps the outcome deterministic for a given input file.
//
// A row whose Dim does not parse is always treated as a winner, so the write
// loop still reaches it and still fails the ingest with the same error as
// before rather than having it quietly filtered out here.
//
// Precedence among CONTENDING rows is ruleRank, not "first row wins": an
// exact match always outranks every normalised one, and among normalised
// rules an unflagged (pure spelling) rule always outranks a flagged
// (circumscription-changing) one. Only two rows resolved via the identical
// rule fall back to row order — see ruleRank's doc comment.
func selectTraitWinners(rows []TraitRow, resolved map[string]traitResolution) map[int]bool {
	best := make(map[traitSlot]int, len(rows))
	winners := make(map[int]bool, len(rows))
	for i, row := range rows {
		res := resolved[domain.Canonicalize(row.Taxon)]
		if !res.matched || res.ambiguous {
			continue
		}
		dim, err := domain.ParseTraitDim(row.Dim)
		if err != nil {
			winners[i] = true
			continue
		}
		slot := traitSlot{conceptID: res.conceptID, dim: string(dim)}
		prev, seen := best[slot]
		if !seen {
			best[slot] = i
			winners[i] = true
			continue
		}
		// A challenger displaces the incumbent only if it strictly outranks
		// it (ruleRank); a tie (both rows resolved via the same rule) keeps
		// the incumbent, so the first row in CSV order still wins the
		// ordinary case of a vocabulary listing the same taxon twice.
		prevRule := resolved[domain.Canonicalize(rows[prev].Taxon)].rule
		if ruleRank[res.rule] < ruleRank[prevRule] {
			delete(winners, prev)
			best[slot] = i
			winners[i] = true
		}
	}
	return winners
}

// traitValueFor builds the domain.TraitValue one matched trait row writes,
// parsing its dimension and stamping the normalisation rule that resolved
// the row's taxon name. Split out of IngestTraits' loop purely to keep that
// loop readable.
//
// Resolution stays EMPTY for domain.RuleExact: an exact canonical match
// needed no normalisation, and writing "exact" would turn the ordinary case
// into a stored assertion (and a rendered wire field) for every one of the
// ~117k trait_value rows. Non-empty means "this value reached its concept
// through a rewrite" — see domain.TraitValue.Resolution.
func traitValueFor(row TraitRow, meta domain.TraitVocabMeta, rule domain.NormalizationRule) (domain.TraitValue, error) {
	dim, err := domain.ParseTraitDim(row.Dim)
	if err != nil {
		return domain.TraitValue{}, err
	}
	resolution := ""
	if rule != domain.RuleExact {
		resolution = string(rule)
	}
	return domain.TraitValue{
		Vocab:        meta.Vocab,
		VocabVersion: meta.Version,
		Dim:          dim,
		Value:        row.Value,
		NicheWidth:   row.NicheWidth,
		NSystems:     row.NSystems,
		Resolution:   resolution,
	}, nil
}

// traitTally accumulates the per-row bookkeeping IngestTraits needs for the
// parts of TraitIngestReport that are not plain counters: the unmatched and
// flagged name samples, and the per-normalisation-rule breakdown.
type traitTally struct {
	unmatched map[string]bool
	flagged   map[string]bool
	ruleRows  map[domain.NormalizationRule]int
	ruleTaxa  map[domain.NormalizationRule]map[string]bool
}

func newTraitTally() *traitTally {
	return &traitTally{
		unmatched: map[string]bool{},
		flagged:   map[string]bool{},
		ruleRows:  map[domain.NormalizationRule]int{},
		ruleTaxa:  map[domain.NormalizationRule]map[string]bool{},
	}
}

func (t *traitTally) countUnmatched(taxon string) { t.unmatched[taxon] = true }

// countMatched records one written row. Rows that resolved on the plain
// exact key are deliberately NOT recorded: TraitIngestReport.Normalized
// exists to show what normalisation ADDED, and an exact hit added nothing.
func (t *traitTally) countMatched(taxon string, rule domain.NormalizationRule) {
	if rule == domain.RuleExact {
		return
	}
	t.ruleRows[rule]++
	if t.ruleTaxa[rule] == nil {
		t.ruleTaxa[rule] = map[string]bool{}
	}
	t.ruleTaxa[rule][taxon] = true
	if rule.Flagged() {
		t.flagged[taxon] = true
	}
}

// report writes the accumulated samples and per-rule counts onto r. The
// RuleCount slice is sorted by rule name: the report is printed by
// "hostus ingest" and compared across runs, and Go map iteration order is
// randomized.
func (t *traitTally) report(r *TraitIngestReport) {
	r.UnmatchedSample = sortedSample(t.unmatched, unmatchedSampleCap)
	r.FlaggedSample = sortedSample(t.flagged, unmatchedSampleCap)
	if len(t.ruleRows) == 0 {
		return
	}
	out := make([]RuleCount, 0, len(t.ruleRows))
	for rule, n := range t.ruleRows {
		out = append(out, RuleCount{Rule: rule, Rows: n, Taxa: len(t.ruleTaxa[rule]), Flagged: rule.Flagged()})
	}
	// out[i].Rule <= out[j].Rule is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY: out carries exactly one entry per DISTINCT
	// rule (it is built from a map keyed by rule), so two different indices
	// never hold equal Rules and sort.Slice never calls less(i, i) — the
	// boundary the mutant moves is unreachable, and no test can observe it.
	// Same provable-equivalence class as sortedSample's documented cap
	// boundary below.
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	r.Normalized = out
}

// checkVocabIdentity reconciles the two independent sources of a trait
// value's (vocab, version) identity — the manifest entry (meta) and the
// canonical CSV's own vocab/vocab_version columns (row) — and refuses the
// whole ingest if they disagree, naming both sides.
//
// This is not defensive paranoia, it is the only thing standing between a
// misconfigured manifest and silently wrong data. trait_value rows are
// written under meta's identity while the CSV's rows carry their own; a
// mismatch on VERSION makes Repository.Traits' LEFT JOIN onto
// trait_vocabulary miss, silently degrading Taxonomy to "" on the wire. A
// mismatch on VOCAB is worse: pointing an `id: eive` manifest entry at
// Tichý's canonical CSV would store Tichý's 1..12 values under vocab=eive,
// which domain.ScaleFor then renders on the 0..10 normalized EIVE scale —
// an invented scale, and exactly the cross-vocabulary merge PoC P10
// forbids.
//
// It runs BEFORE phase 1 (resolution) so a mismatched vocabulary costs no
// repository work and, more importantly, opens no transaction at all —
// "write nothing" is guaranteed structurally, not by a rollback.
func checkVocabIdentity(rows []TraitRow, meta domain.TraitVocabMeta) error {
	for _, row := range rows {
		if row.Vocab != string(meta.Vocab) {
			return fmt.Errorf("application: trait row for taxon %q declares vocab %q but the manifest pins vocab %q: refusing to ingest a vocabulary under another vocabulary's identity", row.Taxon, row.Vocab, meta.Vocab)
		}
		if row.VocabVersion != meta.Version {
			return fmt.Errorf("application: trait row for taxon %q declares vocab %s version %q but the manifest pins version %q: refusing to ingest, the pinned version must match the data", row.Taxon, row.Vocab, row.VocabVersion, meta.Version)
		}
	}
	return nil
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

// resolveTraitName walks domain.NameCandidates' deterministic ladder for
// the already canonicalized name canon and returns the FIRST candidate key
// the index answers at all, classified into exactly one of: matched to a
// single concept id, matched=false (no key answered), or ambiguous=true
// (the answering key resolves to two or more distinct concepts). It never
// picks a concept when ambiguous.
//
// Two properties matter and are worth stating explicitly:
//
//   - The ladder's first key is always domain.Canonicalize(canon) itself,
//     so a name that resolved (or was ambiguous) before name normalisation
//     existed resolves identically now. Normalisation can only ever act on
//     names the exact key left with ZERO candidates — it cannot re-route an
//     existing hit onto a different concept.
//   - An AMBIGUOUS key stops the walk rather than letting a later,
//     semantically looser rule "rescue" it. A key that answers with several
//     distinct concepts is a genuine ambiguity about which taxon the source
//     meant; continuing down the ladder until some rule happens to produce
//     a single-concept key would be guessing, which is precisely what the
//     rest of this crosswalk refuses to do.
func resolveTraitName(ctx context.Context, repo output.Repository, canon string) (traitResolution, error) {
	for _, cand := range domain.NameCandidates(canon) {
		candidates, err := repo.MatchExact(ctx, cand.Key)
		if err != nil {
			return traitResolution{}, err
		}
		if len(candidates) == 0 {
			continue
		}
		distinct := make(map[string]bool, len(candidates))
		for _, c := range candidates {
			distinct[c.Concept.ID] = true
		}
		if len(distinct) > 1 {
			return traitResolution{ambiguous: true, rule: cand.Rule}, nil
		}
		return traitResolution{conceptID: candidates[0].Concept.ID, matched: true, rule: cand.Rule}, nil
	}
	return traitResolution{}, nil
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
