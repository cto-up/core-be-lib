// Package access carries the cross-module "is the caller a tenant
// admin?" flag and the gin middleware that stamps it on the request
// context.
//
// Why this exists in this repo rather than coreapp: core-be-lib is
// vendored and provides only the gin-context role getters
// (auth.IsAdmin / IsCustomerAdmin / IsSuperAdmin). Service-layer code
// that runs without a gin.Context — eino tools, schedulers,
// background jobs — needs the same answer through context.Context's
// Value mechanism. This package is the bridge.
//
// Two concrete consumers today:
//   - care: service-layer authorization helpers (IsUserMemberOfCircle,
//     IsUserAdminOfCircle) short-circuit to "authorized" when the
//     flag is set, giving pro operators tenant-wide reach.
//   - aiemployee: the care_audit_template / care_update_template
//     tools refuse unless the flag is set. Without the bridge, every
//     chat-driven invocation would be blocked even from a SUPER_ADMIN.
//
// Keep this package small. It is access-control plumbing, not a
// business module. New helpers join here only when at least two
// modules need them.
package access

import (
	"context"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
)

// ctxKey is a private named type so other packages can't construct
// the same key by accident — Go's Context.Value compares by interface
// identity, and same-name types in different packages are not equal.
type ctxKey int

const (
	ctxKeyTenantAdmin ctxKey = iota
)

// ContextWithTenantAdmin marks the context as originating from a
// tenant-level admin (SUPER_ADMIN / ADMIN / CUSTOMER_ADMIN).
func ContextWithTenantAdmin(ctx context.Context, isTenantAdmin bool) context.Context {
	return context.WithValue(ctx, ctxKeyTenantAdmin, isTenantAdmin)
}

// IsTenantAdminFromContext reports whether the request was flagged as
// admin-originated by the middleware. Returns false for any context
// that didn't go through the middleware (e.g. background jobs)
// — safe-by-default.
func IsTenantAdminFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(ctxKeyTenantAdmin).(bool)
	return ok && v
}

// TenantAdminContextMiddleware stamps the request context with the
// admin flag based on the gin role getters (auth.Is*). Install on
// every module's RegisterHandler that needs the flag downstream.
//
// Safe on anonymous requests: Is*() return false, the flag stays
// unset, downstream code falls back to its default per-resource
// gates.
func TenantAdminContextMiddleware(c *gin.Context) {
	if auth.IsSuperAdmin(c) || auth.IsAdmin(c) || auth.IsCustomerAdmin(c) {
		ctx := ContextWithTenantAdmin(c.Request.Context(), true)
		c.Request = c.Request.WithContext(ctx)
	}
	c.Next()
}
