package service

import (
	"net/http"
	"strings"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/auth/kratos"
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
		c.Set(auth.REQUEST_URL_PATH, c.Request.URL.Path)
		// Skip auth for public endpoints
		if strings.HasPrefix(c.Request.URL.Path, "/public") {
			c.Next()
			return
		}

		// Check for API token first
		// API token auth is only valid for non-user-management endpoints
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/users/by-email") ||
			(!strings.HasPrefix(c.Request.URL.Path, "/api/v1/users") &&
				!strings.HasPrefix(c.Request.URL.Path, "/admin-api") &&
				!strings.HasPrefix(c.Request.URL.Path, "/superadmin-api")) {

			// X-Api-Key header based authentication can only perform for non-user-management endpoints
			token := c.GetHeader(XApiKeyHeader)
			if token != "" {
				tokenRow, err := am.apiToken.VerifyAPIToken(c, token)
				if err == nil {
					// API token is valid, store info and continue
					c.Set("api_token", tokenRow)
					c.Set("api_token_scopes", tokenRow.Scopes)
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
			}
		}

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
			c.Abort()
			return
		}
		// Check AAL requirements
		if !am.checkAALRequirements(c) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// setAuthenticatedUser stores user info in gin context
func (am *AuthMiddleware) setAuthenticatedUser(c *gin.Context, user *auth.AuthenticatedUser) {
	c.Set(auth.AUTH_EMAIL, user.Email)
	c.Set(auth.AUTH_USER_ID, user.UserID)
	c.Set(auth.AUTH_CLAIMS, user.Claims)

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

// checkAALRequirements validates AAL-based access control
func (am *AuthMiddleware) checkAALRequirements(c *gin.Context) bool {
	// Check AAL requirements for Kratos provider
	kratosProvider, ok := am.authProvider.(*kratos.KratosAuthProvider)
	if !ok {
		return true
	}

	// Applies to mutating methods only
	if !util.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, c.Request.Method) {
		return true
	}

	// Only admin users can alter users
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/users") ||
		strings.HasPrefix(c.Request.URL.Path, "/admin-api") ||
		strings.HasPrefix(c.Request.URL.Path, "/superadmin-api") {
		// Check AAL requirements for Kratos provider
		// Get AAL info (current + available)
		aalInfo, err := kratosProvider.GetSessionAALInfo(c)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get AAL info")
			return false
		}

		// Check if current AAL equals available AAL
		// This ensures user has verified their highest available authentication level
		if aalInfo.Current != aalInfo.Available ||
			(aalInfo.Current == "aal2" && aalInfo.IsAAL2Recent == false) {
			authErr := auth.NewAuthError(
				auth.ErrorCodeSessionAAL2Required,
				"MFA verification required for this operation",
			)
			// Include AAL info in error details
			c.JSON(http.StatusForbidden, authErr)
			c.Abort()
			return false
		}
		return true
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

	return &auth.AuthenticatedUser{
		UserID:            userIDStr,
		Email:             emailStr,
		Claims:            claimsMap,
		TenantID:          tenantIDStr,
		TenantMemberships: tenantMembershipsSlice,
	}
}
