package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CredentialKind says what a session credential IS, not merely where it was
// found. The distinction matters because Kratos accepts the two in different
// slots: a browser cookie goes in the Cookie header, a native session token in
// X-Session-Token. Presenting one in the other's slot fails to decode and is
// answered "no active session" -- a 401 indistinguishable from a wrong
// password.
type CredentialKind string

const (
	// CredentialCookie is a browser session cookie value, produced by Kratos'
	// cookie store during a browser flow.
	CredentialCookie CredentialKind = "cookie"

	// CredentialNativeToken is an opaque session token from a Kratos API flow
	// (POST /self-service/login/api), looked up directly in the session table.
	CredentialNativeToken CredentialKind = "native_token"
)

// SessionCredential is a session credential that carries its own provenance.
//
// The kind travels with the value deliberately. A cookie value and a native
// token are both opaque strings, so nothing downstream can tell them apart by
// looking; a heuristic on their shape would fail either as an auth bypass or as
// an outage, and the difference between those two is one Kratos release.
type SessionCredential struct {
	Value string
	Kind  CredentialKind
}

// ExtractSessionCredential is the only place in the estate that knows where a
// session credential can arrive from. It was previously written out three times
// -- here, in the social sign-in handler, and again in tooling-lib's OAuth
// consent gate -- which is why the rule now lives in one exported function.
//
// The provenance is RETURNED rather than stashed on the gin.Context on purpose.
// A context value is invisible at the call site and is silently ignored by any
// caller that never sets it, which is exactly how a half-fixed authentication
// path happens: the half that still works hides the half that does not.
//
// The order -- header, then cookie, then bearer -- is load-bearing and
// unchanged. A browser attaches its Cookie header to every request, so a native
// client's X-Session-Token has to win over it.
func ExtractSessionCredential(c *gin.Context) (SessionCredential, bool) {
	if token := c.GetHeader("X-Session-Token"); token != "" {
		return SessionCredential{Value: token, Kind: CredentialNativeToken}, true
	}

	if cookie, err := c.Cookie("ory_kratos_session"); err == nil && cookie != "" {
		return SessionCredential{Value: cookie, Kind: CredentialCookie}, true
	}

	// A bearer token is native: no browser sends one for a Kratos session, and
	// the clients that do are by definition not browsers. This was previously
	// flattened in with the other two and presented to Kratos as a cookie,
	// which could only ever have worked for a caller pasting a cookie value
	// into an Authorization header.
	if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		if token := strings.TrimPrefix(authHeader, "Bearer "); token != "" {
			return SessionCredential{Value: token, Kind: CredentialNativeToken}, true
		}
	}

	return SessionCredential{}, false
}
