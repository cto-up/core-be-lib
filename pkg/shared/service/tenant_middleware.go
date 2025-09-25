package service

import (
	"net/http"

	utils "ctoup.com/coreapp/pkg/shared/util"
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
	return func(ctx *gin.Context) {
		subdomain, err := utils.GetSubdomain(ctx)

		// get tenant from context using subdomain
		tenantID, err := fam.multitenantService.GetFirebaseTenantID(ctx, subdomain)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"status":  http.StatusUnauthorized,
				"message": err.Error(),
			})
			ctx.Abort()
			return
		}
		ctx.Set(AUTH_TENANT_ID_KEY, tenantID)
		ctx.Next()
	}
}
