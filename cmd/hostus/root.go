package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNotImplemented was returned by the SP0-era ingest/validate/bundle
// command stubs before their SP1/SP2 implementations landed. It is kept
// (and still asserted against in version_test.go) so those tests keep
// documenting that the real commands no longer hit the old stub path.
var errNotImplemented = errors.New("not implemented yet (planned for SP1/SP2)")

// hostusCmdName is shared with tests so the "hostus" literal only needs to
// be spelled once outside of _test.go files.
const hostusCmdName = "hostus"

// cfgFile is bound to the persistent --config flag and consumed by
// config.Load in serve's RunE.
var cfgFile string

// newRootCmd builds the hostus command tree. The root command's own RunE
// defaults to serve, so `hostus` with no subcommand starts the HTTP server
// exactly like `hostus serve` does.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   hostusCmdName,
		Short: "hostus - local naming service for vascular plants",
		Long: `hostus serves a local, versioned multi-backbone taxonomy
index (COL XR, WCVP/POWO, Euro+Med, FloraVeg.EU) for a frontend autosuggest
field, grouping synonyms under their accepted taxa. It is backed by an
on-disk SQLite/FTS5 index fed by pinned ingest artifacts, not a live
pass-through to any single upstream source.

Running hostus with no subcommand is equivalent to "hostus serve".`,
		RunE:          runServe,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: ./config.yaml)")
	addServeFlags(root)

	root.AddCommand(newVersionCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newBundleCmd())
	root.AddCommand(newExportCrosswalkCmd())

	return root
}
