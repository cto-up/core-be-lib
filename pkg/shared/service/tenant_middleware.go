package service

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// TenantMiddleware is middleware to extract tenant information from the request and set it in the context
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

		// Admin/auth subdomains have no tenant — let the request through with no
		// tenant context.
		if utils.IsAdminSubdomain(subdomain) || subdomain == "auth" {
			ctx.Set(auth.AUTH_TENANT_ID_KEY, "")
			ctx.Next()
			return
		}

		// One DB call per (subdomain, cache TTL): GetTenantBySubdomainCached
		// loads the full tenant and warms both the subdomain → tenant_id map
		// and the tenant-record cache. Downstream middleware and handlers read
		// flags (IsDisabled, IsReseller, AllowSignUp, …) off the gin.Context.
		tenant, err := fam.multitenantService.GetTenantBySubdomainCached(ctx, subdomain)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				log.Info().Str("subdomain", subdomain).Msg("Tenant not found")
				ctx.JSON(http.StatusNotFound, gin.H{
					"status":  http.StatusNotFound,
					"message": "Tenant not found",
				})
			} else {
				log.Err(err).Msg("Failed to load tenant by subdomain")
				ctx.JSON(http.StatusInternalServerError, gin.H{
					"status":  http.StatusInternalServerError,
					"message": err.Error(),
				})
			}
			ctx.Abort()
			return
		}
		if tenant.IsDisabled {
			ctx.JSON(http.StatusForbidden, gin.H{
				"status":  http.StatusForbidden,
				"message": "Tenant account has been suspended",
			})
			ctx.Abort()
			return
		}

		ctx.Set(auth.AUTH_TENANT, tenant)
		ctx.Set(auth.AUTH_TENANT_ID_KEY, tenant.TenantID)
		ctx.Next()
	}
}
