package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNotImplemented is returned by the ingest/validate/bundle stubs. Their
// real implementations land in SP1/SP2; until then they must fail loudly
// rather than silently succeed.
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
		Short: "hostus - read-only taxonomy gateway for vascular plants (GBIF proxy)",
		Long: `hostus proxies and caches GBIF species-search requests for a frontend
autosuggest field, grouping synonyms under their accepted taxa.

Running hostus with no subcommand is equivalent to "hostus serve".`,
		RunE:          runServe,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: ./config.yaml)")
	addServeFlags(root)

	root.AddCommand(newVersionCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newBundleCmd())

	return root
}
