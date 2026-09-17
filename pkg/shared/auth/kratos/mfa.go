package kratos

import (
	"context"
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
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
		// A native client carries its session token, not a cookie. The gate
		// stays closed by default either way: no credential is still a 401.
		cred, ok := auth.ExtractSessionCredential(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication_required",
			})
			c.Abort()
			return
		}

		session, err := m.provider.sessionForCredential(context.Background(), cred)

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
