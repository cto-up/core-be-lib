package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// NewWSAuthMiddlewareWithQueryParams creates WebSocket auth middleware that extracts token from query params
// This is useful for mobile apps and browsers that can't send custom headers in WebSocket connections
func NewWSAuthMiddlewareWithQueryParams(authProvider AuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from query parameter
		token := c.Query("token")
		if token == "" {
			log.Error().Msg("No token provided in query parameter")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Token required"})
			c.Abort()
			return
		}

		// Extract subdomain for tenant resolution
		subdomain := c.Query("subdomain")
		if subdomain == "" {
			log.Error().Msg("No subdomain provided in query parameter")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Subdomain required"})
			c.Abort()
			return
		}

		// Set the token in the Authorization header so the provider can verify it
		c.Request.Header.Set("Authorization", "Bearer "+token)

		// Set subdomain in context for tenant resolution
		c.Set("subdomain", subdomain)

		// Verify token using the auth provider
		user, err := authProvider.VerifyToken(c)
		if err != nil {
			log.Error().Err(err).Msg("Token verification failed")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid token"})
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

// NewWSAuthMiddlewareWithHeaderFallback creates WebSocket auth middleware that tries header first, then query params
// This provides flexibility for different client types
func NewWSAuthMiddlewareWithHeaderFallback(authProvider AuthProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get token from Authorization header first
		authHeader := c.GetHeader("Authorization")
		token := ""

		if authHeader != "" {
			// Token in header (standard approach)
			token = authHeader
		} else {
			// Fallback to query parameter for WebSocket connections
			queryToken := c.Query("token")
			if queryToken != "" {
				token = "Bearer " + queryToken
				c.Request.Header.Set("Authorization", token)
			}
		}

		if token == "" {
			log.Error().Msg("No token provided")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Token required"})
			c.Abort()
			return
		}

		// Handle subdomain from query if not in context
		subdomain := c.Query("subdomain")
		if subdomain != "" {
			c.Set("subdomain", subdomain)
		}

		// Verify token using the auth provider
		user, err := authProvider.VerifyToken(c)
		if err != nil {
			log.Error().Err(err).Msg("Token verification failed")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid token"})
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
