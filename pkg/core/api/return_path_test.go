package core

import "testing"

func TestSafeReturnPath(t *testing.T) {
	const origin = "http://corpa.ctoup.localhost:5173"
	cases := []struct {
		name, raw, origin, want string
	}{
		{"empty", "", origin, ""},
		{"app path", "/lms/courses/intro", origin, "/lms/courses/intro"},
		{"app path with query", "/lms/courses/intro?tab=2#x", origin, "/lms/courses/intro?tab=2#x"},
		{"trimmed", "  /lms  ", origin, "/lms"},
		{"protocol-relative", "//evil.example/x", origin, ""},
		{"backslash host", "/\\evil.example", origin, ""},
		{"foreign absolute", "https://evil.example/phish", origin, ""},
		{"look-alike suffix", "https://ctoup.localhost.evil.example/", origin, ""},
		{"suffix without dot", "https://evilctoup.localhost/", origin, ""},
		{"javascript scheme", "javascript:alert(1)", origin, ""},
		{"data scheme", "data:text/html,x", origin, ""},
		{"relative without slash", "lms/courses", origin, ""},
		{"userinfo", "http://corpa.ctoup.localhost@evil.example/", origin, ""},
		{"newline", "/lms\r\nSet-Cookie: x", origin, ""},
		{"same host absolute", "http://corpa.ctoup.localhost:5173/lms", origin, "http://corpa.ctoup.localhost:5173/lms"},
		{"sibling subdomain", "https://corpb.ctoup.localhost/lms", origin, "https://corpb.ctoup.localhost/lms"},
		{"bare base domain", "https://sparkmeee.com/x", "https://acme.sparkmeee.com", "https://sparkmeee.com/x"},
		{"absolute with no origin", "https://sparkmeee.com/x", "", ""},
		{"too long", "/" + string(make([]byte, maxReturnPathLen)), origin, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeReturnPath(tc.raw, tc.origin); got != tc.want {
				t.Errorf("safeReturnPath(%q, %q) = %q, want %q", tc.raw, tc.origin, got, tc.want)
			}
		})
	}
}
