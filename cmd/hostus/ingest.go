package main

import (
	"errors"
	"fmt"
	"io"

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

	report, err := app.Ingest(cmd.Context(), datasetPath, dbPath)
	if err != nil {
		return err
	}

	printIngestReport(cmd.OutOrStdout(), report)
	return nil
}

// printIngestReport renders report as one line per backbone, so an operator
// running "hostus ingest" (and T9's smoke test) can see counts without
// reading logs.
func printIngestReport(w io.Writer, report application.IngestReport) {
	_, _ = fmt.Fprintln(w, "Ingest complete:")
	for _, b := range report.Backbones {
		_, _ = fmt.Fprintf(w, "  %s: names=%d concepts=%d synonyms=%d orphaned=%d\n",
			b.ID, b.Names, b.Concepts, b.Synonyms, b.Orphaned)
	}
}
