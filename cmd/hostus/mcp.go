package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	mcpAdapter "github.com/jobrunner/hostus/internal/adapters/mcp"
	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/config"
)

// mcpCmdName is shared with mcp_test.go so the "mcp" literal only needs to
// be spelled once outside of that file.
const mcpCmdName = "mcp"

// newMCPCmd builds "hostus mcp": a stdio debug MCP server that exposes a
// *live* hostus instance's buffered logs and spans to an MCP client (e.g.
// Claude Code). It shares serve's flags because it also runs the HTTP
// listener in the background — the whole point is inspecting a real,
// traffic-serving instance, not a synthetic one.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   mcpCmdName,
		Short: "Serve hostus' logs and spans as a stdio MCP for AI-assisted debugging",
		Long: `hostus mcp starts the same HTTP server as "hostus serve" in the
background and additionally serves a read-only Model Context Protocol
server over stdio, exposing the running instance's buffered logs and
spans (get_recent_logs, tail_errors, get_trace, list_spans) to an MCP
client such as Claude Code.

stdout is reserved for MCP JSON-RPC framing; all of this command's own
diagnostic output goes to stderr instead.`,
		RunE: runMCP,
	}
	addServeFlags(cmd)
	return cmd
}

// runMCP loads config exactly like runServe, then builds one App shared by
// both transports: the HTTP listener (Serve, backgrounded) and the stdio
// MCP server (ServeStdio, also backgrounded). Both run concurrently and are
// watched via a select, so whichever one fails or ends first — the HTTP
// listener failing to bind while a real MCP client stays connected over
// stdio is the case that matters most — is noticed immediately, not only
// once stdio happens to end. Whichever finishes first cancels the shared
// ctx so the other transport unwinds via graceful shutdown too, and runMCP
// waits for both before returning, so App.Shutdown has always actually run
// by the time this function returns.
func runMCP(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := bindServeFlags(cmd, cfg); err != nil {
		return fmt.Errorf("applying flag overrides: %w", err)
	}

	// stdout is pure JSON-RPC once ServeStdio starts, so this command's own
	// messages go to stderr — never stdout.
	stderrLog := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	a, err := app.New(cfg, app.WithVersion(Version))
	if err != nil {
		return fmt.Errorf("building app: %w", err)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	httpErrCh := make(chan error, 1)
	go func() { httpErrCh <- a.Serve(ctx) }()

	stderrLog.Info("starting debug MCP over stdio", "http_address", cfg.Server.Address())
	mcpServer := mcpAdapter.NewServer(a.Telemetry.Log, a.Telemetry.Memory)
	stdioErrCh := make(chan error, 1)
	go func() { stdioErrCh <- mcpServer.ServeStdio(ctx) }()

	var httpErr, stdioErr error
	var httpDone, stdioDone bool

	// Whichever transport ends first — cleanly or not — is the signal to
	// tear the other one down too, rather than waiting for it to notice on
	// its own (which, for a long-lived stdio session with a connected
	// client, could be never).
	select {
	case httpErr = <-httpErrCh:
		httpDone = true
	case stdioErr = <-stdioErrCh:
		stdioDone = true
	}
	cancel()

	if !httpDone {
		httpErr = <-httpErrCh
	}
	if !stdioDone {
		stdioErr = <-stdioErrCh
	}

	// A real HTTP-listener failure (e.g. the port was already in use) is
	// the more actionable error and takes priority over stdioErr, which in
	// this branch is just the induced context-cancellation unwind.
	if httpErr != nil {
		stderrLog.Error("http listener stopped with error", "error", httpErr)
		return httpErr
	}
	return stdioErr
}
