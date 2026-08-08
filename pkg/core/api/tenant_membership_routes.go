package core

import (
	"github.com/gin-gonic/gin"
)

// RegisterTenantMembershipRoutes mounts the self-service membership endpoints.
//
// These are registered here rather than generated from the OpenAPI spec because
// they were never in it: the handler existed, fully stubbed and completely
// unmounted, so nothing had to be un-generated — only mounted. Core keeps
// ownership of its own routes rather than each consumer hand-registering them.
//
// middlewares are the consumer's auth chain. Every route here needs an
// authenticated caller: two act on "me", and four act on a workspace's
// membership.
func RegisterTenantMembershipRoutes(router gin.IRouter, h *TenantMembershipHandler, middlewares ...gin.HandlerFunc) {
	// The "me" group moved into the OpenAPI spec (ADR 040 §8) and is mounted by
	// the generated router. Registering it here as well would bind each route
	// twice and gin panics at boot.
	//
	// Workspace membership administration.
	members := router.Group("/api/v1/tenants/:tenantId/members", middlewares...)
	members.GET("", h.ListTenantMembers)
	members.POST("", h.InviteUserToTenant)
	members.PATCH("/:userId", h.UpdateMemberRole)
	members.DELETE("/:userId", h.RemoveMemberFromTenant)
}
