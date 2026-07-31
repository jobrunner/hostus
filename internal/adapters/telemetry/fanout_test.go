package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/telemetry"
)

func TestFanoutHandler_HandleWritesToAllHandlers(t *testing.T) {
	var bufA, bufB bytes.Buffer
	ha := slog.NewJSONHandler(&bufA, nil)
	hb := slog.NewJSONHandler(&bufB, nil)
	logger := slog.New(telemetry.NewFanoutHandler(ha, hb))

	logger.Info("hello", "k", "v")

	if !strings.Contains(bufA.String(), `"msg":"hello"`) {
		t.Fatalf("handler A missing record: %s", bufA.String())
	}
	if !strings.Contains(bufB.String(), `"msg":"hello"`) {
		t.Fatalf("handler B missing record: %s", bufB.String())
	}
}

func TestFanoutHandler_EnabledIfAnySubHandlerEnabled(t *testing.T) {
	onlyErrors := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})
	allLevels := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	f := telemetry.NewFanoutHandler(onlyErrors, allLevels)

	if !f.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("want Enabled(Info) true because allLevels accepts it")
	}
}

func TestFanoutHandler_EnabledFalseWhenNoSubHandlerEnabled(t *testing.T) {
	onlyErrors1 := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})
	onlyErrors2 := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})
	f := telemetry.NewFanoutHandler(onlyErrors1, onlyErrors2)

	if f.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("want Enabled(Info) false, no sub-handler accepts it")
	}
}

func TestFanoutHandler_HandleSkipsDisabledSubHandler(t *testing.T) {
	var bufErrOnly, bufAll bytes.Buffer
	errOnly := slog.NewJSONHandler(&bufErrOnly, &slog.HandlerOptions{Level: slog.LevelError})
	all := slog.NewJSONHandler(&bufAll, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(telemetry.NewFanoutHandler(errOnly, all))

	logger.Info("info-level")

	if bufErrOnly.Len() != 0 {
		t.Fatalf("errOnly handler should not receive an Info record, got %s", bufErrOnly.String())
	}
	if !strings.Contains(bufAll.String(), "info-level") {
		t.Fatalf("all handler missing record: %s", bufAll.String())
	}
}

// stubHandler is a minimal slog.Handler double that always reports enabled
// and returns a fixed error from Handle, used to pin FanoutHandler.Handle's
// error-joining behavior without depending on any real handler's error
// paths (which are hard to trigger deterministically).
type stubHandler struct{ err error }

func (h *stubHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (h *stubHandler) Handle(context.Context, slog.Record) error { return h.err }
func (h *stubHandler) WithAttrs([]slog.Attr) slog.Handler        { return h }
func (h *stubHandler) WithGroup(string) slog.Handler             { return h }

func TestFanoutHandler_HandleJoinsErrorsFromAllHandlers(t *testing.T) {
	err1 := errors.New("sink1 failed")
	err2 := errors.New("sink2 failed")
	f := telemetry.NewFanoutHandler(&stubHandler{err: err1}, &stubHandler{err: err2})

	err := f.Handle(context.Background(), slog.Record{})
	if !errors.Is(err, err1) {
		t.Errorf("want joined error to wrap %v, got %v", err1, err)
	}
	if !errors.Is(err, err2) {
		t.Errorf("want joined error to wrap %v, got %v", err2, err)
	}
}

func TestFanoutHandler_HandleReturnsNilWhenAllHandlersSucceed(t *testing.T) {
	f := telemetry.NewFanoutHandler(&stubHandler{err: nil}, &stubHandler{err: nil})

	if err := f.Handle(context.Background(), slog.Record{}); err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
}

func TestFanoutHandler_WithAttrsAppliesToAllHandlers(t *testing.T) {
	var bufA, bufB bytes.Buffer
	ha := slog.NewJSONHandler(&bufA, nil)
	hb := slog.NewJSONHandler(&bufB, nil)
	f := telemetry.NewFanoutHandler(ha, hb)

	logger := slog.New(f.WithAttrs([]slog.Attr{slog.String("component", "serve")}))
	logger.Info("hi")

	for name, buf := range map[string]*bytes.Buffer{"A": &bufA, "B": &bufB} {
		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("handler %s: unmarshal %q: %v", name, buf.String(), err)
		}
		if entry["component"] != "serve" {
			t.Fatalf("handler %s missing component attr: %v", name, entry)
		}
	}
}

func TestFanoutHandler_WithGroupAppliesToAllHandlers(t *testing.T) {
	var bufA, bufB bytes.Buffer
	ha := slog.NewJSONHandler(&bufA, nil)
	hb := slog.NewJSONHandler(&bufB, nil)
	f := telemetry.NewFanoutHandler(ha, hb)

	logger := slog.New(f.WithGroup("req")).With("id", "abc")
	logger.Info("hi")

	for name, buf := range map[string]*bytes.Buffer{"A": &bufA, "B": &bufB} {
		if !strings.Contains(buf.String(), `"req":{"id":"abc"}`) {
			t.Fatalf("handler %s missing group: %s", name, buf.String())
		}
	}
}
