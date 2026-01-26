package kratos

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AAL2Middleware is a middleware that ensures the user has completed MFA (AAL2)
type AAL2Middleware struct {
	provider *KratosAuthProvider
}

// NewAAL2Middleware creates a new AAL2 middleware
func NewAAL2Middleware(provider *KratosAuthProvider) *AAL2Middleware {
	return &AAL2Middleware{provider: provider}
}

// RequireAAL2 returns a Gin middleware that enforces AAL2 (MFA required)
func (m *AAL2Middleware) RequireAAL2() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionCookie, err := c.Cookie("ory_kratos_session")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication_required",
			})
			c.Abort()
			return
		}

		cookieString := "ory_kratos_session=" + sessionCookie
		session, _, err := m.provider.publicClient.FrontendAPI.ToSession(context.Background()).
			Cookie(cookieString).
			Execute()

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_session",
			})
			c.Abort()
			return
		}

		// Check AAL level
		aal := "aal1" // Default
		if session.AuthenticatorAssuranceLevel != nil {
			aal = string(*session.AuthenticatorAssuranceLevel)
		}

		if aal != "aal2" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":        "mfa_required",
				"message":      "This action requires MFA verification",
				"require_aal2": true,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
