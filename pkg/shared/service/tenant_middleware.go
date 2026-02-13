package service

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
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
			if err.Error() == pgx.ErrNoRows.Error() {
				log.Info().Msg("Failed to get tenant ID with subdomain: " + subdomain)
				ctx.JSON(http.StatusNotFound, gin.H{
					"status":  http.StatusNotFound,
					"message": "Tenant not found",
				})
			} else {
				log.Error().Err(err).Msg("Failed to get tenant ID with subdomain")
				ctx.JSON(http.StatusInternalServerError, gin.H{
					"status":  http.StatusInternalServerError,
					"message": err.Error(),
				})
			}
			ctx.Abort()
			return
		}
		ctx.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
		ctx.Next()
	}
}
