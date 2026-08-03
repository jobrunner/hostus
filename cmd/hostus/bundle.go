package main

import "github.com/spf13/cobra"

// newBundleCmd is a stub for the SP2 area-bundle command: it declares the
// intended flag surface but always returns errNotImplemented.
func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Bundle a taxonomy snapshot for an area (not implemented yet, see SP2)",
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented
		},
	}
	cmd.Flags().String("area", "", "area identifier to bundle taxa for")
	cmd.Flags().String("out", "", "output path for the bundle")
	return cmd
}
