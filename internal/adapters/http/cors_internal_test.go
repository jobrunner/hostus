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
