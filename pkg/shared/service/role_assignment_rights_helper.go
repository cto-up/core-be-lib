package service

import (
	"errors"
	"fmt"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
)

// GlobalRole represents possible roles with global scope
type GlobalRole string

const (
	GlobalRoleSuperAdmin GlobalRole = "SUPER_ADMIN"
)

// TenantRole represents possible roles within a tenant
type TenantRole string

const (
	TenantRoleCustomerAdmin TenantRole = "CUSTOMER_ADMIN"
	TenantRoleAdmin         TenantRole = "ADMIN"
	TenantRoleUser          TenantRole = "USER"
)

func HasRightsForRole(c *gin.Context, role core.Role) error {
	if role == "CUSTOMER_ADMIN" && (!IsCustomerAdmin(c) && !IsSuperAdmin(c) && !IsAdmin(c)) {
		return errors.New("must be at a CUSTOMER_ADMIN or SUPER_ADMIN or ADMIN to perform such operation")
	}
	if role == "ADMIN" && (!IsSuperAdmin(c) && !IsAdmin(c)) {
		return errors.New("must be an ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == "SUPER_ADMIN" && !IsSuperAdmin(c) {
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
	claims, exist := c.Get(auth.AUTH_CLAIMS)
	if !exist {
		return false
	}
	isCustomerAdmin := claims.((map[string]interface{}))["CUSTOMER_ADMIN"] == true
	return isCustomerAdmin
}

func IsAdmin(c *gin.Context) bool {
	claims, exist := c.Get(auth.AUTH_CLAIMS)
	if !exist {
		return false
	}
	isAdmin := claims.((map[string]interface{}))["ADMIN"] == true
	return isAdmin
}
func IsSuperAdmin(c *gin.Context) bool {
	claims, exist := c.Get(auth.AUTH_CLAIMS)
	if !exist {
		return false
	}
	// Works for both Firebase and Kratos:
	// - Firebase: Sets SUPER_ADMIN as custom claim boolean
	// - Kratos: Extracts from global_roles array and sets as boolean for backward compatibility
	isSuperAdmin := claims.((map[string]interface{}))["SUPER_ADMIN"] == true
	return isSuperAdmin
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
func HasTenantRole(c *gin.Context, requiredRole TenantRole) bool {
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
func HasAnyTenantRole(c *gin.Context, requiredRoles ...TenantRole) bool {
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
func HasMinimumTenantRole(c *gin.Context, minimumRole TenantRole) bool {
	roles, err := GetUserTenantRoles(c)
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

// getRoleLevel returns the hierarchical level of a role
// Higher number = more permissions
func getRoleLevel(role TenantRole) int {
	switch role {
	case TenantRoleAdmin:
		return 3
	case TenantRoleCustomerAdmin:
		return 2
	case TenantRoleUser:
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
		if role == string(TenantRoleAdmin) {
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
		if role == string(TenantRoleCustomerAdmin) {
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
		if role == string(TenantRoleAdmin) || role == string(TenantRoleCustomerAdmin) {
			return true
		}
	}
	return false
}
