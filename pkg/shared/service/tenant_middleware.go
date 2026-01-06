package service

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

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
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			ctx.Abort()
			return
		}

		// get tenant from context using subdomain
		tenantID, err := fam.multitenantService.GetTenantIDWithSubdomain(ctx, subdomain)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"status":  http.StatusUnauthorized,
				"message": err.Error(),
			})
			ctx.Abort()
			return
		}
		ctx.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
		ctx.Next()
	}
}
