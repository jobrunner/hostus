package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/application"
)

// ingestCmdName is shared with tests so the "ingest" literal only needs to
// be spelled once outside of _test.go files.
const ingestCmdName = "ingest"

// redistributionAllowed is domain.RedistributionAllowed's spelling as it
// arrives here: the report DTOs carry redistribution as a plain string, and
// this file must not import internal/domain to compare against it.
const redistributionAllowed = "allowed"

// newIngestCmd builds "hostus ingest --dataset dataset.yaml --db
// hostus.sqlite": it parses+validates the manifest, opens (or creates) the
// SQLite database, and runs the WCVP-backed ingest use case against it.
func newIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   ingestCmdName,
		Short: "Ingest a pinned taxonomy dataset (dataset.yaml manifest) into a SQLite database",
		RunE:  runIngest,
	}
	cmd.Flags().String("dataset", "", "path to the dataset.yaml manifest to ingest")
	cmd.Flags().String("db", "", "path to the SQLite database to ingest into")
	return cmd
}

// runIngest wires cmd's flags into internal/app.Ingest — the composition
// root's manifest-parse + wcvp.Read + sqlite.Open + application.Ingest
// pipeline — and prints the resulting per-backbone report.
func runIngest(cmd *cobra.Command, _ []string) error {
	datasetPath, err := cmd.Flags().GetString("dataset")
	if err != nil {
		return err
	}
	if datasetPath == "" {
		return errors.New("ingest: --dataset is required")
	}

	dbPath, err := cmd.Flags().GetString("db")
	if err != nil {
		return err
	}
	if dbPath == "" {
		return errors.New("ingest: --db is required")
	}

	reports, err := app.Ingest(cmd.Context(), datasetPath, dbPath)
	if err != nil {
		return err
	}

	printIngestReport(cmd.OutOrStdout(), reports.Backbone)
	printTraitReports(cmd.OutOrStdout(), reports.Traits)
	printXrefReports(cmd.OutOrStdout(), reports.Xrefs)
	printConceptSourceReports(cmd.OutOrStdout(), reports.ConceptSources)
	return nil
}

// printIngestReport renders report as one line per backbone, so an operator
// running "hostus ingest" (and T9's smoke test) can see counts without
// reading logs. A backbone whose manifest-pinned redistribution is not
// "allowed" additionally gets a "hinweis:" line — local ingest is never
// gated by redistribution, but the operator must SEE that this source
// cannot be shipped in an exported bundle without --force-include-restricted
// (see internal/adapters/sqlite.ExportBundle).
func printIngestReport(w io.Writer, report application.IngestReport) {
	_, _ = fmt.Fprintln(w, "Ingest complete:")
	for _, b := range report.Backbones {
		_, _ = fmt.Fprintf(w, "  %s: names=%d concepts=%d synonyms=%d orphaned=%d\n",
			b.ID, b.Names, b.Concepts, b.Synonyms, b.Orphaned)
		printOtherRanksNotice(w, b)
		printRedistributionNotice(w, b.ID, b.Redistribution)
	}
}

// printOtherRanksNotice prints one "ranks: other=N (...)" line when b
// carries any taxon rows whose "taxonrank" spelling didn't match a
// canonical domain.Rank (see application.ParseRankLenient / domain.RankOther).
// This is what makes an exotic rank spelling (e.g. WCVP's "proles") VISIBLE
// to whoever runs "hostus ingest" — the ingest itself never aborts on it —
// mirroring printTraitReports' "unmatched sample" line below.
func printOtherRanksNotice(w io.Writer, b application.BackboneReport) {
	printOtherRanksLine(w, b.OtherRanks, b.OtherRankSample)
}

// printRedistributionNotice prints one "hinweis:" line for id if
// redistribution is set and not "allowed" — see printIngestReport's doc
// comment. A blank redistribution (should not happen once the manifest
// schema enforces it, but defensively handled) is treated the same as
// "allowed": silent, no notice.
func printRedistributionNotice(w io.Writer, id, redistribution string) {
	if redistribution == "" || redistribution == redistributionAllowed {
		return
	}
	_, _ = fmt.Fprintf(w, "  hinweis: %s (redistribution=%s) — lokal genutzt, nicht redistribuierbar\n", id, redistribution)
}

// printTraitReports renders one line per ingested trait vocabulary,
// including its UnmatchedSample — the crosswalk from a trait table's bare
// taxon name to a hostus taxon_concept is lossy by construction (PoC P6:
// the trait tables carry no external taxon id), so this is where that loss
// becomes VISIBLE to whoever runs "hostus ingest", not silently swallowed.
func printTraitReports(w io.Writer, reports []application.TraitIngestReport) {
	if len(reports) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "Trait vocabularies:")
	for _, r := range reports {
		_, _ = fmt.Fprintf(w, "  %s: rows=%d matched=%d unmatched=%d ambiguous=%d\n",
			r.Vocab, r.Rows, r.Matched, r.Unmatched, r.Ambiguous)
		for _, n := range r.Normalized {
			flag := ""
			if n.Flagged {
				flag = " [flagged: circumscriptions equated, not identical]"
			}
			_, _ = fmt.Fprintf(w, "    normalized %s: rows=%d taxa=%d%s\n", n.Rule, n.Rows, n.Taxa, flag)
		}
		if len(r.FlaggedSample) > 0 {
			_, _ = fmt.Fprintf(w, "    flagged sample: %s\n", strings.Join(r.FlaggedSample, ", "))
		}
		if len(r.UnmatchedSample) > 0 {
			_, _ = fmt.Fprintf(w, "    unmatched sample: %s\n", strings.Join(r.UnmatchedSample, ", "))
		}
		printRedistributionNotice(w, r.Vocab, r.Redistribution)
	}
}

