package middleware

import (
	"testing"
	"time"
)

func TestLoadShedder_InitialState(t *testing.T) {
	ls := NewLoadShedder(3, time.Second)

	if ls.IsShedding() {
		t.Error("expected not shedding initially")
	}
	if ls.ShouldShed() {
		t.Error("expected ShouldShed to return false initially")
	}
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 consecutive errors, got %d", ls.ConsecutiveErrors())
	}
}

func TestLoadShedder_ActivatesAfterThreshold(t *testing.T) {
	ls := NewLoadShedder(3, time.Second)

	ls.RecordError()
	ls.RecordError()
	if ls.IsShedding() {
		t.Error("expected not shedding after 2 errors")
	}

	ls.RecordError()
	if !ls.IsShedding() {
		t.Error("expected shedding after 3 errors")
	}
	if !ls.ShouldShed() {
		t.Error("expected ShouldShed to return true")
	}
}

func TestLoadShedder_ResetOnSuccess(t *testing.T) {
	ls := NewLoadShedder(3, time.Second)

	ls.RecordError()
	ls.RecordError()
	ls.RecordError()

	if !ls.IsShedding() {
		t.Error("expected shedding after errors")
	}

	ls.RecordSuccess()

	if ls.IsShedding() {
		t.Error("expected not shedding after success")
	}
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 consecutive errors after success, got %d", ls.ConsecutiveErrors())
	}
}

func TestLoadShedder_AllowsProbeAfterBackoff(t *testing.T) {
	ls := NewLoadShedder(3, 50*time.Millisecond)

	ls.RecordError()
	ls.RecordError()
	ls.RecordError()

	if !ls.ShouldShed() {
		t.Error("expected ShouldShed immediately after errors")
	}

	time.Sleep(100 * time.Millisecond)

	if ls.ShouldShed() {
		t.Error("expected ShouldShed to return false after backoff (allowing probe)")
	}
}

func TestLoadShedder_PartialErrorsDoNotActivate(t *testing.T) {
	ls := NewLoadShedder(5, time.Second)

	ls.RecordError()
	ls.RecordError()
	ls.RecordSuccess() // Reset
	ls.RecordError()
	ls.RecordError()

	if ls.IsShedding() {
		t.Error("expected not shedding with intermittent successes")
	}
	if ls.ConsecutiveErrors() != 2 {
		t.Errorf("expected 2 consecutive errors, got %d", ls.ConsecutiveErrors())
	}
}
