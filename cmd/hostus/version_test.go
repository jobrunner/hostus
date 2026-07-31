package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestVersionCommand is the brief's Step 1 RED test: `hostus version` must
// print output that at least identifies itself as hostus.
func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{versionCmdName})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hostus") {
		t.Fatalf("got %q", buf.String())
	}
}

// TestVersionCommandContainsBuildInfo pins that all three ldflags-injected
// values (Version/Commit/BuildDate) show up in the output, not just the
// static "hostus" label.
func TestVersionCommandContainsBuildInfo(t *testing.T) {
	oldV, oldC, oldB := Version, Commit, BuildDate
	Version, Commit, BuildDate = "v9.9.9", "deadbeef", "2026-07-31"
	defer func() { Version, Commit, BuildDate = oldV, oldC, oldB }()

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{versionCmdName})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"v9.9.9", "deadbeef", "2026-07-31"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q missing %q", out, want)
		}
	}
}

// TestIngestWithoutFlagsFailsFast documents that "ingest" is no longer the
// SP0 errNotImplemented stub (see ingest_test.go for its real behavior):
// invoked bare it now fails because --dataset/--db are required, not
// because the command is unimplemented.
func TestIngestWithoutFlagsFailsFast(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{ingestCmdName})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want an error when --dataset/--db are missing, got nil")
	}
	if errors.Is(err, errNotImplemented) {
		t.Fatalf("got errNotImplemented, want the real --dataset-required error (see ingest_test.go)")
	}
}

// TestValidateWithoutFlagsFailsFast is validate's counterpart to
// TestIngestWithoutFlagsFailsFast (see validate_test.go for its real
// behavior).
func TestValidateWithoutFlagsFailsFast(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{validateCmdName})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want an error when --dataset is missing, got nil")
	}
	if errors.Is(err, errNotImplemented) {
		t.Fatalf("got errNotImplemented, want the real --dataset-required error (see validate_test.go)")
	}
}

func TestBundleReturnsNotImplemented(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bundle"})
	err := cmd.Execute()
	if !errors.Is(err, errNotImplemented) {
		t.Fatalf("got %v, want errNotImplemented", err)
	}
}
