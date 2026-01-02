package service

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// TenantRole represents possible roles within a tenant
type TenantRole string

const (
	TenantRoleOwner TenantRole = "OWNER"
	TenantRoleAdmin TenantRole = "ADMIN"
	TenantRoleUser  TenantRole = "USER"
)

// Context keys for tenant role information
const (
	ContextKeyTenantRoles = "tenant_roles"
)

// TenantRoleService handles tenant-specific role operations
type TenantRoleService struct {
	membershipService *UserTenantMembershipService
}

// NewTenantRoleService creates a new tenant role service
func NewTenantRoleService(membershipService *UserTenantMembershipService) *TenantRoleService {
	return &TenantRoleService{
		membershipService: membershipService,
	}
}

// GetUserTenantRoles retrieves the user's roles in the current tenant from context
func (s *TenantRoleService) GetUserTenantRoles(c *gin.Context) ([]string, error) {
	rolesInterface, exists := c.Get(ContextKeyTenantRoles)
	if !exists {
		return nil, fmt.Errorf("tenant roles not found in context")
	}

	roles, ok := rolesInterface.([]string)
	if !ok {
		return nil, fmt.Errorf("invalid tenant roles type in context")
	}

	return roles, nil
}

// HasTenantRole checks if the user has a specific role in the current tenant
func (s *TenantRoleService) HasTenantRole(c *gin.Context, requiredRole TenantRole) bool {
	roles, err := s.GetUserTenantRoles(c)
	if err != nil {
		return false
	}

	for _, role := range roles {
		if role == string(requiredRole) {
			return true
		}
	}
	return false
}

// HasAnyTenantRole checks if the user has any of the specified roles
func (s *TenantRoleService) HasAnyTenantRole(c *gin.Context, requiredRoles ...TenantRole) bool {
	roles, err := s.GetUserTenantRoles(c)
	if err != nil {
		return false
	}

	for _, role := range roles {
		for _, required := range requiredRoles {
			if role == string(required) {
				return true
			}
		}
	}
	return false
}

// HasMinimumTenantRole checks if the user has at least the specified role level
// Role hierarchy: OWNER > ADMIN > USER
func (s *TenantRoleService) HasMinimumTenantRole(c *gin.Context, minimumRole TenantRole) bool {
	roles, err := s.GetUserTenantRoles(c)
	if err != nil {
		return false
	}

	minimumLevel := getRoleLevel(minimumRole)

	for _, role := range roles {
		if getRoleLevel(TenantRole(role)) >= minimumLevel {
			return true
		}
	}
	return false
}

// RequireTenantRole middleware that requires a specific tenant role
func (s *TenantRoleService) RequireTenantRole(requiredRole TenantRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.HasTenantRole(c, requiredRole) {
			roles, _ := s.GetUserTenantRoles(c)
			log.Warn().
				Str("user_id", c.GetString("user_id")).
				Str("tenant_id", c.GetString("tenant_id")).
				Strs("current_roles", roles).
				Str("required_role", string(requiredRole)).
				Msg("Insufficient tenant role")

			c.JSON(403, gin.H{
				"status":  403,
				"message": fmt.Sprintf("Requires %s role", requiredRole),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAnyTenantRole middleware that requires any of the specified roles
func (s *TenantRoleService) RequireAnyTenantRole(requiredRoles ...TenantRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.HasAnyTenantRole(c, requiredRoles...) {
			roles, _ := s.GetUserTenantRoles(c)
			log.Warn().
				Str("user_id", c.GetString("user_id")).
				Str("tenant_id", c.GetString("tenant_id")).
				Strs("current_roles", roles).
				Msg("Insufficient tenant role")

			c.JSON(403, gin.H{
				"status":  403,
				"message": "Insufficient permissions",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireMinimumTenantRole middleware that requires at least a certain role level
func (s *TenantRoleService) RequireMinimumTenantRole(minimumRole TenantRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.HasMinimumTenantRole(c, minimumRole) {
			roles, _ := s.GetUserTenantRoles(c)
			log.Warn().
				Str("user_id", c.GetString("user_id")).
				Str("tenant_id", c.GetString("tenant_id")).
				Strs("current_roles", roles).
				Str("minimum_role", string(minimumRole)).
				Msg("Insufficient tenant role level")

			c.JSON(403, gin.H{
				"status":  403,
				"message": fmt.Sprintf("Requires at least %s role", minimumRole),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// LoadTenantRolesMiddleware loads the user's tenant roles into the context
// This should run after tenant validation middleware
// NOTE: Not needed if using KratosTenantMiddleware with membershipService
func (s *TenantRoleService) LoadTenantRolesMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		tenantID := c.GetString("tenant_id")

		if userID == "" || tenantID == "" {
			log.Debug().Msg("Skipping tenant roles loading - no user or tenant in context")
			c.Next()
			return
		}

		// Check if user is SUPER_ADMIN (global role)
		if IsSuperAdmin(c) {
			// SUPER_ADMIN gets OWNER role in all tenants
			c.Set(ContextKeyTenantRoles, []string{string(TenantRoleOwner)})
			log.Debug().
				Str("user_id", userID).
				Str("tenant_id", tenantID).
				Msg("SUPER_ADMIN granted OWNER role in tenant")
			c.Next()
			return
		}

		// Get user's roles in this tenant from database
		roles, err := s.membershipService.GetUserTenantRoles(c.Request.Context(), userID, tenantID)
		if err != nil {
			log.Error().
				Err(err).
				Str("user_id", userID).
				Str("tenant_id", tenantID).
				Msg("Failed to get user tenant roles")

			// Default to USER role if we can't determine
			c.Set(ContextKeyTenantRoles, []string{string(TenantRoleUser)})
		} else {
			c.Set(ContextKeyTenantRoles, roles)
			log.Debug().
				Str("user_id", userID).
				Str("tenant_id", tenantID).
				Strs("roles", roles).
				Msg("Tenant roles loaded into context")
		}

		c.Next()
	}
}

// getRoleLevel returns the hierarchical level of a role
// Higher number = more permissions
func getRoleLevel(role TenantRole) int {
	switch role {
	case TenantRoleOwner:
		return 3
	case TenantRoleAdmin:
		return 2
	case TenantRoleUser:
		return 1
	default:
		return 0
	}
}

// IsTenantOwner checks if the user is an owner of the current tenant
func IsTenantOwner(c *gin.Context) bool {
	rolesInterface, exists := c.Get(ContextKeyTenantRoles)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, role := range roles {
		if role == string(TenantRoleOwner) {
			return true
		}
	}
	return false
}

// IsTenantAdmin checks if the user is an admin of the current tenant
func IsTenantAdmin(c *gin.Context) bool {
	rolesInterface, exists := c.Get(ContextKeyTenantRoles)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, role := range roles {
		if role == string(TenantRoleAdmin) {
			return true
		}
	}
	return false
}

// IsTenantAdminOrOwner checks if the user is an admin or owner of the current tenant
func IsTenantAdminOrOwner(c *gin.Context) bool {
	rolesInterface, exists := c.Get(ContextKeyTenantRoles)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, role := range roles {
		if role == string(TenantRoleAdmin) || role == string(TenantRoleOwner) {
			return true
		}
	}
	return false
}

// HasRole checks if user has a specific role
func HasRole(c *gin.Context, role TenantRole) bool {
	rolesInterface, exists := c.Get(ContextKeyTenantRoles)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, r := range roles {
		if r == string(role) {
			return true
		}
	}
	return false
}
