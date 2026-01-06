package service

import (
	"net/http"
	"strings"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const (
	// Header key for API tokens
	XApiKeyHeader = "X-Api-Key"
)

// AuthMiddleware combines both API token and provider-based authentication
type AuthMiddleware struct {
	authProvider auth.AuthProvider
	apiToken     *ClientApplicationService
}

// NewAuthMiddleware creates a new combined authentication middleware
func NewAuthMiddleware(
	authProvider auth.AuthProvider,
	apiToken *ClientApplicationService,
) *AuthMiddleware {
	return &AuthMiddleware{
		authProvider: authProvider,
		apiToken:     apiToken,
	}
}

// MiddlewareFunc implements OR authentication logic
func (am *AuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for public endpoints
		if strings.HasPrefix(c.Request.URL.Path, "/public") {
			c.Next()
			return
		}

		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/users/by-email") ||
			(!strings.HasPrefix(c.Request.URL.Path, "/api/v1/users") &&
				!strings.HasPrefix(c.Request.URL.Path, "/admin-api") &&
				!strings.HasPrefix(c.Request.URL.Path, "/superadmin-api")) {
			// Check X-Api-Key header first
			token := c.GetHeader(XApiKeyHeader)

			// If X-Api-Key is not present, try legacy token extraction
			if token != "" {
				tokenRow, err := am.apiToken.VerifyAPIToken(c, token)
				if err == nil {
					// API token is valid, store info and continue
					c.Set("api_token", tokenRow)
					c.Set("api_token_scopes", tokenRow.Scopes)
					// c.Set(auth.AUTH_EMAIL,)
					// c.Set(auth.AUTH_CLAIMS, idToken.Claims)
					c.Set(auth.AUTH_USER_ID, tokenRow.CreatedBy)
					c.Next()
					return
				} else {
					// API token is invalid
					c.JSON(http.StatusForbidden, gin.H{
						"status":  http.StatusForbidden,
						"message": "Invalid API token",
					})
					c.Abort()
					return
				}
			} else {
				// Try provider-based authentication
				user, err := am.authProvider.VerifyToken(c)
				if err != nil {
					log.Error().Err(err).Str("provider", am.authProvider.GetProviderName()).Msg("authentication failed")
					c.JSON(http.StatusUnauthorized, gin.H{
						"status":  http.StatusUnauthorized,
						"message": "Authentication required",
					})
					c.Abort()
					return
				}

				// Store authenticated user info in context
				am.setAuthenticatedUser(c, user)
			}
		} else {
			// Use provider-based authentication
			user, err := am.authProvider.VerifyToken(c)
			if err != nil {
				log.Error().Err(err).Str("provider", am.authProvider.GetProviderName()).Msg("authentication failed")
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  http.StatusUnauthorized,
					"message": http.StatusText(http.StatusUnauthorized),
				})
				c.Abort()
				return
			}

			// Store authenticated user info in context
			am.setAuthenticatedUser(c, user)

			// Check role-based permissions
			if !am.checkPermissions(c, user) {
				return
			}

			c.Next()
		}
	}
}

// setAuthenticatedUser stores user info in gin context
func (am *AuthMiddleware) setAuthenticatedUser(c *gin.Context, user *auth.AuthenticatedUser) {
	c.Set(auth.AUTH_EMAIL, user.Email)
	c.Set(auth.AUTH_USER_ID, user.UserID)
	c.Set(auth.AUTH_CLAIMS, user.Claims)

	// Set custom claims for easy role checking
	if len(user.CustomClaims) > 0 {
		c.Set("custom_claims", user.CustomClaims)
	}

	// Set tenant context if available
	if user.TenantID != "" {
		c.Set(auth.AUTH_TENANT_ID_KEY, user.TenantID)
	}

	// Set tenant memberships for efficient middleware validation
	if len(user.TenantMemberships) > 0 {
		c.Set(auth.AUTH_TENANT_MEMBERSHIPS, user.TenantMemberships)
	}
}

// checkPermissions validates role-based access control
func (am *AuthMiddleware) checkPermissions(c *gin.Context, user *auth.AuthenticatedUser) bool {
	claims := user.Claims

	// Only admin users can alter users
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/users") &&
		util.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, c.Request.Method) {

		if claims["SUPER_ADMIN"] == true || claims["ADMIN"] == true || claims["CUSTOMER_ADMIN"] == true {
			return true
		}
		c.JSON(http.StatusForbidden, gin.H{
			"status":  http.StatusForbidden,
			"message": "Need to be an ADMIN to perform such operation",
		})
		c.Abort()
		return false
	}

	// Admin API access
	if strings.HasPrefix(c.Request.URL.Path, "/admin-api") {
		if claims["SUPER_ADMIN"] == true || claims["ADMIN"] == true {
			return true
		}
		c.JSON(http.StatusForbidden, gin.H{
			"status":  http.StatusForbidden,
			"message": "Need to be an ADMIN or SUPER_ADMIN to perform such operation",
		})
		c.Abort()
		return false
	}

	// Super admin API access
	if strings.HasPrefix(c.Request.URL.Path, "/superadmin-api") {
		if claims["SUPER_ADMIN"] == true {
			return true
		}
		c.JSON(http.StatusForbidden, gin.H{
			"status":  http.StatusForbidden,
			"message": "Need to be a SUPER_ADMIN to perform such operation",
		})
		c.Abort()
		return false
	}

	return true
}

// GetAuthenticatedUser retrieves the authenticated user from context
func GetAuthenticatedUser(c *gin.Context) *auth.AuthenticatedUser {
	email, _ := c.Get(auth.AUTH_EMAIL)
	userID, _ := c.Get(auth.AUTH_USER_ID)
	claims, _ := c.Get(auth.AUTH_CLAIMS)
	tenantID, _ := c.Get(auth.AUTH_TENANT_ID_KEY)
	tenantMemberships, _ := c.Get(auth.AUTH_TENANT_MEMBERSHIPS)

	emailStr, _ := email.(string)
	userIDStr, _ := userID.(string)
	claimsMap, _ := claims.(map[string]interface{})
	tenantIDStr, _ := tenantID.(string)
	tenantMembershipsSlice, _ := tenantMemberships.([]auth.TenantMembership)

	customClaims := util.FilterMapToArray(claimsMap, util.UppercaseOnly)

	return &auth.AuthenticatedUser{
		UserID:            userIDStr,
		Email:             emailStr,
		Claims:            claimsMap,
		CustomClaims:      customClaims,
		TenantID:          tenantIDStr,
		TenantMemberships: tenantMembershipsSlice,
	}
}
