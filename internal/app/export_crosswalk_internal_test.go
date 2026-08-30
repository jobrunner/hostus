package app

import (
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
)

// TestDetectCrosswalkCollisions_MultipleFallAIDsUnderOneName_EachReported
// pins the fix for a name legitimately holding more than one Fall-A concept
// id (homonyms: an accepted vs. a synonym spelling, or two independently
// ingested spellings under the same string). Before the fix, the
// intermediate byName map kept only the LAST Fall-A id seen for a given
// name, silently dropping any earlier one — so a name with two Fall-A ids
// and one colliding Fall-B id produced only one CrosswalkCollision instead
// of two. detectCrosswalkCollisions's own doc comment promises "every match
// is reported"; this is the case that promise was not yet keeping.
func TestDetectCrosswalkCollisions_MultipleFallAIDsUnderOneName_EachReported(t *testing.T) {
	fallA := []sqlite.CrosswalkEntry{
		{Name: "Festuca ovina agg.", ConceptID: "wcvp:concept:1"},
		{Name: "Festuca ovina agg.", ConceptID: "wcvp:concept:2"},
	}
	fallB := []sqlite.CrosswalkEntry{
		{Name: "Festuca ovina agg.", ConceptID: "eurosl:concept:e-agg1"},
	}

	got := detectCrosswalkCollisions(fallA, fallB)

	if len(got) != 2 {
		t.Fatalf("detectCrosswalkCollisions = %+v, want 2 collisions (one per Fall-A id)", got)
	}
	want := map[string]bool{"wcvp:concept:1": false, "wcvp:concept:2": false}
	for _, c := range got {
		if c.Name != "Festuca ovina agg." {
			t.Errorf("collision.Name = %q, want %q", c.Name, "Festuca ovina agg.")
		}
		if c.FallBConceptID != "eurosl:concept:e-agg1" {
			t.Errorf("collision.FallBConceptID = %q, want %q", c.FallBConceptID, "eurosl:concept:e-agg1")
		}
		if _, ok := want[c.FallAConceptID]; !ok {
			t.Errorf("unexpected FallAConceptID %q", c.FallAConceptID)
			continue
		}
		want[c.FallAConceptID] = true
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("no collision reported for Fall-A id %q", id)
		}
	}
}
