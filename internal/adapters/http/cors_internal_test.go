package httpx

import "testing"

func TestMatchOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		pattern string
		want    bool
	}{
		{"exact", "https://a.example", "https://a.example", true},
		{"exact mismatch", "https://b.example", "https://a.example", false},
		{"wildcard subdomain", "https://sub.example.com", "https://*.example.com", true},
		{"wildcard deep subdomain", "https://a.b.example.com", "https://*.example.com", true},
		{"wildcard excludes bare domain", "https://example.com", "https://*.example.com", false},
		{"wildcard excludes lookalike", "https://evilexample.com", "https://*.example.com", false},
		{"wildcard keeps scheme", "http://sub.example.com", "https://*.example.com", false},
		{"wildcard keeps port", "https://sub.example.com:8443", "https://*.example.com", false},
		{"wildcard matches with equal port", "https://sub.example.com:8443", "https://*.example.com:8443", true},
		{"non-wildcard pattern never fuzzy-matches", "https://sub.example.com", "https://example.com", false},
		{"pattern without scheme matches nothing real", "https://sub.example.com", "*.example.com", false},
		{"exact match ignores host case", "https://habitatus.example", "https://Habitatus.Example", true},
		{"exact match ignores scheme case", "https://a.example", "HTTPS://a.example", true},
		{"wildcard ignores host case", "https://x.EXAMPLE.com", "https://*.example.com", true},
		{"case folding does not widen the host", "https://evil.example", "https://A.EXAMPLE", false},
		// The asymmetry is deliberate, see matchOrigin's doc comment: a path
		// on the ORIGIN side is a request-supplied value that would end up
		// echoed verbatim in Access-Control-Allow-Origin, while a path on the
		// PATTERN side is an operator typo worth forgiving.
		{"origin with trailing slash is rejected", "https://ok.example/", "https://ok.example", false},
		{"origin with path is rejected", "https://ok.example/app", "https://ok.example", false},
		{"origin with path is rejected against a wildcard too", "https://sub.example.com/app", "https://*.example.com", false},
		{"pattern with path is forgiven", "https://ok.example", "https://ok.example/app", true},
		{"pattern with trailing slash is forgiven", "https://ok.example", "https://ok.example/", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchOrigin(tt.origin, tt.pattern); got != tt.want {
				t.Fatalf("matchOrigin(%q, %q) = %v, want %v", tt.origin, tt.pattern, got, tt.want)
			}
		})
	}
}

// TestSplitOrigin pins the three-way split directly: matchOrigin only ever
// compares the parts, so a mis-sliced scheme or port could cancel itself out
// there and stay invisible.
func TestSplitOrigin(t *testing.T) {
	tests := []struct {
		name               string
		in                 string
		scheme, host, port string
	}{
		{"scheme and host", "https://a.example", "https", "a.example", ""},
		{"scheme host port", "https://a.example:8443", "https", "a.example", "8443"},
		{"trailing path is dropped", "https://a.example/foo/bar", "https", "a.example", ""},
		{"path after port is dropped", "https://a.example:8443/foo", "https", "a.example", "8443"},
		{"no scheme", "a.example", "", "a.example", ""},
		{"wildcard pattern", "https://*.example.com", "https", "*.example.com", ""},
		// Scheme and host are case-insensitive per RFC 3986 and are folded
		// here so every comparison in matchOrigin inherits that; the port is
		// left alone (folding digits would be pointless) and the path never
		// survives the split at all, so nothing case-sensitive is touched.
		{"scheme and host are folded, port is not", "HTTPS://A.Example:8443", "https", "a.example", "8443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, host, port := splitOrigin(tt.in)
			if scheme != tt.scheme || host != tt.host || port != tt.port {
				t.Fatalf("splitOrigin(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.in, scheme, host, port, tt.scheme, tt.host, tt.port)
			}
		})
	}
}
