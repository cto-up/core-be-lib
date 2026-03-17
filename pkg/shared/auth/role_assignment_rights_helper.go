package auth

import (
	"errors"
	"fmt"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Context keys for tenant role information
const (
	CONTEXT_KEY_TENANT_ROLES = "tenant_roles"
	TENANT_IS_RESELLER       = "TENANT_IS_RESELLER"
	ACTING_RESELLER          = "ACTING_RESELLER"
)

func HasRightsForRole(c *gin.Context, role core.Role) error {
	if role == core.CUSTOMERADMIN && (!IsCustomerAdmin(c) && !IsSuperAdmin(c) && !IsAdmin(c)) {
		return errors.New("must be at a CUSTOMER_ADMIN or SUPER_ADMIN or ADMIN to perform such operation")
	}
	if role == core.ADMIN && (!IsSuperAdmin(c) && !IsAdmin(c)) {
		return errors.New("must be an ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == core.SUPERADMIN && !IsSuperAdmin(c) {
		return errors.New("must be an SUPER_ADMIN to perform such operation")
	}
	return nil
}

func HasRightsForRoles(c *gin.Context, roles []core.Role) error {
	for _, role := range roles {
		if err := HasRightsForRole(c, role); err != nil {
			return err
		}
	}
	return nil
}

func IsCustomerAdmin(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isCustomerAdmin := claims.((map[string]interface{}))[string(core.CUSTOMERADMIN)] == true
	return isCustomerAdmin
}

func IsActingReseller(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isActingReseller := claims.((map[string]interface{}))["ACTING_RESELLER"] == true
	return isActingReseller
}

func IsAdmin(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isAdmin := claims.((map[string]interface{}))[string(core.ADMIN)] == true
	return isAdmin
}
func IsSuperAdmin(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	// Works for both Firebase and Kratos:
	// - Firebase: Sets SUPER_ADMIN as custom claim boolean
	// - Kratos: Extracts from global_roles array and sets as boolean for backward compatibility
	isSuperAdmin := claims.((map[string]interface{}))[string(core.SUPERADMIN)] == true
	return isSuperAdmin
}

func IsAllowedToManageTenantByID(c *gin.Context, store *db.Store, id uuid.UUID) (bool, error) {
	if IsSuperAdmin(c) {
		return true, nil
	}

	existing, err := store.GetTenantByID(c, id)
	if err != nil {
		return false, err
	}

	authTenantID := c.GetString(AUTH_TENANT_ID_KEY)
	if IsReseller(c) {
		if !existing.ResellerID.Valid || existing.ResellerID.String != authTenantID {
			return false, errors.New("not allowed to manage this tenant")
		}
	} else {
		return false, errors.New("not allowed to perform this operation")
	}
	return true, nil
}
func IsAllowedToManageTenant(c *gin.Context, tenant repository.CoreTenant) bool {
	if IsSuperAdmin(c) {
		return true
	}
	if IsReseller(c) {
		authTenantID := c.GetString(AUTH_TENANT_ID_KEY)
		if tenant.ResellerID.Valid && tenant.ResellerID.String == authTenantID {
			return true
		}
	}
	return false
}

func IsReseller(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isReseller := claims.((map[string]interface{}))[TENANT_IS_RESELLER] == true
	return isReseller
}

// GetUserTenantRoles retrieves the user's roles in the current tenant from context
func GetUserTenantRoles(c *gin.Context) ([]string, error) {
	rolesInterface, exists := c.Get(CONTEXT_KEY_TENANT_ROLES)
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
func HasTenantRole(c *gin.Context, requiredRole string) bool {
	roles, err := GetUserTenantRoles(c)
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
func HasAnyTenantRole(c *gin.Context, requiredRoles ...string) bool {
	roles, err := GetUserTenantRoles(c)
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
// Role hierarchy: ADMIN > CUSTOMER_ADMIN > USER
func HasMinimumTenantRole(c *gin.Context, minimumRole string) bool {
	roles, err := GetUserTenantRoles(c)
	if err != nil {
		return false
	}

	minimumLevel := GetRoleLevel(minimumRole)

	for _, role := range roles {
		if GetRoleLevel(role) >= minimumLevel {
			return true
		}
	}
	return false
}

// GetRoleLevel returns the hierarchical level of a role
// Higher number = more permissions
func GetRoleLevel(role string) int {
	switch role {
	case string(core.ADMIN):
		return 3
	case string(core.CUSTOMERADMIN):
		return 2
	case string(core.USER):
		return 1
	default:
		return 0
	}
}

// IsTenantOwner checks if the user is an owner of the current tenant
func IsTenantAdmin(c *gin.Context) bool {
	rolesInterface, exists := c.Get(CONTEXT_KEY_TENANT_ROLES)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, role := range roles {
		if role == string(core.ADMIN) {
			return true
		}
	}
	return false
}

// IsTenantAdmin checks if the user is an admin of the current tenant
func IsTenantCustomerAdmin(c *gin.Context) bool {
	rolesInterface, exists := c.Get(CONTEXT_KEY_TENANT_ROLES)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, role := range roles {
		if role == string(core.CUSTOMERADMIN) {
			return true
		}
	}
	return false
}

// IsTenantAdminOrOwner checks if the user is an admin or owner of the current tenant
func IsTenantAdminOrCustomerAdmin(c *gin.Context) bool {
	rolesInterface, exists := c.Get(CONTEXT_KEY_TENANT_ROLES)
	if !exists {
		return false
	}
	roles, ok := rolesInterface.([]string)
	if !ok {
		return false
	}
	for _, role := range roles {
		if role == string(core.ADMIN) || role == string(core.CUSTOMERADMIN) {
			return true
		}
	}
	return false
}
