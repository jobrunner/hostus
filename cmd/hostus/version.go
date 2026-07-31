package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version, Commit and BuildDate are injected at build time via the
// Makefile's LDFLAGS ("-X main.Version=... -X main.Commit=... -X
// main.BuildDate=..."). They keep placeholder values for `go run`/`go test`
// and any build that skips the Makefile.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// versionCmdName is shared with version_test.go so the "version" literal
// only needs to be spelled once outside of that file.
const versionCmdName = "version"

// newVersionCmd prints the ldflags-injected build info.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   versionCmdName,
		Short: "Print the hostus version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "hostus %s\n  commit:     %s\n  build date: %s\n", Version, Commit, BuildDate)
			return err
		},
	}
}
