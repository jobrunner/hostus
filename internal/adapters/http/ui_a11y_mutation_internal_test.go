package httpx

import (
	"strings"
	"testing"
)

// Mutation testing for the accessibility properties.
//
// `make mutation` (gremlins) cannot reach these: it mutates the Go AST, while
// every property asserted in ui_a11y_internal_test.go lives in style.css,
// index.html or app.js, which reach the binary through go:embed. The mutation
// gate reported "Not covered = 0" for this package while generating no mutant
// at all for the barriers the audit removed — the checks were graded by a
// harness that never tested them.
//
// So this file supplies the missing half: it re-applies, as code, the
// regressions that were reverted by hand during the audit, and demands that
// the check responsible for each one complains. A check that survives its own
// regression is decoration.
//
// The honest limit: these are the mutants we thought of, not a systematic walk
// of the space the way gremlins walks Go operators. It guards the barriers we
// have actually seen — no more, and it should be extended whenever a new one
// is found.

// a11yMutant is one regression: a change to the assets, plus the check that is
// supposed to notice.
type a11yMutant struct {
	name string
	// caughtBy names the check in a11yChecks that must report a finding.
	caughtBy string
	apply    func(a uiAssets) uiAssets
}

func replaceHTML(old, new string) func(uiAssets) uiAssets {
	return func(a uiAssets) uiAssets { a.html = strings.Replace(a.html, old, new, 1); return a }
}

func replaceCSS(old, new string) func(uiAssets) uiAssets {
	return func(a uiAssets) uiAssets { a.css = strings.Replace(a.css, old, new, 1); return a }
}

func replaceJS(old, new string) func(uiAssets) uiAssets {
	return func(a uiAssets) uiAssets { a.js = strings.Replace(a.js, old, new, 1); return a }
}

var a11yMutants = []a11yMutant{
	// WCAG 2.1.1 — the scrollable-region barrier and the over-correction.
	{
		name:     "overflow test removed, so every table becomes a tab stop",
		caughtBy: "scroll-focus-follows-overflow",
		apply:    replaceJS("d.scrollWidth > d.clientWidth", "true"),
	},
	{
		name:     "tab stop is never revoked when a table stops overflowing",
		caughtBy: "scroll-focus-follows-overflow",
		apply:    replaceJS(`d.removeAttribute("tabindex")`, `void 0`),
	},
	{
		name:     "focusability decided once instead of on every layout change",
		caughtBy: "scroll-focus-follows-overflow",
		apply:    replaceJS("new ResizeObserver", "null && ResizeObserver"),
	},
	{
		name:     "hard-coded tabindex back in the markup",
		caughtBy: "scroll-focus-follows-overflow",
		apply:    replaceHTML(`<div class="scroll">`, `<div class="scroll" tabindex="0">`),
	},

	// WCAG 4.1.3 — status messages, both paths.
	{
		name:     "errors render without a live role",
		caughtBy: "errors-announce",
		apply:    replaceJS(`box.setAttribute("role", "alert");`, ""),
	},
	{
		name:     "summary made a live region again, nesting alert inside status",
		caughtBy: "single-concise-live-region",
		apply:    replaceHTML(`id="suggest-summary"`, `id="suggest-summary" role="status"`),
	},
	{
		name:     "dedicated status region removed",
		caughtBy: "single-concise-live-region",
		apply:    replaceHTML(`id="a11y-status" class="sr-only" role="status"`, `id="a11y-status"`),
	},
	{
		name:     "status region hidden from assistive technology too",
		caughtBy: "single-concise-live-region",
		apply:    replaceCSS("position: absolute;", "display: none;"),
	},
	{
		name:     "nothing writes to the status region any more",
		caughtBy: "single-concise-live-region",
		apply:    replaceJS("function announce(", "function announceDisabled("),
	},

	// WCAG 1.4.11 and 2.5.8 — the two measured control fixes.
	{
		name:     "controls fall back to the decorative hairline border",
		caughtBy: "control-contrast-and-target-size",
		apply:    replaceCSS("border: 1px solid var(--control-line);", "border: 1px solid var(--line);"),
	},
	{
		name:     "control border token quietly lightened below 3:1",
		caughtBy: "control-contrast-and-target-size",
		apply:    replaceCSS("--control-line: #7e8896;", "--control-line: #ccd2da;"),
	},
	{
		// Review caught this as a live vacuous pass: an unparseable color was
		// read as black, which scores 21:1 against white and sailed through.
		name:     "control border token becomes an unparseable color",
		caughtBy: "control-contrast-and-target-size",
		apply:    replaceCSS("--control-line: #7e8896;", "--control-line: #zzzzzz;"),
	},
	{
		// Review found this as a live regression: the space pickers were added
		// as <select> and the shared control rule did not cover them.
		name:     "select dropped from the shared control rule",
		caughtBy: "control-contrast-and-target-size",
		apply:    replaceCSS("input, textarea, select, button {", "input, textarea, button {"),
	},
	{
		name:     "select shrunk under the minimum target size",
		caughtBy: "control-contrast-and-target-size",
		apply:    replaceCSS("select { min-height: 1.5rem; }", "select { min-height: 19px; }"),
	},
	{
		name:     "checkbox shrunk back under the minimum target size",
		caughtBy: "control-contrast-and-target-size",
		apply:    replaceCSS(`input[type="checkbox"] { width: 1.5rem; height: 1.5rem;`, `input[type="checkbox"] { width: 13px; height: 13px;`),
	},

	// WCAG 2.4.7 — the focus indicator.
	{
		name:     "focus rule left in place but drawing nothing",
		caughtBy: "visible-focus-indicator",
		apply:    replaceCSS("outline: 2px solid var(--fg);", "outline-offset: 0;"),
	},
	{
		name:     "focus rule reduced to a mention in a comment",
		caughtBy: "visible-focus-indicator",
		apply:    replaceCSS(":focus-visible {", "/* :focus-visible */ .never-matches {"),
	},
}

// TestUIAccessibilityChecksCatchTheirRegression is the gate: for every known
// regression, the responsible check must complain.
func TestUIAccessibilityChecksCatchTheirRegression(t *testing.T) {
	byName := make(map[string]a11yCheck, len(a11yChecks))
	for _, c := range a11yChecks {
		byName[c.name] = c
	}

	for _, m := range a11yMutants {
		t.Run(m.name, func(t *testing.T) {
			check, ok := byName[m.caughtBy]
			if !ok {
				t.Fatalf("mutant names check %q, which does not exist", m.caughtBy)
			}

			original := embeddedAssets()
			mutated := m.apply(original)
			if mutated == original {
				t.Fatalf("the mutation changed nothing — its search text no longer occurs in the assets, so this mutant silently stopped testing anything")
			}

			if findings := check.run(mutated); len(findings) == 0 {
				t.Errorf("check %q accepts this regression; it would not stop the barrier from coming back", m.caughtBy)
			}
		})
	}
}

// TestUIAccessibilityChecksAreAllExercised keeps the two tables honest: a check
// nobody mutates is a check nobody has shown can fail.
func TestUIAccessibilityChecksAreAllExercised(t *testing.T) {
	mutated := make(map[string]int, len(a11yChecks))
	for _, m := range a11yMutants {
		mutated[m.caughtBy]++
	}
	for _, c := range a11yChecks {
		if mutated[c.name] == 0 {
			t.Errorf("no mutant exercises check %q, so nothing proves it can fail", c.name)
		}
	}
}
