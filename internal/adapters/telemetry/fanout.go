package telemetry

import (
	"context"
	"errors"
	"log/slog"
)

// FanoutHandler is an slog.Handler that dispatches every record to multiple
// underlying handlers, so a single *slog.Logger can feed several sinks
// (e.g. the always-on in-memory RingLog the debug MCP reads from, plus a
// stderr handler so `hostus serve` is not silent on the terminal) without
// either sink knowing about the other.
type FanoutHandler struct {
	handlers []slog.Handler
}

// NewFanoutHandler returns a FanoutHandler dispatching every record to all
// of handlers.
func NewFanoutHandler(handlers ...slog.Handler) *FanoutHandler {
	return &FanoutHandler{handlers: handlers}
}

// Enabled reports whether ANY wrapped handler would handle a record at
// level, so a level only one sink cares about isn't silently dropped for
// all of them.
func (f *FanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches rec to every wrapped handler whose own Enabled accepts
// it, cloning the record for each (slog.Record's Attrs iterator is
// single-use, so the same value can't be replayed across handlers).
// Errors from every handler are joined rather than short-circuiting, so one
// failing sink can't suppress delivery to the others.
func (f *FanoutHandler) Handle(ctx context.Context, rec slog.Record) error {
	var errs []error
	for _, h := range f.handlers {
		if !h.Enabled(ctx, rec.Level) {
			continue
		}
		if err := h.Handle(ctx, rec.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// WithAttrs returns a new FanoutHandler wrapping each underlying handler's
// own WithAttrs result, so attributes propagate to every sink.
func (f *FanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &FanoutHandler{handlers: next}
}

// WithGroup returns a new FanoutHandler wrapping each underlying handler's
// own WithGroup result, so groups propagate to every sink.
func (f *FanoutHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return &FanoutHandler{handlers: next}
}
