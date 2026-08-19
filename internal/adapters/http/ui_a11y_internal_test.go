package httpx

import (
	"fmt"
	"strings"
	"testing"
)

// The console was audited against WCAG 2.2 AA (static grep gate, axe-core in a
// real DOM, plus the manual keyboard/zoom/reduced-motion passes). These tests
// pin the four fixes that audit produced, because an accessibility property
// that nothing asserts is one refactor away from silently disappearing — and
// unlike a broken feature, nobody on the team would notice.

// TestUIScrollFocusFollowsActualOverflow pins WCAG 2.1.1 AND the correction the
// review forced. A container that scrolls horizontally is unreachable without a
// mouse unless it is a tab stop — axe reported that as serious once results
// rendered. But making every wrapper a tab stop unconditionally was measurably
// wrong too: on a wide screen most tables fit (measured 842/842), and each one
// became a silent, nameless stop in the tab order. Focusability therefore has
// to track real overflow, and overflow changes with viewport and content — so
// it is re-decided by a ResizeObserver, not once at construction.
func TestUIScrollFocusFollowsActualOverflow(t *testing.T) {
	if strings.Contains(uiIndexHTML, `class="scroll" tabindex`) {
		t.Error("a scroll container carries a hard-coded tabindex; focusability must follow measured overflow instead")
	}
	for _, want := range []string{
		"d.scrollWidth > d.clientWidth", // the overflow test itself
		`d.removeAttribute("tabindex")`, // and it must be able to REVOKE the stop
		"new ResizeObserver",            // re-decided when layout changes
	} {
		if !strings.Contains(uiAppJS, want) {
			t.Errorf("app.js no longer contains %q; scroll focusability would stop tracking real overflow", want)
		}
	}
}

// TestUIErrorsAnnounceThemselves pins WCAG 4.1.3 for the failure path. Every
// panel renders its errors through errorBox(); without a live role the message
// appears silently and a screen-reader user is left waiting for a result that
// never comes.
func TestUIErrorsAnnounceThemselves(t *testing.T) {
	if !strings.Contains(uiAppJS, `"role", "alert"`) {
		t.Error("errorBox() no longer marks the message as an alert; API errors would appear silently")
	}
}

// TestUILiveRegionIsSingleAndConcise pins the second correction the review
// forced. The first attempt made #suggest-summary itself the live region, which
// was wrong twice over: errorBox() (role="alert") is appended INTO it, nesting
// two live roles that screen readers handle inconsistently; and the summary
// holds a three-sentence analysis that a type-ahead search would re-read after
// every pause in typing. There is now one short, screen-reader-only region that
// receives a single sentence per operation.
func TestUILiveRegionIsSingleAndConcise(t *testing.T) {
	if strings.Contains(uiIndexHTML, `id="suggest-summary" role="status"`) {
		t.Error(`#suggest-summary is a live region again; errorBox()'s role="alert" would nest inside it`)
	}
	if !strings.Contains(uiIndexHTML, `id="a11y-status" class="sr-only" role="status"`) {
		t.Error("the dedicated status region is gone; result counts would no longer be announced")
	}
	if !strings.Contains(uiStyleCSS, ".sr-only") {
		t.Error("the .sr-only rule is gone; the status region would become visible page furniture")
	}
	if strings.Contains(uiStyleCSS, ".sr-only") && strings.Contains(uiStyleCSS, "display: none") {
		t.Error(".sr-only must stay in the accessibility tree; display:none would silence the announcements")
	}
	if !strings.Contains(uiAppJS, "function announce(") {
		t.Error("announce() is gone; nothing writes to the status region")
	}
}

// TestUIControlsMeetContrastAndTargetSize pins the two measured fixes:
//   - WCAG 1.4.11: the border that makes an input recognizable needs >= 3:1.
//     The decorative --line (#ccd2da) is 1.52:1 on white, so controls use a
//     separate --control-line (#7e8896, 3.59:1). Keeping them separate is the
//     point: hairline table rules may stay faint, control borders may not.
//   - WCAG 2.2 SC 2.5.8: a default checkbox renders 13px tall, under the
//     24x24 CSS-px minimum.
func TestUIControlsMeetContrastAndTargetSize(t *testing.T) {
	if !strings.Contains(uiStyleCSS, "--control-line:") {
		t.Fatal("the --control-line token is gone; control borders would fall back to the decorative hairline")
	}
	_, after, found := strings.Cut(uiStyleCSS, "input, textarea, button {")
	if !found {
		t.Fatal("the shared input/textarea/button rule is gone; this test can no longer tell which border controls use")
	}
	controls, _, _ := strings.Cut(after, "}")
	if !strings.Contains(controls, "var(--control-line)") {
		t.Errorf("input/textarea/button no longer use --control-line:\n%s", controls)
	}

	// Assert the measured SIZE, not merely that a rule exists: a rule that
	// shrank back below 24px would satisfy a presence check while restoring
	// the barrier.
	_, cbAfter, cbFound := strings.Cut(uiStyleCSS, `input[type="checkbox"] {`)
	if !cbFound {
		t.Fatal("the checkbox sizing rule is gone; the default 13px box is below the 24x24 minimum target size")
	}
	cb, _, _ := strings.Cut(cbAfter, "}")
	for _, prop := range []string{"width", "height"} {
		px := cssLengthPx(cb, prop)
		if px < 24 {
			t.Errorf("checkbox %s resolves to %.1fpx, want >= 24 (WCAG 2.2 SC 2.5.8); rule was: %s", prop, px, cb)
		}
	}
}

// cssLengthPx reads "<prop>: <number><unit>" out of a declaration block and
// returns it in CSS pixels. rem is relative to the ROOT element, which the
// console never restyles, so 1rem is 16px here. Returns 0 when absent, so a
// missing declaration fails the caller's >= 24 check.
func cssLengthPx(block, prop string) float64 {
	_, after, found := strings.Cut(block, prop+":")
	if !found {
		return 0
	}
	value, _, _ := strings.Cut(after, ";")
	value = strings.TrimSpace(value)
	for unit, factor := range map[string]float64{"rem": 16, "px": 1} {
		if num, ok := strings.CutSuffix(value, unit); ok {
			var f float64
			if _, err := fmt.Sscanf(strings.TrimSpace(num), "%g", &f); err == nil {
				return f * factor
			}
		}
	}
	return 0
}

// TestUIHasAVisibleFocusIndicator pins WCAG 2.4.7. It matters more since the
// audit: the scroll containers are now tab stops, and a focus ring is the only
// thing telling a sighted keyboard user they are on one.
func TestUIHasAVisibleFocusIndicator(t *testing.T) {
	// A rule, not a mention: ":focus-visible" inside a comment would satisfy a
	// plain substring check while the actual outline is gone.
	_, after, found := strings.Cut(uiStyleCSS, ":focus-visible {")
	if !found {
		t.Fatal("no :focus-visible rule; keyboard users would rely on the browser default, which is weak on the dark buttons")
	}
	rule, _, _ := strings.Cut(after, "}")
	if !strings.Contains(rule, "outline:") {
		t.Errorf(":focus-visible declares no outline, so it draws no indicator; rule was: %s", rule)
	}
}
