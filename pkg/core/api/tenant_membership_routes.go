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
	// "My workspaces" and "my invitations" — the invitee's own view. Scoped to
	// the caller by the handler, which reads the user from the auth context and
	// never from the path, so one user cannot read another's invitations.
	me := router.Group("/api/v1/users/me/tenants", middlewares...)
	me.GET("", h.ListUserTenants)
	me.GET("/pending", h.ListPendingInvitations)
	me.POST("/:tenantId/accept", h.AcceptTenantInvitation)
	me.POST("/:tenantId/reject", h.RejectTenantInvitation)

	// Workspace membership administration.
	members := router.Group("/api/v1/tenants/:tenantId/members", middlewares...)
	members.GET("", h.ListTenantMembers)
	members.POST("", h.InviteUserToTenant)
	members.PATCH("/:userId", h.UpdateMemberRole)
	members.DELETE("/:userId", h.RemoveMemberFromTenant)
}
