package service

import (
	"github.com/gin-gonic/gin"
)

// WSAuthMiddleware is middleware for WebSocket Authentication
// Can work with any AuthProvider implementation
type WSAuthMiddleware struct {
	authProvider AuthProvider
}

// NewWSAuthMiddleware creates a new WebSocket auth middleware with any auth provider
func NewWSAuthMiddleware(authProvider AuthProvider) *WSAuthMiddleware {
	return &WSAuthMiddleware{
		authProvider: authProvider,
	}
}

// MiddlewareFunc verifies token using the configured auth provider
func (wam *WSAuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := wam.authProvider.VerifyToken(c)
		if err != nil {
			c.JSON(401, gin.H{
				"status":  401,
				"message": "Authentication required",
			})
			c.Abort()
			return
		}

		// Store authenticated user info in context
		c.Set(AUTH_EMAIL, user.Email)
		c.Set(AUTH_USER_ID, user.UserID)
		c.Set(AUTH_CLAIMS, user.Claims)
		c.Next()
	}
}
