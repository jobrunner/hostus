// Package main is the hostus CLI entry point: a thin wrapper around the
// cobra command tree in this package (root.go, serve.go, version.go,
// ingest.go, validate.go, bundle.go).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	os.Exit(run())
}

// run wires the interrupt/SIGTERM signal context that serve's graceful
// shutdown relies on, executes the cobra command tree, and maps any
// returned error to a process exit code.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "hostus: %v\n", err)
		return 1
	}
	return 0
}
