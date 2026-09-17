package service

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// NewWSAuthMiddleware creates WebSocket auth middleware for Kratos session-based authentication.
//
// Resolution order:
//  1. ory_kratos_session cookie — browsers send this automatically on same-origin WS upgrades
//  2. X-Session-Token header — for native/mobile clients using Kratos API flows
//  3. ?token= query param — for mobile clients that can't set headers on WS connections
//
// subdomain is resolved from the ?subdomain= query param when present.
func NewWSAuthMiddleware(authProvider auth.AuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Resolve subdomain for tenant context
		if subdomain := c.Query("subdomain"); subdomain != "" {
			c.Set("subdomain", subdomain)
		}

		// Priority 1: ory_kratos_session cookie (browser clients — sent automatically)
		// Priority 2: X-Session-Token header (native/mobile Kratos API clients)
		// Priority 3: ?token= query param (mobile clients that can't set WS headers)
		promoteQueryToken(c)

		user, err := authProvider.VerifyToken(c)
		if err != nil {
			log.Err(err).Msg("Token verification failed")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid session"})
			c.Abort()
			return
		}

		c.Set(auth.AUTH_EMAIL, user.Email)
		c.Set(auth.AUTH_USER_ID, user.UserID)
		c.Set(auth.AUTH_CLAIMS, user.Claims)

		c.Next()
	}
}

// promoteQueryToken moves a ?token= query parameter into the X-Session-Token
// header, so that the single extractor in pkg/shared/auth sees it like any
// other credential. A browser cannot set headers on a WebSocket upgrade, which
// is the entire reason this parameter exists.
//
// Promoting it declares the value to be a NATIVE session token, since that is
// now what X-Session-Token means. A client that puts something else there -- a
// session UUID, say -- is simply not authenticated, and should not be: that was
// hub#182 on the HTTP path and hub#189 here.
//
// The cookie wins, and an already-present header is never overwritten: a
// browser on a same-origin upgrade sends its cookie automatically, and it is
// the better credential of the two.
func promoteQueryToken(c *gin.Context) {
	if _, err := c.Cookie("ory_kratos_session"); err == nil {
		return
	}
	if c.GetHeader("X-Session-Token") != "" {
		return
	}
	if token := c.Query("token"); token != "" {
		c.Request.Header.Set("X-Session-Token", token)
	}
}
