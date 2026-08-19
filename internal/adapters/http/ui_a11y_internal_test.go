package httpx

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// The console was audited against WCAG 2.2 AA (static grep gate, axe-core in a
// real DOM, plus the manual keyboard/zoom/reduced-motion passes). What follows
// pins the properties that audit produced.
//
// Each property is a FUNCTION returning its findings rather than a test body
// calling t.Error directly, for one reason: it lets a second driver
// (TestUIAccessibilityChecksCatchTheirRegression) feed the same function
// deliberately broken assets and assert it complains. An accessibility
// property that nothing asserts is one refactor from vanishing — and a check
// that cannot fail is no better than none.

// uiAssets is one snapshot of the three inlined assets, so a mutant can be
// applied to a copy without touching the embedded originals.
type uiAssets struct {
	html string
	css  string
	js   string
}

func embeddedAssets() uiAssets {
	return uiAssets{html: uiIndexHTML, css: uiStyleCSS, js: uiAppJS}
}

// a11yCheck is one WCAG property. run reports every way a violates it; an
// empty result means the property holds.
type a11yCheck struct {
	name string
	run  func(a uiAssets) []string
}

var a11yChecks = []a11yCheck{
	{"scroll-focus-follows-overflow", checkScrollFocus},
	{"errors-announce", checkErrorsAnnounce},
	{"single-concise-live-region", checkLiveRegion},
	{"control-contrast-and-target-size", checkControls},
	{"visible-focus-indicator", checkFocusIndicator},
}

// TestUIAccessibilityProperties is the gate proper: every property must hold
// for the assets actually shipped.
func TestUIAccessibilityProperties(t *testing.T) {
	a := embeddedAssets()
	for _, c := range a11yChecks {
		t.Run(c.name, func(t *testing.T) {
			for _, finding := range c.run(a) {
				t.Error(finding)
			}
		})
	}
}

// checkScrollFocus pins WCAG 2.1.1 and the correction review forced on it. A
// container that scrolls horizontally is unreachable without a mouse unless it
// is a tab stop — axe rates that serious. But making every wrapper a tab stop
// was measurably wrong too: on a wide screen most tables fit (measured
// 486/486) and each became a silent, nameless stop in the tab order, which axe
// cannot see because its rule only flags a MISSING tabindex on an overflowing
// region. Focusability therefore has to track real overflow, re-decided as
// viewport and content change.
func checkScrollFocus(a uiAssets) []string {
	var out []string
	if strings.Contains(a.html, `class="scroll" tabindex`) {
		out = append(out, `a scroll container carries a hard-coded tabindex; focusability must follow measured overflow`)
	}
	for _, want := range []string{
		"d.scrollWidth > d.clientWidth", // the overflow test itself
		`d.removeAttribute("tabindex")`, // and it must be able to REVOKE the stop
		"new ResizeObserver",            // re-decided when layout changes
	} {
		if !strings.Contains(a.js, want) {
			out = append(out, fmt.Sprintf("app.js lost %q; scroll focusability would stop tracking real overflow", want))
		}
	}
	return out
}

// checkErrorsAnnounce pins WCAG 4.1.3 for the failure path. Every panel renders
// errors through errorBox(); without a live role the message appears silently
// and a screen-reader user waits for a result that never comes.
func checkErrorsAnnounce(a uiAssets) []string {
	if !strings.Contains(a.js, `"role", "alert"`) {
		return []string{`errorBox() no longer marks the message as an alert; API errors would appear silently`}
	}
	return nil
}

// checkLiveRegion pins the second correction review forced. Making
// #suggest-summary itself the live region was wrong twice over: errorBox()
// (role="alert") is appended INTO it, nesting two live roles that screen
// readers handle inconsistently; and the summary holds a three-sentence
// analysis that a type-ahead search re-reads after every pause in typing.
// One short, screen-reader-only region carries a single sentence instead.
func checkLiveRegion(a uiAssets) []string {
	var out []string
	if strings.Contains(a.html, `id="suggest-summary" role="status"`) {
		out = append(out, `#suggest-summary is a live region again; errorBox()'s role="alert" would nest inside it`)
	}
	if !strings.Contains(a.html, `id="a11y-status" class="sr-only" role="status"`) {
		out = append(out, "the dedicated status region is gone; result counts would no longer be announced")
	}
	srOnly, found := cssRule(a.css, ".sr-only {")
	switch {
	case !found:
		out = append(out, "the .sr-only rule is gone; the status region would become visible page furniture")
	case strings.Contains(srOnly, "display: none"), strings.Contains(srOnly, "visibility: hidden"):
		out = append(out, "the status region is hidden from assistive technology too, so nothing would be announced")
	}
	if !strings.Contains(a.js, "function announce(") {
		out = append(out, "announce() is gone; nothing writes to the status region")
	}
	return out
}

