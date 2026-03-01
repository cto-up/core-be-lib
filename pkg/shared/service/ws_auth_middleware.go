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
		//
		// VerifyToken already handles cookie and X-Session-Token natively, so we only
		// need to promote the query param into the header as a fallback.
		if _, err := c.Cookie("ory_kratos_session"); err != nil {
			if c.GetHeader("X-Session-Token") == "" {
				if token := c.Query("token"); token != "" {
					c.Request.Header.Set("X-Session-Token", token)
				}
			}
		}

		user, err := authProvider.VerifyToken(c)
		if err != nil {
			log.Error().Err(err).Msg("Token verification failed")
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

// NewWSAuthMiddlewareWithQueryParams is an alias for NewWSAuthMiddleware kept for call-site compatibility.
// Deprecated: use NewWSAuthMiddleware directly.
var NewWSAuthMiddlewareWithQueryParams = NewWSAuthMiddleware

// NewWSAuthMiddlewareWithHeaderFallback is an alias for NewWSAuthMiddleware kept for call-site compatibility.
// Deprecated: use NewWSAuthMiddleware directly.
var NewWSAuthMiddlewareWithHeaderFallback = NewWSAuthMiddleware
