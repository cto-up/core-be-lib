package service

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// KratosTenantMiddleware validates tenant context for Kratos-authenticated requests
// This middleware should run AFTER AuthMiddleware to ensure user is authenticated
type KratosTenantMiddleware struct {
	multitenantService *MultitenantService
	authProvider       auth.AuthProvider
	membershipService  *UserTenantMembershipService
}

// NewKratosTenantMiddleware creates a new tenant validation middleware for Kratos
func NewKratosTenantMiddleware(
	multitenantService *MultitenantService,
	authProvider auth.AuthProvider,
	membershipService *UserTenantMembershipService,
) *KratosTenantMiddleware {
	return &KratosTenantMiddleware{
		multitenantService: multitenantService,
		authProvider:       authProvider,
		membershipService:  membershipService,
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
			}
			c.Next()
			return
		}

		// SUPER_ADMIN can access any tenant
		if isSuperAdmin {
			// Get tenant ID from database for context, but don't validate user's tenant
			tenantID, err := ktm.multitenantService.GetFirebaseTenantID(c, subdomain)
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
			c.Set("tenant_subdomain", subdomain)

			log.Debug().
				Str("tenant_id", tenantID).
				Str("subdomain", subdomain).
				Str("user_id", c.GetString(auth.AUTH_USER_ID)).
				Msg("SUPER_ADMIN accessing tenant - validation bypassed")

			c.Next()
			return
		}

		// Get tenant ID from database using subdomain
		tenantID, err := ktm.multitenantService.GetFirebaseTenantID(c, subdomain)
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

		// Check if user has access to this tenant via tenant_memberships in session
		// This is stored in the session by the auth middleware from Kratos metadata_public
		tenantMembershipsInterface, exists := c.Get("tenant_memberships")
		if exists {
			tenantMemberships, ok := tenantMembershipsInterface.([]string)
			if ok {
				// Check if user has access to this tenant
				hasAccess := false
				for _, memberTenantID := range tenantMemberships {
					if memberTenantID == tenantID {
						hasAccess = true
						break
					}
				}

				if !hasAccess {
					log.Warn().
						Str("user_id", userID).
						Str("tenant_id", tenantID).
						Str("subdomain", subdomain).
						Strs("user_tenants", tenantMemberships).
						Msg("User does not have access to tenant")
					c.JSON(http.StatusForbidden, gin.H{
						"status":  http.StatusForbidden,
						"message": "Access denied: no membership in this tenant",
					})
					c.Abort()
					return
				}

				// User has valid membership
				c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
				c.Set("tenant_subdomain", subdomain)

				log.Debug().
					Str("tenant_id", tenantID).
					Str("subdomain", subdomain).
					Str("user_id", userID).
					Msg("Tenant access validated via session memberships")

				c.Next()
				return
			}
		}

		// Fallback: Check database if tenant_memberships not in session
		// This handles cases where session was created before membership feature
		if ktm.membershipService != nil {
			hasAccess, err := ktm.membershipService.CheckUserTenantAccess(c, userID, tenantID)
			if err != nil {
				log.Error().
					Err(err).
					Str("user_id", userID).
					Str("tenant_id", tenantID).
					Msg("Failed to check user tenant access")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
				c.Abort()
				return
			}

			if !hasAccess {
				log.Warn().
					Str("user_id", userID).
					Str("tenant_id", tenantID).
					Str("subdomain", subdomain).
					Msg("User does not have access to tenant (DB check)")
				c.JSON(http.StatusForbidden, gin.H{
					"status":  http.StatusForbidden,
					"message": "Access denied: no membership in this tenant",
				})
				c.Abort()
				return
			}

			// User has valid membership - update session for next time
			log.Info().
				Str("user_id", userID).
				Str("tenant_id", tenantID).
				Msg("User validated via DB - session should be refreshed")

			c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
			c.Set("tenant_subdomain", subdomain)

			log.Debug().
				Str("tenant_id", tenantID).
				Str("subdomain", subdomain).
				Str("user_id", userID).
				Msg("Tenant access validated via database fallback")

			c.Next()
			return
		}

		// Final fallback to metadata-based validation (legacy)
		userTenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
		if !exists {
			log.Warn().
				Str("user_id", userID).
				Str("subdomain", subdomain).
				Msg("User has no tenant metadata or memberships")

			c.JSON(http.StatusForbidden, gin.H{
				"status":  http.StatusForbidden,
				"message": "Access denied: no tenant assignment",
			})
			c.Abort()
			return
		}

		userTenantIDStr, ok := userTenantID.(string)
		if !ok || userTenantIDStr != tenantID {
			log.Error().
				Str("user_tenant", userTenantIDStr).
				Str("requested_tenant", tenantID).
				Str("subdomain", subdomain).
				Msg("Tenant mismatch")

			c.JSON(http.StatusForbidden, gin.H{
				"status":  http.StatusForbidden,
				"message": "Access denied: tenant mismatch",
			})
			c.Abort()
			return
		}

		c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
		c.Set("tenant_subdomain", subdomain)

		log.Debug().
			Str("tenant_id", tenantID).
			Str("subdomain", subdomain).
			Str("user_id", userID).
			Msg("Tenant context validated via legacy metadata")

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
