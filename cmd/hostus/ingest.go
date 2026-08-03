package main

import "github.com/spf13/cobra"

// ingestCmdName is shared with tests so the "ingest" literal only needs to
// be spelled once outside of _test.go files.
const ingestCmdName = "ingest"

// newIngestCmd is a stub for the SP1 dataset-ingest command: it declares
// the intended flag surface but always returns errNotImplemented.
func newIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   ingestCmdName,
		Short: "Ingest a GBIF taxonomy dataset dump (not implemented yet, see SP1)",
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented
		},
	}
	cmd.Flags().String("dataset", "", "path to the GBIF dataset dump to ingest")
	return cmd
}
