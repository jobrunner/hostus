package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestValidateCommand_ValidManifest_ExitsCleanAndPrintsOK confirms "hostus
// validate --dataset <manifest>" succeeds (no error, i.e. exit 0 once run()
// maps it) and reports something a human can read on stdout, for a manifest
// that passes both the embedded JSON Schema and the strict YAML decode.
func TestValidateCommand_ValidManifest_ExitsCleanAndPrintsOK(t *testing.T) {
	cmd := newValidateCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dataset=testdata/dataset.yaml"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "OK") {
		t.Errorf("stdout = %q, want it to contain %q", out.String(), "OK")
	}
}

// TestValidateCommand_InvalidManifest_ReturnsError confirms an
// unknown-field manifest is rejected (non-zero exit once run() maps the
// returned error), never silently accepted, and never a DB write (validate
// never opens a database at all — it has no --db flag).
func TestValidateCommand_InvalidManifest_ReturnsError(t *testing.T) {
	cmd := newValidateCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dataset=testdata/dataset-invalid.yaml"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error for an unknown-field manifest, got nil")
	}
}

// TestValidateCommand_MissingDatasetFlag_ReturnsError confirms the command
// fails fast (rather than panicking on an empty path) when --dataset is not
// supplied at all.
func TestValidateCommand_MissingDatasetFlag_ReturnsError(t *testing.T) {
	cmd := newValidateCmd()
	cmd.SetOut(new(bytes.Buffer))

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("Execute: want an error when --dataset is missing, got nil")
	}
}

// TestValidateCommand_RegisteredOnRoot confirms "hostus validate" is wired
// into the command tree, not just constructible in isolation.
func TestValidateCommand_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{validateCmdName})
	if err != nil {
		t.Fatalf("Find(validate): %v", err)
	}
	if cmd.Use != validateCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, validateCmdName)
	}
}
