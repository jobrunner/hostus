package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/jobrunner/hostus/internal/app"
)

const exportCrosswalkCmdName = "export-crosswalk"

func newExportCrosswalkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   exportCrosswalkCmdName,
		Short: "Export eurosl_crosswalk.csv + aggregate_members.csv for situs' file-based species ingest",
		RunE:  runExportCrosswalk,
	}
	cmd.Flags().String("db", "", "path to the source SQLite database")
	cmd.Flags().String("out-dir", "", "output directory for eurosl_crosswalk.csv and aggregate_members.csv")
	return cmd
}

func runExportCrosswalk(cmd *cobra.Command, _ []string) error {
	dbPath, err := cmd.Flags().GetString("db")
	if err != nil {
		return err
	}
	if dbPath == "" {
		return errors.New("export-crosswalk: --db is required")
	}

	outDir, err := cmd.Flags().GetString("out-dir")
	if err != nil {
		return err
	}
	if outDir == "" {
		return errors.New("export-crosswalk: --out-dir is required")
	}

	report, err := app.ExportCrosswalk(cmd.Context(), dbPath, outDir)
	if err != nil {
		return err
	}

	printExportCrosswalkReport(cmd.OutOrStdout(), outDir, report)
	return nil
}

func printExportCrosswalkReport(w io.Writer, outDir string, report app.ExportCrosswalkReport) {
	_, _ = fmt.Fprintf(w, "Crosswalk export complete: %s (crosswalk_rows=%d member_rows=%d collisions=%d)\n",
		outDir, report.CrosswalkRows, report.MemberRows, len(report.NameCollisions))
	for _, c := range report.NameCollisions {
		_, _ = fmt.Fprintf(w, "  collision: %q -> fall_a=%s fall_b=%s\n", c.Name, c.FallAConceptID, c.FallBConceptID)
	}
}