// printXrefReports renders one line per ingested xref source, including its
// per-authority coverage and both conflict-sample lines — mirroring
// printTraitReports' visibility posture: the ID-based join's two loss modes
// (unmatched join ids, conflicting external ids) must be seen by whoever
// runs "hostus ingest", never silently swallowed.
func printXrefReports(w io.Writer, reports []application.XrefIngestReport) {
	if len(reports) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "Xref sources:")
	for _, r := range reports {
		_, _ = fmt.Fprintf(w, "  %s: rows=%d matched=%d unmatched=%d conflicting=%d\n",
			r.Source, r.Rows, r.Matched, r.Unmatched, r.Conflicting)
		for _, authority := range sortedKeys(r.PerAuthority) {
			_, _ = fmt.Fprintf(w, "    %s: concepts=%d\n", authority, r.PerAuthority[authority])
		}
		for _, authority := range sortedKeys(r.MultiPerAuthority) {
			_, _ = fmt.Fprintf(w, "    %s: %d concept(s) hold more than one id (not a conflict)\n", authority, r.MultiPerAuthority[authority])
		}
		if len(r.UnmatchedSample) > 0 {
			_, _ = fmt.Fprintf(w, "    unmatched sample: %s\n", strings.Join(r.UnmatchedSample, ", "))
		}
		if len(r.ConflictSample) > 0 {
			_, _ = fmt.Fprintf(w, "    conflict sample: %s\n", strings.Join(r.ConflictSample, ", "))
		}
		printRedistributionNotice(w, r.Source, r.Redistribution)
	}
}

// sortedKeys returns m's keys in sorted order, so printXrefReports' output
// is deterministic across runs — Go map iteration order is randomized.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// printConceptSourceReports renders one line per ingested concept source
// (SP5). Its visibility posture matches printTraitReports/printXrefReports,
// and the four loss counters it prints are the whole point: a CDM ingest
// legitimately writes fewer relations than it read, and the operator must be
// able to see WHY — dropped misapplied-name rows, unresolvable ends,
// unresolvable parents, reader-level bad rows — rather than being told a
// number that quietly does not add up.
func printConceptSourceReports(w io.Writer, reports []application.CDMIngestReport) {
	if len(reports) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "Concept sources:")
	for _, r := range reports {
		_, _ = fmt.Fprintf(w, "  %s: concepts=%d written=%d sec_spaces=%d relations=%d written=%d\n",
			r.Backbone, r.Concepts, r.ConceptsWritten, r.SecReferences, r.Relations, r.RelationsWritten)
		for _, rel := range sortedKeys(r.PerRelationType) {
			_, _ = fmt.Fprintf(w, "    %s: %d\n", rel, r.PerRelationType[rel])
		}
		_, _ = fmt.Fprintf(w, "    dropped: misapplied/non-concept=%d unresolved ends=%d unresolved parents=%d reader errors=%d\n",
			r.NonConcept, r.UnresolvedEnds, r.UnresolvedParents, r.ReaderErrors)
		_, _ = fmt.Fprintf(w, "    unknown concept-relation flag=%d, concepts without sec.=%d, empty status=%d\n",
			r.UnknownFlag, r.ConceptsWithoutSec, r.EmptyStatus)
		printOtherRanksLine(w, r.OtherRanks, r.OtherRankSample)
		if len(r.NonConceptSample) > 0 {
			_, _ = fmt.Fprintf(w, "    non-concept sample: %s\n", strings.Join(r.NonConceptSample, ", "))
		}
		if len(r.UnresolvedSample) > 0 {
			_, _ = fmt.Fprintf(w, "    unresolved sample: %s\n", strings.Join(r.UnresolvedSample, ", "))
		}
		printRedistributionNotice(w, r.Backbone, r.Redistribution)
	}
}

// printOtherRanksLine is the shared rendering of an "other ranks" tally,
// extracted so printOtherRanksNotice (backbones) and the concept-source
// report above cannot drift apart.
func printOtherRanksLine(w io.Writer, other int, sample []application.RankVerbatimCount) {
	if other == 0 {
		return
	}
	parts := make([]string, len(sample))
	for i, rc := range sample {
		verbatim := rc.Verbatim
		if verbatim == "" {
			verbatim = "(empty)"
		}
		parts[i] = fmt.Sprintf("%s %d", verbatim, rc.Count)
	}
	_, _ = fmt.Fprintf(w, "    ranks: other=%d (%s)\n", other, strings.Join(parts, ", "))
}
