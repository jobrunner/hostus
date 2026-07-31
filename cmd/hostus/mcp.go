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
// MCP server (ServeStdio, foregrounded/blocking). Canceling ctx — via the
// stdio transport reaching EOF, or the process receiving a shutdown signal
// (wired in main.go) — tears both down through the same App.Shutdown path.
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

	a, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("building app: %w", err)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	httpErr := make(chan error, 1)
	go func() { httpErr <- a.Serve(ctx) }()

	stderrLog.Info("starting debug MCP over stdio", "http_address", cfg.Server.Address())
	mcpServer := mcpAdapter.NewServer(a.Telemetry.Log, a.Telemetry.Memory)
	stdioErr := mcpServer.ServeStdio(ctx)

	// The stdio session ending (client disconnect/EOF, or ctx already
	// canceled by a signal) is this command's cue to shut the HTTP listener
	// down too, rather than leaving it running headless.
	cancel()
	if err := <-httpErr; err != nil {
		stderrLog.Error("http listener stopped with error", "error", err)
		if stdioErr == nil {
			return err
		}
	}
	return stdioErr
}
