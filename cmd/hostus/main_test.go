package main

import (
	"os"
	"testing"
)

// TestRun exercises main's run() end to end via os.Args (the one thing
// version_test.go and serve_test.go can't reach, since they build and
// execute cobra.Command values directly rather than going through run()'s
// os.Args-driven ExecuteContext + exit-code mapping).
func TestRun(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{hostusCmdName, versionCmdName}
	if got := run(); got != 0 {
		t.Fatalf("version: got exit code %d, want 0", got)
	}

	os.Args = []string{hostusCmdName, ingestCmdName}
	if got := run(); got != 1 {
		t.Fatalf("ingest (not implemented): got exit code %d, want 1", got)
	}
}