// checkControls pins two measured fixes:
//   - WCAG 1.4.11: the border that makes an input recognizable needs >= 3:1.
//     The decorative --line (#ccd2da) is 1.52:1 on white, so controls carry a
//     separate --control-line. The ratio is COMPUTED here, not spelled out as
//     a literal: a token quietly lightened back toward the hairline is exactly
//     the regression worth catching, and a string comparison would miss it.
//   - WCAG 2.2 SC 2.5.8: a default checkbox renders 13px tall, under 24x24.
func checkControls(a uiAssets) []string {
	var out []string

	controls, found := cssRule(a.css, "input, textarea, select, button {")
	if !found {
		return []string{"the shared input/textarea/button rule is gone; control borders cannot be checked"}
	}
	if !strings.Contains(controls, "var(--control-line)") {
		out = append(out, "the shared control rule no longer uses --control-line, so borders fall back to the decorative hairline")
	}
	// select was omitted from that rule when the space pickers were added, and
	// this check did not notice: the browser default renders them ~19px tall
	// with their own border, below SC 2.5.8 and outside 1.4.11.
	if selectRule, ok := cssRule(a.css, "select {"); !ok {
		out = append(out, "no select rule; the pickers would render at the browser default height, under the 24x24 minimum")
	} else if px := cssLengthPx(selectRule, "min-height"); px < 24 {
		out = append(out, fmt.Sprintf("select min-height resolves to %.1fpx, want >= 24 (WCAG 2.2 SC 2.5.8)", px))
	}

	border, ok := cssHexToken(a.css, "--control-line")
	if !ok {
		out = append(out, "the --control-line token is gone or is no longer a hex color")
	} else {
		for _, bg := range []string{"--bg", "--panel"} {
			against, ok := cssHexToken(a.css, bg)
			if !ok {
				out = append(out, fmt.Sprintf("the %s token is gone; control border contrast cannot be checked", bg))
				continue
			}
			if r := contrastRatio(border, against); r < 3.0 {
				out = append(out, fmt.Sprintf("control border %s on %s is %.2f:1, want >= 3:1 (WCAG 1.4.11)", border, bg, r))
			}
		}
	}

	checkbox, found := cssRule(a.css, `input[type="checkbox"] {`)
	if !found {
		out = append(out, "the checkbox sizing rule is gone; the default 13px box is below the 24x24 minimum target size")
		return out
	}
	for _, prop := range []string{"width", "height"} {
		if px := cssLengthPx(checkbox, prop); px < 24 {
			out = append(out, fmt.Sprintf("checkbox %s resolves to %.1fpx, want >= 24 (WCAG 2.2 SC 2.5.8)", prop, px))
		}
	}
	return out
}

// checkFocusIndicator pins WCAG 2.4.7. It matters more since the audit: the
// scroll containers are tab stops now, and the ring is the only thing telling
// a sighted keyboard user they are on one.
func checkFocusIndicator(a uiAssets) []string {
	rule, found := cssRule(a.css, ":focus-visible {")
	if !found {
		return []string{"no :focus-visible rule; keyboard users would rely on the browser default, which is weak on the dark buttons"}
	}
	if !strings.Contains(rule, "outline:") {
		return []string{"the :focus-visible rule declares no outline, so it draws no indicator"}
	}
	return nil
}

/* ---------- tiny CSS readers (no parser, just enough to assert values) ---------- */

// cssRule returns the declaration block introduced by opener. Matching the
// opener including its brace is what keeps a mention in a COMMENT from passing
// for a real rule.
func cssRule(css, opener string) (string, bool) {
	_, after, found := strings.Cut(css, opener)
	if !found {
		return "", false
	}
	block, _, _ := strings.Cut(after, "}")
	return block, true
}

// cssLengthPx reads "<prop>: <number><unit>" out of a declaration block and
// returns CSS pixels. rem is relative to the ROOT element, which the console
// never restyles, so 1rem is 16px here. Returns 0 when absent, so a missing
// declaration fails the caller's >= 24 check.
//
// Unit spellings are matched exactly and lowercase: "1.5REM", "calc(...)" or
// an em value all read as 0 and therefore FAIL. That is the intended direction
// — if the stylesheet starts using a form this cannot read, the gate should
// stop and be taught the form, not quietly stop protecting the size.
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

// cssHexToken reads a custom property declared as a #rrggbb literal.
//
// Every digit is validated, not just the length: relativeLuminance parses with
// Sscanf and treats an unparseable pair as 0, so "#zzzzzz" would be read as
// black and score 21:1 against white — a malformed color would PASS the
// contrast check instead of failing it. Rejecting it here is what keeps that
// vacuous pass out.
//
// Deliberately strict in the other direction too: three-digit shorthand
// (#fff), uppercase, or a var() reference all return false, and the caller
// turns that into a finding. For a gate protecting an accessibility property,
// a loud false failure is the right way to be wrong.
func cssHexToken(css, name string) (string, bool) {
	_, after, found := strings.Cut(css, name+":")
	if !found {
		return "", false
	}
	value, _, _ := strings.Cut(after, ";")
	value = strings.TrimSpace(value)
	if len(value) != 7 || value[0] != '#' {
		return "", false
	}
	for _, r := range value[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "", false
		}
	}
	return value, true
}

// contrastRatio is the WCAG 2.x relative-luminance contrast of two #rrggbb
// colors: (lighter + 0.05) / (darker + 0.05).
func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func relativeLuminance(hex string) float64 {
	channel := func(i int) float64 {
		var v int
		if _, err := fmt.Sscanf(hex[i:i+2], "%x", &v); err != nil {
			return 0
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(1) + 0.7152*channel(3) + 0.0722*channel(5)
}
