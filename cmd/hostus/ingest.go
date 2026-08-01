package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/application"
)

// ingestCmdName is shared with tests so the "ingest" literal only needs to
// be spelled once outside of _test.go files.
const ingestCmdName = "ingest"

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

	report, traitReports, err := app.Ingest(cmd.Context(), datasetPath, dbPath)
	if err != nil {
		return err
	}

	printIngestReport(cmd.OutOrStdout(), report)
	printTraitReports(cmd.OutOrStdout(), traitReports)
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
	if b.OtherRanks == 0 {
		return
	}
	parts := make([]string, len(b.OtherRankSample))
	for i, rc := range b.OtherRankSample {
		verbatim := rc.Verbatim
		if verbatim == "" {
			verbatim = "(empty)"
		}
		parts[i] = fmt.Sprintf("%s %d", verbatim, rc.Count)
	}
	_, _ = fmt.Fprintf(w, "    ranks: other=%d (%s)\n", b.OtherRanks, strings.Join(parts, ", "))
}

// printRedistributionNotice prints one "hinweis:" line for id if
// redistribution is set and not "allowed" — see printIngestReport's doc
// comment. A blank redistribution (should not happen once the manifest
// schema enforces it, but defensively handled) is treated the same as
// "allowed": silent, no notice.
func printRedistributionNotice(w io.Writer, id, redistribution string) {
	if redistribution == "" || redistribution == "allowed" {
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
		if len(r.UnmatchedSample) > 0 {
			_, _ = fmt.Fprintf(w, "    unmatched sample: %s\n", strings.Join(r.UnmatchedSample, ", "))
		}
		printRedistributionNotice(w, r.Vocab, r.Redistribution)
	}
}
