package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jobrunner/hostus/internal/adapters/telemetry"
)

// Default limits applied when a tool's caller omits (or zeroes) the
// corresponding argument.
const (
	defaultLogLimit   = 50
	defaultErrorLimit = 20
	defaultSpanLimit  = 50
)

// registerTools mounts the four read-only diagnostic tools on srv.
func registerTools(srv *sdkmcp.Server, log *telemetry.RingLog, spans *telemetry.MemoryExporter) {
	addGetRecentLogs(srv, log)
	addTailErrors(srv, log)
	addGetTrace(srv, spans)
	addListSpans(srv, spans)
}

// ---- get_recent_logs -------------------------------------------------------

type getRecentLogsIn struct {
	Level string `json:"level,omitempty" jsonschema:"minimum log level to include: debug, info, warn, or error (default: debug, i.e. everything buffered)"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of records to return, newest first (default 50)"`
}

type logsOut struct {
	Records []telemetry.LogRecord `json:"records"`
	Count   int                   `json:"count"`
}

func addGetRecentLogs(srv *sdkmcp.Server, log *telemetry.RingLog) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "get_recent_logs",
		Description: "Return the most recent buffered hostus log records, newest first. " +
			"Optionally filter by minimum level (debug, info, warn, error).",
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, in getRecentLogsIn) (*sdkmcp.CallToolResult, logsOut, error) {
		min, err := parseLevelOrDefault(in.Level, slog.LevelDebug)
		if err != nil {
			return nil, logsOut{}, err
		}
		records := log.Records(min, resolveLimit(in.Limit, defaultLogLimit))
		return nil, logsOut{Records: records, Count: len(records)}, nil
	})
}

// ---- tail_errors ------------------------------------------------------------

type tailErrorsIn struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of records to return, newest first (default 20)"`
}

func addTailErrors(srv *sdkmcp.Server, log *telemetry.RingLog) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "tail_errors",
		Description: "Return the most recent buffered warn/error hostus log records, newest first.",
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, in tailErrorsIn) (*sdkmcp.CallToolResult, logsOut, error) {
		records := log.Records(slog.LevelWarn, resolveLimit(in.Limit, defaultErrorLimit))
		return nil, logsOut{Records: records, Count: len(records)}, nil
	})
}

// ---- get_trace ---------------------------------------------------------------

type getTraceIn struct {
	TraceID string `json:"trace_id" jsonschema:"hex-encoded trace id to fetch every buffered span for"`
}

type spansOut struct {
	Spans []telemetry.SpanRecord `json:"spans"`
	Count int                    `json:"count"`
}

func addGetTrace(srv *sdkmcp.Server, spans *telemetry.MemoryExporter) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "get_trace",
		Description: "Return every buffered span belonging to the given trace_id, oldest first. " +
			"Returns an empty list (not an error) if the trace id is unknown or was evicted.",
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, in getTraceIn) (*sdkmcp.CallToolResult, spansOut, error) {
		if in.TraceID == "" {
			return nil, spansOut{}, fmt.Errorf("trace_id is required")
		}
		recs := spans.Trace(in.TraceID)
		return nil, spansOut{Spans: recs, Count: len(recs)}, nil
	})
}

// ---- list_spans ---------------------------------------------------------------

type listSpansIn struct {
	Operation     string  `json:"operation,omitempty" jsonschema:"only return spans whose name contains this substring"`
	MinDurationMS float64 `json:"min_duration_ms,omitempty" jsonschema:"only return spans whose duration is at least this many milliseconds"`
	Limit         int     `json:"limit,omitempty" jsonschema:"maximum number of spans to return, newest first (default 50)"`
}

func addListSpans(srv *sdkmcp.Server, spans *telemetry.MemoryExporter) {
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name: "list_spans",
		Description: "List buffered spans, newest first, optionally filtered by an operation-name " +
			"substring and/or a minimum duration in milliseconds.",
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, in listSpansIn) (*sdkmcp.CallToolResult, spansOut, error) {
		filtered := filterSpans(spans.Spans(), in.Operation, in.MinDurationMS, resolveLimit(in.Limit, defaultSpanLimit))
		return nil, spansOut{Spans: filtered, Count: len(filtered)}, nil
	})
}

// resolveLimit returns requested when it is a positive count, otherwise def.
// Every tool's optional `limit` argument goes through this so "omitted or
// zero (or negative)" consistently means "use the tool's own default", never
// "unlimited" — RingLog/MemoryExporter treat <=0 as unlimited, which would
// silently defeat the point of a default cap.
func resolveLimit(requested, def int) int {
	if requested > 0 {
		return requested
	}
	return def
}

// filterSpans returns up to limit spans from all (oldest first, as returned
// by MemoryExporter.Spans) that match operation (substring of the span
// name; empty matches everything) and have a duration of at least
// minDurationMS. Results are newest first and capped at limit.
func filterSpans(all []telemetry.SpanRecord, operation string, minDurationMS float64, limit int) []telemetry.SpanRecord {
	filtered := make([]telemetry.SpanRecord, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- { // walk newest first
		s := all[i]
		if operation != "" && !strings.Contains(s.Name, operation) {
			continue
		}
		if s.DurationMS < minDurationMS {
			continue
		}
		filtered = append(filtered, s)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

// parseLevelOrDefault parses s as a slog.Level, returning def when s is
// empty. An unparseable non-empty level is a caller error.
func parseLevelOrDefault(s string, def slog.Level) (slog.Level, error) {
	if s == "" {
		return def, nil
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid level %q: %w", s, err)
	}
	return lvl, nil
}
