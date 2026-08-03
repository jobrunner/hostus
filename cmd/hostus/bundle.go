package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/app"
)

// bundleCmdName is shared with tests so the "bundle" literal only needs to
// be spelled once outside of _test.go files.
const bundleCmdName = "bundle"

// newBundleCmd builds "hostus bundle --db hostus.sqlite --area AUT --out
// bundle.sqlite [--snapshot v1]": it exports an offline, standalone
// SQLite/FTS5 bundle scoped to --area (or the whole database, if --area is
// empty) from the SQLite database at --db.
func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   bundleCmdName,
		Short: "Export an offline SQLite/FTS5 bundle scoped to an area",
		RunE:  runBundle,
	}
	cmd.Flags().String("db", "", "path to the source SQLite database to bundle from")
	cmd.Flags().String("area", "", "area identifier to scope the bundle to (empty = whole database)")
	cmd.Flags().String("out", "", "output path for the bundle")
	cmd.Flags().String("snapshot", "", "snapshot version recorded into the bundle's bundle_meta table")
	return cmd
}

// runBundle wires cmd's flags into internal/app.Bundle and prints the
// resulting BundleReport.
func runBundle(cmd *cobra.Command, _ []string) error {
	dbPath, err := cmd.Flags().GetString("db")
	if err != nil {
		return err
	}
	if dbPath == "" {
		return errors.New("bundle: --db is required")
	}

	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	if out == "" {
		return errors.New("bundle: --out is required")
	}

	area, err := cmd.Flags().GetString("area")
	if err != nil {
		return err
	}
	snapshot, err := cmd.Flags().GetString("snapshot")
	if err != nil {
		return err
	}

	report, err := app.Bundle(cmd.Context(), dbPath, out, sqlite.BundleOpts{
		Area:            area,
		SnapshotVersion: snapshot,
	})
	if err != nil {
		return err
	}

	printBundleReport(cmd.OutOrStdout(), report)
	return nil
}

// printBundleReport renders report as one line, so an operator running
// "hostus bundle" can see what was written without reading logs.
func printBundleReport(w io.Writer, report sqlite.BundleReport) {
	_, _ = fmt.Fprintf(w, "Bundle complete: %s (concepts=%d names=%d areas=%d)\n",
		report.Path, report.Concepts, report.Names, report.Areas)
}
