package main

import "github.com/spf13/cobra"

// newValidateCmd is a stub for the SP1 dataset-validate command: it
// declares the intended flag surface but always returns errNotImplemented.
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a GBIF taxonomy dataset dump (not implemented yet, see SP1)",
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented
		},
	}
	cmd.Flags().String("dataset", "", "path to the GBIF dataset dump to validate")
	return cmd
}
