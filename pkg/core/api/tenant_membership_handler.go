package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type TenantMembershipHandler struct {
	store             *db.Store
	authProvider      auth.AuthProvider
	membershipService *service.UserTenantMembershipService
}

func NewTenantMembershipHandler(
	store *db.Store,
	authProvider auth.AuthProvider,
	membershipService *service.UserTenantMembershipService,
) *TenantMembershipHandler {
	return &TenantMembershipHandler{
		store:             store,
		authProvider:      authProvider,
		membershipService: membershipService,
	}
}

// ListUserTenants returns all tenants the current user belongs to
// GET /api/v1/users/me/tenants
func (h *TenantMembershipHandler) ListUserTenants(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(nil))
		return
	}

	memberships, err := h.membershipService.GetUserTenants(c, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user tenants")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, memberships)
}

// ListPendingInvitations returns all pending invitations for the current user
// GET /api/v1/users/me/tenants/pending
func (h *TenantMembershipHandler) ListPendingInvitations(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(nil))
		return
	}

	invitations, err := h.membershipService.GetPendingInvitations(c, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get pending invitations")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, invitations)
}

// AcceptTenantInvitation accepts a pending invitation
// POST /api/v1/users/me/tenants/{tenantId}/accept
func (h *TenantMembershipHandler) AcceptTenantInvitation(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(nil))
		return
	}

	tenantID := c.Param("tenantId")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	err := h.membershipService.AcceptTenantInvitation(c, userID, tenantID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to accept invitation")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation accepted"})
}

// RejectTenantInvitation rejects a pending invitation
// POST /api/v1/users/me/tenants/{tenantId}/reject
func (h *TenantMembershipHandler) RejectTenantInvitation(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(nil))
		return
	}

	tenantID := c.Param("tenantId")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	err := h.membershipService.RejectTenantInvitation(c, userID, tenantID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reject invitation")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// SetPrimaryTenant sets the user's primary tenant
// POST /api/v1/users/me/primary-tenant
func (h *TenantMembershipHandler) SetPrimaryTenant(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(nil))
		return
	}

	var req struct {
		TenantID string `json:"tenant_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err := h.membershipService.SwitchPrimaryTenant(c, userID, req.TenantID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set primary tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Primary tenant updated"})
}

// ListTenantMembers returns all members of a tenant (requires ADMIN role)
// GET /api/v1/tenants/{tenantId}/members
func (h *TenantMembershipHandler) ListTenantMembers(c *gin.Context) {
	tenantID := c.Param("tenantId")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	status := c.DefaultQuery("status", "active")

	members, err := h.membershipService.GetTenantMembers(c, tenantID, status)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get tenant members")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, members)
}

// InviteUserToTenant invites a user to join the tenant (requires ADMIN role)
// POST /api/v1/tenants/{tenantId}/members
func (h *TenantMembershipHandler) InviteUserToTenant(c *gin.Context) {
	tenantID := c.Param("tenantId")
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	inviterID := c.GetString(auth.AUTH_USER_ID)
	if inviterID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(nil))
		return
	}

	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role" binding:"required,oneof=USER ADMIN"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err := h.membershipService.InviteUserToTenant(c, req.Email, tenantID, req.Role, inviterID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to invite user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User invited"})
}

// UpdateMemberRole updates a member's role in the tenant (requires ADMIN role)
// PATCH /api/v1/tenants/{tenantId}/members/{userId}
func (h *TenantMembershipHandler) UpdateMemberRole(c *gin.Context) {
	tenantID := c.Param("tenantId")
	userID := c.Param("userId")

	if tenantID == "" || userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id and user_id are required"})
		return
	}

	var req struct {
		Role string `json:"role" binding:"required,oneof=USER ADMIN OWNER"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err := h.membershipService.UpdateMemberRole(c, userID, tenantID, req.Role)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update member role")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member role updated"})
}

// RemoveMemberFromTenant removes a member from the tenant (requires ADMIN role)
// DELETE /api/v1/tenants/{tenantId}/members/{userId}
func (h *TenantMembershipHandler) RemoveMemberFromTenant(c *gin.Context) {
	tenantID := c.Param("tenantId")
	userID := c.Param("userId")

	if tenantID == "" || userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id and user_id are required"})
		return
	}

	err := h.membershipService.RemoveUserFromTenant(c, userID, tenantID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove member")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}
