package core

import (
	"net/url"
	"strings"
)

const maxReturnPathLen = 2048

// safeReturnPath keeps a caller-supplied post-auth destination only when it
// cannot send the user off the app: an app path (`/x`, never `//x` or `/\x`,
// which browsers read as another host) or an absolute http(s) URL on the base
// domain of origin. Anything else yields "". Mirrors core-fe-lib's
// safeRedirectTarget, which the landing page applies again.
func safeReturnPath(raw, origin string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxReturnPathLen || strings.ContainsAny(raw, "\\\r\n\t") {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		if strings.HasPrefix(raw, "//") {
			return ""
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "" || u.Host != "" {
			return ""
		}
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return ""
	}
	o, err := url.Parse(origin)
	if err != nil || o.Hostname() == "" {
		return ""
	}
	base := baseDomainOf(o.Hostname())
	host := u.Hostname()
	if host != base && !strings.HasSuffix(host, "."+base) {
		return ""
	}
	return u.String()
}

func baseDomainOf(hostname string) string {
	parts := strings.Split(hostname, ".")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return hostname
}
