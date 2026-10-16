package core

import (
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

const maxReturnPathLen = 2048

// safeReturnPath keeps a caller-supplied post-auth destination only when it
// cannot send the user off the app: an app path (`/x`, never `//x` or `/\x`,
// which browsers read as another host) or an absolute http(s) URL on the
// registrable domain of origin, never downgraded from https to http. Anything
// else yields "". Mirrors core-fe-lib's
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
	if err != nil || o.Hostname() == "" || (o.Scheme == "https" && u.Scheme != "https") {
		return ""
	}
	base := baseDomainOf(o.Hostname())
	host := u.Hostname()
	if host != base && !strings.HasSuffix(host, "."+base) {
		return ""
	}
	return u.String()
}

// baseDomainOf is the registrable domain (eTLD+1) of hostname, read from the
// public-suffix list so that a two-part suffix such as co.uk is never taken
// for a domain. A host with no registrable part (an IP, a bare suffix) is its
// own base.
func baseDomainOf(hostname string) string {
	base, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		return hostname
	}
	return base
}
