package validation

import "testing"

func TestMatchWildcardOriginNestedSubdomain(t *testing.T) {
	tests := []struct {
		origin, pattern string
		want            bool
	}{
		{"https://app.tragge.com", "https://*.tragge.com", true},
		{"https://api.tragge.com", "https://*.tragge.com", true},
		{"https://evil.sub.tragge.com", "https://*.tragge.com", false},  // nested
		{"https://a.b.c.tragge.com", "https://*.tragge.com", false},     // deeply nested
		{"https://tragge.com", "https://*.tragge.com", false},           // exact domain, not subdomain
		{"http://app.tragge.com", "https://*.tragge.com", false},        // wrong scheme
		// Bare wildcard patterns
		{"https://app.example.com", "*.example.com", true},
		{"https://evil.sub.example.com", "*.example.com", false},        // nested via bare wildcard
		{"https://a.b.c.example.com", "*.example.com", false},           // deeply nested via bare wildcard
	}
	for _, tt := range tests {
		got := MatchWildcardOrigin(tt.origin, tt.pattern)
		if got != tt.want {
			t.Errorf("MatchWildcardOrigin(%q, %q) = %v, want %v", tt.origin, tt.pattern, got, tt.want)
		}
	}
}
