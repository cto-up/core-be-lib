package service

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// KratosTenantMiddleware validates tenant context for Kratos-authenticated requests
// This middleware MUST run AFTER AuthMiddleware to ensure user is authenticated
// and claims are available in the context
type KratosTenantMiddleware struct {
	multitenantService *MultitenantService
	authProvider       auth.AuthProvider
}

// NewKratosTenantMiddleware creates a new tenant validation middleware for Kratos
func NewKratosTenantMiddleware(
	multitenantService *MultitenantService,
	authProvider auth.AuthProvider,
) *KratosTenantMiddleware {
	return &KratosTenantMiddleware{
		multitenantService: multitenantService,
		authProvider:       authProvider,
	}
}

// MiddlewareFunc validates tenant context from subdomain and user metadata
func (ktm *KratosTenantMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract subdomain from request
		subdomain, err := utils.GetSubdomain(c)
		if err != nil {
			log.Error().Err(err).Msg("Failed to extract subdomain")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subdomain"})
			c.Abort()
			return
		}

		// Check if user is SUPER_ADMIN
		isSuperAdmin := IsSuperAdmin(c)

		// Skip tenant validation for root domain or SUPER_ADMIN
		if subdomain == "" || subdomain == "www" {
			if isSuperAdmin {
				log.Debug().
					Str("user_id", c.GetString(auth.AUTH_USER_ID)).
					Msg("SUPER_ADMIN accessing root domain - no tenant required")
				c.Next()
				return
			}
			log.Error().Msg("Tenant subdomain required for non-SUPER_ADMIN users")
			c.Abort()
			return
		}

		// SUPER_ADMIN can access any tenant with ADMIN role
		if isSuperAdmin {
			// Get tenant ID from database for context, but don't validate user's tenant
			tenantID, err := ktm.multitenantService.GetTenantIDWithSubdomain(c, subdomain)
			if err != nil {
				log.Error().Err(err).Str("subdomain", subdomain).Msg("Tenant not found")
				c.JSON(http.StatusNotFound, gin.H{
					"status":  http.StatusNotFound,
					"message": "Tenant not found",
				})
				c.Abort()
				return
			}

			// Set tenant context for downstream handlers
			c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)

			log.Debug().
				Str("tenant_id", tenantID).
				Str("subdomain", subdomain).
				Str("user_id", c.GetString(auth.AUTH_USER_ID)).
				Strs("roles", []string{"ADMIN"}).
				Msg("SUPER_ADMIN accessing tenant with ADMIN role")

			c.Next()
			return
		}

		// Get tenant ID from database using subdomain
		tenantID, err := ktm.multitenantService.GetTenantIDWithSubdomain(c, subdomain)
		if err != nil {
			log.Error().Err(err).Str("subdomain", subdomain).Msg("Tenant not found")
			c.JSON(http.StatusNotFound, gin.H{
				"status":  http.StatusNotFound,
				"message": "Tenant not found",
			})
			c.Abort()
			return
		}

		// Get user ID
		userID := c.GetString(auth.AUTH_USER_ID)
		if userID == "" {
			log.Error().Msg("User ID not found in context")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		// Get tenant memberships from context (set by AuthMiddleware)
		tenantMembershipsInterface, exists := c.Get(auth.AUTH_TENANT_MEMBERSHIPS)
		if !exists {
			log.Warn().
				Str("user_id", userID).
				Str("subdomain", subdomain).
				Msg("User has no tenant memberships in metadata")

			c.JSON(http.StatusForbidden, gin.H{
				"status":  http.StatusForbidden,
				"message": "Access denied: no tenant assignment",
			})
			c.Abort()
			return
		}

		tenantMemberships, ok := tenantMembershipsInterface.([]auth.TenantMembership)
		if !ok {
			log.Error().
				Str("user_id", userID).
				Msg("Invalid tenant memberships format in context")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			c.Abort()
			return
		}

		// Find membership for this tenant
		var userRoles []string
		hasAccess := false
		for _, membership := range tenantMemberships {
			if membership.TenantID == tenantID {
				hasAccess = true
				userRoles = membership.Roles
				break
			}
		}

		if !hasAccess {
			log.Warn().
				Str("user_id", userID).
				Str("tenant_id", tenantID).
				Str("subdomain", subdomain).
				Msg("User does not have access to tenant")
			c.JSON(http.StatusForbidden, gin.H{
				"status":  http.StatusForbidden,
				"message": "Access denied: no membership in this tenant",
			})
			c.Abort()
			return
		}

		// User has valid membership - set context
		c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)

		log.Debug().
			Str("tenant_id", tenantID).
			Str("subdomain", subdomain).
			Str("user_id", userID).
			Strs("roles", userRoles).
			Msg("Tenant access validated with roles from metadata")

		c.Next()
	}
}

// GetTenantContext retrieves tenant information from gin context
// Returns empty strings if no tenant context (e.g., SUPER_ADMIN on root domain)
func GetTenantContext(c *gin.Context) (tenantID string, subdomain string, ok bool) {
	tenantIDVal, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		return "", "", false
	}

	subdomainVal, _ := c.Get("tenant_subdomain")

	tenantID, _ = tenantIDVal.(string)
	subdomain, _ = subdomainVal.(string)

	return tenantID, subdomain, tenantID != ""
}

// RequiresTenantContext checks if the current request requires tenant context
// SUPER_ADMIN users don't require tenant context
func RequiresTenantContext(c *gin.Context) bool {
	return !IsSuperAdmin(c)
}
