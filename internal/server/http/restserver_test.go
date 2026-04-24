package http

import "testing"

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		domain string
		want   bool
	}{
		// empty inputs — fail closed
		{"empty domain rejects everything", "https://ctoup.com", "", false},
		{"empty origin", "", "ctoup.com", false},

		// apex host match
		{"apex https", "https://ctoup.com", "ctoup.com", true},
		{"apex http", "http://ctoup.com", "ctoup.com", true},
		{"apex with port", "https://ctoup.com:8443", "ctoup.com", true},
		{"apex uppercase normalized", "HTTPS://CTOUP.COM", "ctoup.com", true},

		// subdomain match
		{"single-level subdomain", "https://app.ctoup.com", "ctoup.com", true},
		{"nested subdomain", "https://a.b.ctoup.com", "ctoup.com", true},
		{"subdomain with port", "https://app.ctoup.com:3000", "ctoup.com", true},
		{"subdomain uppercase normalized", "https://APP.CTOUP.COM", "ctoup.com", true},

		// localhost (dev case)
		{"localhost apex", "http://localhost", "localhost", true},
		{"localhost with port", "http://localhost:5173", "localhost", true},
		{"localhost subdomain", "http://tenant-a.localhost", "localhost", true},

		// unrelated domains
		{"unrelated domain", "https://evil.com", "ctoup.com", false},
		{"different tld", "https://ctoup.org", "ctoup.com", false},

		// suffix-confusion attacks — must all be blocked
		{"apex-name inside attacker domain", "https://ctoup.com.evil.com", "ctoup.com", false},
		{"no dot boundary before apex", "https://foomctoup.com", "ctoup.com", false},
		{"apex name as path, not host", "https://evil.com/ctoup.com", "ctoup.com", false},
		{"apex name as query, not host", "https://evil.com?x=ctoup.com", "ctoup.com", false},

		// malformed origins
		{"bare string (no scheme)", "ctoup.com", "ctoup.com", false},
		{"not a url", "not-a-url", "ctoup.com", false},
		{"scheme only", "https://", "ctoup.com", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOriginAllowed(tc.origin, tc.domain); got != tc.want {
				t.Errorf("isOriginAllowed(%q, %q) = %v, want %v", tc.origin, tc.domain, got, tc.want)
			}
		})
	}
}
