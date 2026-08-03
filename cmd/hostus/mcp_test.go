package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMCPCommandRejectsBadPort confirms "hostus mcp" validates config the
// same way "hostus serve" does (via the shared addServeFlags/bindServeFlags
// wiring) — a bad port must fail fast, before ever touching app.New or the
// stdio transport.
func TestMCPCommandRejectsBadPort(t *testing.T) {
	cmd := newMCPCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--port=99999"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("want error for out-of-range port, got nil")
	}
}

// TestMCPCommandRegisteredOnRoot confirms "hostus mcp" is wired into the
// command tree (root.go), not just constructible in isolation.
func TestMCPCommandRegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{mcpCmdName})
	if err != nil {
		t.Fatalf("Find(mcp): %v", err)
	}
	if cmd.Use != mcpCmdName {
		t.Fatalf("got command %q, want %q", cmd.Use, mcpCmdName)
	}
}

// TestRunMCPShutsDownCleanlyOnStdinEOF drives runMCP end to end: it
// redirects os.Stdin to a pipe that's immediately closed (so the stdio MCP
// transport sees EOF right away, the same way it would if a connecting
// client disconnected), and confirms both the HTTP listener (started in the
// background) and the stdio session shut down cleanly rather than hanging
// or erroring.
func TestRunMCPShutsDownCleanlyOnStdinEOF(t *testing.T) {
	port := freePort(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if err := w.Close(); err != nil { // immediate EOF on the read side
		t.Fatal(err)
	}

	cmd := newMCPCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--host=127.0.0.1", "--port=" + strconv.Itoa(port)})

	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want clean shutdown on stdin EOF, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mcp command did not shut down in time")
	}
}

// TestRunMCPSurfacesHTTPErrorWhenStdioIsClean covers the rarer branch where
// the HTTP listener fails (here: the port is already taken) while the stdio
// session itself ends cleanly (immediate EOF, as above). runMCP must still
// surface the HTTP error rather than silently returning nil just because
// the stdio half was fine.
func TestRunMCPSurfacesHTTPErrorWhenStdioIsClean(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not reserve a port to occupy: %v", err)
	}
	defer func() { _ = l.Close() }()
	port := l.Addr().(*net.TCPAddr).Port

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := newMCPCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--host=127.0.0.1", "--port=" + strconv.Itoa(port)})

	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want the HTTP bind failure to be surfaced, got nil")
		}
		if !strings.Contains(err.Error(), "bind") && !strings.Contains(err.Error(), "address already in use") {
			t.Fatalf("want a bind-failure error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mcp command did not shut down in time")
	}
}

// TestRunMCPSurfacesHTTPErrorPromptlyWhileStdioStaysOpen is the regression
// test for the concurrency bug found in review: the HTTP listener failing
// (port already bound) MUST be noticed and returned promptly even while a
// real MCP client is still connected over stdio (stdin deliberately left
// open here, not EOF'd) — the normal shape of a long-lived Claude Code
// session. Before the fix, runMCP only read the buffered HTTP error AFTER
// ServeStdio returned, so a bind failure during an open stdio session sat
// unnoticed until the client eventually disconnected. This test fails (by
// timing out) against that old, sequential implementation.
func TestRunMCPSurfacesHTTPErrorPromptlyWhileStdioStaysOpen(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not reserve a port to occupy: %v", err)
	}
	defer func() { _ = l.Close() }()
	port := l.Addr().(*net.TCPAddr).Port

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = w.Close() // unblock/clean up the still-open write end
	})
	// Deliberately do NOT close w: stdin stays open, as it would for a real,
	// still-connected MCP client that simply hasn't sent anything yet.

	cmd := newMCPCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"--host=127.0.0.1", "--port=" + strconv.Itoa(port)})

	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want the HTTP bind failure to be surfaced, got nil")
		}
		if !strings.Contains(err.Error(), "bind") && !strings.Contains(err.Error(), "address already in use") {
			t.Fatalf("want a bind-failure error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mcp command did not surface the HTTP bind error promptly while stdio stayed open — " +
			"the HTTP and stdio failure paths are not being watched concurrently")
	}
}
