package domain

import (
	"fmt"
	"strings"
)

// Redistribution classifies whether a backbone or trait-vocabulary source
// may be copied into an exported bundle or served publicly (spec: the
// "redistribution gate", 2026-08-01 reality-check milestone). Local ingest
// and private use are never gated by this value — only ExportBundle is.
type Redistribution string

const (
	// RedistributionAllowed marks a source with a clear license permitting
	// redistribution (e.g. WCVP, EIVE, Tichý, Midolo, IPNI, WFO, COL-XR).
	RedistributionAllowed Redistribution = "allowed"
	// RedistributionRestricted marks a source whose license is known to
	// forbid or condition redistribution.
	RedistributionRestricted Redistribution = "restricted"
	// RedistributionUnknown marks a source with no findable license: usable
	// locally/privately (research-use privileges may apply), but its
	// redistribution status has not been cleared.
	RedistributionUnknown Redistribution = "unknown"
)

// ParseRedistribution maps a redistribution spelling (case-insensitive,
// leading/trailing whitespace ignored) to a Redistribution. Unknown or empty
// input returns an error — there is no silent default, since defaulting a
// missing/misspelled value to "allowed" is exactly the failure this type
// exists to prevent.
func ParseRedistribution(s string) (Redistribution, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(RedistributionAllowed):
		return RedistributionAllowed, nil
	case string(RedistributionRestricted):
		return RedistributionRestricted, nil
	case string(RedistributionUnknown):
		return RedistributionUnknown, nil
	default:
		return "", fmt.Errorf("domain: unknown redistribution value %q", s)
	}
}
