package app

import (
	"fmt"
	"os"
)

// describeOpenFailure turns sqlite.OpenExisting's refusal into something a
// person reading a terminal can act on.
//
// SQLite answers a missing database with "unable to open database file (14)",
// which is the same message a permission problem gets — and for the case this
// exists for, a typo'd --db, it says nothing about the actual mistake. So the
// path is stat'd here and, when it simply is not there, the error says that
// instead.
//
// The stat runs AFTER the open has already failed, and that ordering is the
// point: it only describes a refusal that has happened, so it cannot
// reintroduce the check-then-open race OpenExisting was written to close (see
// its doc comment). Any other failure is passed through untouched rather than
// guessed at.
func describeOpenFailure(path string, err error) error {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return fmt.Errorf("database does not exist: %w", statErr)
	}
	return err
}
