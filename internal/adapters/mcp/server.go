// Package mcp exposes hostus' in-memory logs and spans (telemetry.RingLog,
// telemetry.MemoryExporter — see S7) as a read-only stdio Model Context
// Protocol server, so Claude Code can inspect a running hostus instance
// while debugging. All four tools only read the injected buffers; nothing
// they do can mutate hostus' state.
package mcp

import (
	"context"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jobrunner/hostus/internal/adapters/telemetry"
)

// serverName/serverVersion identify this MCP server to connecting clients
// (Claude Code's MCP handshake surfaces these).
const (
	serverName    = "hostus-debug"
	serverVersion = "dev"
)

// Server wraps the SDK's *mcp.Server with the four read-only diagnostic
// tools already registered against a specific RingLog/MemoryExporter pair.
type Server struct {
	mcpServer *sdkmcp.Server
}

// NewServer builds a Server that serves get_recent_logs, tail_errors,
// get_trace, and list_spans over log and spans. Both buffers are read-only
// from the tools' perspective — nothing here writes back into them.
func NewServer(log *telemetry.RingLog, spans *telemetry.MemoryExporter) *Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)
	registerTools(srv, log, spans)
	return &Server{mcpServer: srv}
}

// ServeStdio runs the MCP server over stdin/stdout until ctx is canceled or
// the stdio transport hits EOF (e.g. the connecting client disconnected).
// This is the production entry point used by `hostus mcp`.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &sdkmcp.StdioTransport{})
}

// CallTool is a thin, testable seam over the SDK's real JSON-RPC dispatch:
// it connects an in-process client/server session pair over an in-memory
// transport, invokes the named tool exactly as a real MCP client would, and
// flattens the response's text content into a single string. A tool-level
// error (CallToolResult.IsError) is surfaced as a Go error.
func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()

	serverSession, err := s.mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		return "", fmt.Errorf("connecting mcp server session: %w", err)
	}
	defer func() { _ = serverSession.Close() }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "hostus-mcp-internal-client",
		Version: serverVersion,
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return "", fmt.Errorf("connecting mcp client session: %w", err)
	}
	defer func() { _ = clientSession.Close() }()

	res, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("calling tool %q: %w", name, err)
	}

	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	if res.IsError {
		return "", fmt.Errorf("tool %q returned an error: %s", name, sb.String())
	}
	return sb.String(), nil
}
