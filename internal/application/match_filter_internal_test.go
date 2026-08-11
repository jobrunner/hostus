package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// secErrRepo is a Repository whose SecReferenceByID returns a caller-supplied
// error. It embeds the interface (nil) so any OTHER method call panics — the
// point is to exercise validateFilter's non-ErrNotFound error path in
// isolation, where only SecReferenceByID is reached.
type secErrRepo struct {
	output.Repository
	err error
}

func (r secErrRepo) SecReferenceByID(context.Context, string) (domain.SecReference, error) {
	return domain.SecReference{}, r.err
}

// TestValidateFilter_SecLookupErrorIsSurfaced pins that a non-ErrNotFound error
// from SecReferenceByID is returned as-is (not swallowed, not mistaken for
// ErrUnknownSec). Without this, the `if err != nil { return err }` passthrough
// has no test and a negation of it would silently accept a filter the store
// could not validate.
func TestValidateFilter_SecLookupErrorIsSurfaced(t *testing.T) {
	sentinel := errors.New("boom: sec store unavailable")
	err := validateFilter(context.Background(), secErrRepo{err: sentinel}, MatchFilter{Sec: "whatever"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("validateFilter err = %v, want the sentinel store error surfaced as-is", err)
	}
	if errors.Is(err, ErrUnknownSec) {
		t.Error("a store error was mis-reported as ErrUnknownSec")
	}
}
