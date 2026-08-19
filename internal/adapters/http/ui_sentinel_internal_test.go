package httpx

import (
	"strings"
	"testing"
)

// uiSentinels is every substitution marker buildUIDocument replaces.
var uiSentinels = map[string]string{
	"style":   uiStyleSentinel,
	"script":  uiScriptSentinel,
	"version": uiVersionSentinel,
}

// TestUISentinelsAppearExactlyOnceInTheTemplate pins the precondition every
// strings.Replace(..., 1) in buildUIDocument relies on. A second occurrence
// would leave one of them unsubstituted, and the page would render a literal
// "/*hostus:version*/" to the user.
func TestUISentinelsAppearExactlyOnceInTheTemplate(t *testing.T) {
	for name, sentinel := range uiSentinels {
		if n := strings.Count(uiIndexHTML, sentinel); n != 1 {
			t.Errorf("assets/index.html contains the %s sentinel %d times, want exactly 1", name, n)
		}
	}
}

// TestUISentinelsAreAbsentFromTheInlinedAssets closes a real substitution
// trap. The sentinels are literal CSS/JS comments, and buildUIDocument
// substitutes into ONE shared string in a fixed order: style, script, then
// version. If assets/style.css ever contained the text "/*hostus:version*/" —
// entirely plausible, a contributor documenting the mechanism in a comment —
// then by the time the version replace runs, the FIRST occurrence in the
// document sits inside the already-inlined <style> block. The version would be
// injected into the stylesheet (breaking the CSS and desyncing the CSP hash,
// which covers the unmodified asset) while the footer kept showing the raw
// sentinel. Nothing else in the suite would catch it, so this test does.
func TestUISentinelsAreAbsentFromTheInlinedAssets(t *testing.T) {
	assets := map[string]string{
		"assets/style.css": uiStyleCSS,
		"assets/app.js":    uiAppJS,
	}
	for assetName, content := range assets {
		for sentinelName, sentinel := range uiSentinels {
			if strings.Contains(content, sentinel) {
				t.Errorf("%s contains the %s sentinel %q; buildUIDocument substitutes into one shared string, so this would misplace a substitution — rename the comment",
					assetName, sentinelName, sentinel)
			}
		}
	}
}

// TestBuildUIDocumentLeavesNoSentinelBehind is the end-to-end guarantee the two
// tests above protect: whatever the assets contain, a served document must
// carry no unsubstituted marker.
func TestBuildUIDocumentLeavesNoSentinelBehind(t *testing.T) {
	doc := buildUIDocument("v1.2.3")
	for name, sentinel := range uiSentinels {
		if strings.Contains(doc, sentinel) {
			t.Errorf("composed document still contains the %s sentinel %q", name, sentinel)
		}
	}
}
