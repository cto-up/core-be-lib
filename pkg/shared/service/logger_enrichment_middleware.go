package service

import (
	"context"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// LoggerEnrichmentMiddleware augments the request-scoped zerolog logger with
// tenant and user identifiers. It must run AFTER the tenant and auth middleware,
// which set those values on the gin context. Fields are only added when present
// and non-empty (tenant is "" on admin/auth subdomains; user is unset on public
// routes), so log lines stay clean for unauthenticated requests.
func LoggerEnrichmentMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		logCtx := util.GetLoggerFromCtx(c.Request.Context()).With()

		if v, ok := c.Get(auth.AUTH_TENANT_ID_KEY); ok {
			if tenantID, ok := v.(string); ok && tenantID != "" {
				logCtx = logCtx.Str("tenant_id", tenantID)
			}
		}
		if v, ok := c.Get(auth.AUTH_USER_ID); ok {
			if userID, ok := v.(string); ok && userID != "" {
				logCtx = logCtx.Str("user_id", userID)
			}
		}

		ctx := context.WithValue(c.Request.Context(), util.LoggerKey, logCtx.Logger())
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
