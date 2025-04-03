package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const AUTH_TENANT_ID_KEY = "auth_tenant_id"

// TenantMiddleware is middleware for Firebase Authentication
type TenantMiddleware struct {
	multitenantService *MultitenantService
}

// New is constructor of the middleware
func NewTenantMiddleware(unAuthorized func(c *gin.Context), multitenantService *MultitenantService) *TenantMiddleware {
	return &TenantMiddleware{
		multitenantService: multitenantService,
	}
}

// MiddlewareFunc is function to verify token
func (fam *TenantMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {

		tenantID, err := fam.multitenantService.GetFirebaseTenantID(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  http.StatusUnauthorized,
				"message": err.Error(),
			})
			c.Abort()
			return
		}
		c.Set(AUTH_TENANT_ID_KEY, tenantID)
		c.Next()
	}
}
