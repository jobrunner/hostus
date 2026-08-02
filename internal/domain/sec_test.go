package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestSecReferenceIsZeroWhenEmpty(t *testing.T) {
	var s domain.SecReference
	if !s.IsZero() {
		t.Error("zero SecReference must report IsZero")
	}
	if (domain.SecReference{ID: "060afae5"}).IsZero() {
		t.Error("SecReference with an ID must not report IsZero")
	}
	// A title without an identity is NOT a usable reference space: the id is
	// what taxon_concept.sec_reference stores and what /translate keys on.
	if !(domain.SecReference{Title: "Wisskirchen & Haeupler 1998"}).IsZero() {
		t.Error("SecReference without an ID must report IsZero")
	}
}
