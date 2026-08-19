package httpx

import (
	"regexp"
	"strings"
	"testing"
)

// The console was audited against WCAG 2.2 AA (static grep gate, axe-core in a
// real DOM, plus the manual keyboard/zoom/reduced-motion passes). These tests
// pin the four fixes that audit produced, because an accessibility property
// that nothing asserts is one refactor away from silently disappearing — and
// unlike a broken feature, nobody on the team would notice.

// uiScrollDiv matches a horizontally scrollable table wrapper in the markup.
var uiScrollDiv = regexp.MustCompile(`<div class="scroll"[^>]*>`)

// TestUIScrollContainersAreKeyboardFocusable pins WCAG 2.1.1: a container that
// scrolls horizontally can only be reached with a mouse unless it is a tab
// stop. axe reported this as a serious violation ("scrollable-region-focusable")
// once results were rendered. The dynamic wrappers are built by scroller() in
// app.js; the one static wrapper lives in index.html.
func TestUIScrollContainersAreKeyboardFocusable(t *testing.T) {
	for _, m := range uiScrollDiv.FindAllString(uiIndexHTML, -1) {
		if !strings.Contains(m, "tabindex") {
			t.Errorf("scroll container %q is not focusable; a keyboard user cannot scroll it", m)
		}
	}
	if !strings.Contains(uiAppJS, "tabIndex") {
		t.Error("scroller() in app.js no longer sets tabIndex; dynamically built tables would stop being keyboard-scrollable")
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

// TestUISuggestSummaryIsAStatusRegion pins WCAG 4.1.3 for the success path.
// The summary carries the short status text ("Keine Treffer.", the prefix
// line). It is deliberately the live region rather than the result table,
// which would be re-read in full on every keystroke.
func TestUISuggestSummaryIsAStatusRegion(t *testing.T) {
	if !strings.Contains(uiIndexHTML, `id="suggest-summary" role="status"`) {
		t.Error(`#suggest-summary lost role="status"; result counts and "Keine Treffer." would no longer be announced`)
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

	if !strings.Contains(uiStyleCSS, `input[type="checkbox"]`) {
		t.Error("the checkbox sizing rule is gone; the default 13px box is below the 24x24 minimum target size")
	}
}

// TestUIHasAVisibleFocusIndicator pins WCAG 2.4.7. It matters more since the
// audit: the scroll containers are now tab stops, and a focus ring is the only
// thing telling a sighted keyboard user they are on one.
func TestUIHasAVisibleFocusIndicator(t *testing.T) {
	if !strings.Contains(uiStyleCSS, ":focus-visible") {
		t.Error("no :focus-visible rule; keyboard users would rely on the browser default, which is weak on the dark buttons")
	}
}
