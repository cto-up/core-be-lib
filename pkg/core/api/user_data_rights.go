package core

import (
	"errors"
	"net/http"
	"slices"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// A member's data-protection rights, exercised by an administrator on their
// behalf: the same Export, Leave and ScheduleDeletion the member reaches from
// their Organizations page, behind the user handlers' guards.
//
// Leaving concerns the administrator's tenant only, so it is always theirs to
// do. Export and closure reach every organization the person belongs to, so a
// tenant ADMIN may do them only for somebody who belongs to no other active
// tenant; a SUPER_ADMIN may always.

const exportResource = "core.user_export"

var (
	msgOtherTenants = "This person also belongs to another organization; their data there is not yours to export or close. A platform administrator can do it, or they can do it from their account."
	msgScheduled    = "This account is already scheduled for deletion."
)

// memberInScope resolves the tenant and applies the guards every on-behalf
// action shares. accountWide adds the other-tenants rule. The caller stops
// when it returns false.
func (o *userOps) memberInScope(c *gin.Context, scope TenantScope, userID string, accountWide bool) (scoped, bool) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	if !o.refuseSelf(c, userID, "Use your own Organizations page for your own data") {
		return scoped{}, false
	}
	if !o.requireAdminPrivileges(c, "act on a member's data") {
		return scoped{}, false
	}
	target, ok := resolve(c, scope)
	if !ok {
		return scoped{}, false
	}
	if target.id == "" {
		c.JSON(http.StatusBadRequest, helpers.ErrorStringResponse("a tenant is required"))
		return scoped{}, false
	}

	isMember, err := o.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID: userID, TenantID: target.id,
	})
	if err != nil {
		logger.Err(err).Msg("failed to check user membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return scoped{}, false
	}
	if !isMember {
		c.JSON(http.StatusNotFound, helpers.ErrorStringResponse("user not found in this tenant"))
		return scoped{}, false
	}
	if !o.refuseIfTargetOutranks(c, target.id, userID) {
		return scoped{}, false
	}
	// Tenant roles say nothing about a platform administrator.
	if !auth.IsSuperAdmin(c) {
		user, err := o.store.GetSharedUserByID(c, userID)
		if err != nil {
			logger.Err(err).Msg("failed to get user")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return scoped{}, false
		}
		if slices.Contains(user.Roles, string(core.SUPERADMIN)) {
			c.JSON(http.StatusForbidden, helpers.ErrorStringResponse("caller does not have rights over this user"))
			return scoped{}, false
		}
	}

	if accountWide && !auth.IsSuperAdmin(c) {
		err := access.CheckUserNotActiveInOtherTenants(c, o.store, userID, target.id)
		var blocked *access.ErrUserActiveInOtherTenants
		switch {
		case errors.As(err, &blocked):
			// Names of the other organizations stay out: they are not this
			// tenant's to know.
			c.JSON(http.StatusConflict, core.MemberActionRefusal{Reason: core.MemberOfOtherTenants, Message: msgOtherTenants})
			return scoped{}, false
		case err != nil:
			logger.Err(err).Msg("failed to check active tenant memberships")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return scoped{}, false
		}
	}
	return target, true
}

func (o *userOps) lifecycle() *service.MembershipLifecycleService {
	return service.NewMembershipLifecycleService(o.store, o.authProvider)
}

func (o *userOps) exportMemberData(c *gin.Context, scope TenantScope, userID string) {
	target, ok := o.memberInScope(c, scope, userID, true)
	if !ok {
		return
	}
	export, err := o.lifecycle().Export(c.Request.Context(), userID)
	if err != nil {
		logCtxErr(c, err).Msg("member export failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	access.RecordPersonalDataAccess(c, access.PersonalDataAccess{
		TenantID:      target.id,
		ActorUserID:   c.GetString(auth.AUTH_USER_ID),
		SubjectUserID: userID,
		Resource:      exportResource,
	})
	c.Header("Content-Disposition", `attachment; filename="member-data.json"`)
	c.JSON(http.StatusOK, export)
}

func (o *userOps) memberLeavePreview(c *gin.Context, scope TenantScope, userID string) {
	target, ok := o.memberInScope(c, scope, userID, false)
	if !ok {
		return
	}
	preview, err := o.lifecycle().LeavePreview(c.Request.Context(), userID, target.id)
	if err != nil {
		if errors.Is(err, service.ErrNoActiveMembership) {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		logCtxErr(c, err).Msg("member leave preview failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (o *userOps) leaveTenantForMember(c *gin.Context, scope TenantScope, userID string) {
	target, ok := o.memberInScope(c, scope, userID, false)
	if !ok {
		return
	}
	var req core.LeaveTenantRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
			return
		}
	}

	lifecycle := o.lifecycle()
	if _, err := lifecycle.Leave(c.Request.Context(), userID, target.id, req); err != nil {
		switch {
		case errors.Is(err, service.ErrNoActiveMembership):
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		case errors.Is(err, service.ErrDecisionsOutstanding):
			preview, previewErr := lifecycle.LeavePreview(c.Request.Context(), userID, target.id)
			if previewErr != nil {
				c.JSON(http.StatusConflict, helpers.ErrorResponse(err))
				return
			}
			c.JSON(http.StatusConflict, preview)
		default:
			logCtxErr(c, err).Msg("member leave failed")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		}
		return
	}
	logger := util.GetLoggerFromCtx(c.Request.Context())
	logger.Info().
		Str("user_id", userID).Str("tenant_id", target.id).Str("caller_id", c.GetString(auth.AUTH_USER_ID)).
		Msg("member removed from the organization by an administrator")
	c.Status(http.StatusNoContent)
}

func (o *userOps) scheduleMemberAccountDeletion(c *gin.Context, scope TenantScope, userID string) {
	target, ok := o.memberInScope(c, scope, userID, true)
	if !ok {
		return
	}
	var req core.MemberAccountDeletionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
			return
		}
	}

	closure := service.AdminClosure{TenantName: target.name}
	if req.Reason != nil {
		closure.Reason = *req.Reason
	}
	if tenant, err := o.store.GetTenantByTenantID(c, target.id); err == nil {
		closure.TenantName = tenant.Name
	}
	if url, err := buildTenantURL(c, "/user/me/organizations", target.subdomain); err == nil {
		closure.KeepURL = url
	}

	status, err := o.lifecycle().ScheduleDeletionByAdmin(c.Request.Context(), userID, closure)
	if err != nil {
		if errors.Is(err, service.ErrDeletionScheduled) {
			c.JSON(http.StatusConflict, core.MemberActionRefusal{Reason: core.DeletionAlreadyScheduled, Message: msgScheduled})
			return
		}
		logCtxErr(c, err).Msg("member account closure failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	logger := util.GetLoggerFromCtx(c.Request.Context())
	logger.Warn().
		Str("user_id", userID).Str("tenant_id", target.id).Str("caller_id", c.GetString(auth.AUTH_USER_ID)).
		Msg("account closure scheduled by an administrator")
	c.JSON(http.StatusOK, status)
}

// (GET /admin-api/v1/users/{userid}/export)
func (uh *UserAdminHandler) ExportMemberData(c *gin.Context, userid string) {
	uh.exportMemberData(c, uh.SessionTenant, userid)
}

// (GET /admin-api/v1/users/{userid}/leave-preview)
func (uh *UserAdminHandler) GetMemberLeavePreview(c *gin.Context, userid string) {
	uh.memberLeavePreview(c, uh.SessionTenant, userid)
}

// (POST /admin-api/v1/users/{userid}/leave)
func (uh *UserAdminHandler) LeaveTenantForMember(c *gin.Context, userid string) {
	uh.leaveTenantForMember(c, uh.SessionTenant, userid)
}

// (POST /admin-api/v1/users/{userid}/deletion)
func (uh *UserAdminHandler) ScheduleMemberAccountDeletion(c *gin.Context, userid string) {
	uh.scheduleMemberAccountDeletion(c, uh.SessionTenant, userid)
}

// (GET /superadmin-api/v1/tenants/{tenantid}/users/{userid}/export)
func (uh *UserSuperAdminHandler) ExportMemberDataFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.exportMemberData(c, uh.ManagedTenant(tenantId), userid)
}

// (GET /superadmin-api/v1/tenants/{tenantid}/users/{userid}/leave-preview)
func (uh *UserSuperAdminHandler) GetMemberLeavePreviewFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.memberLeavePreview(c, uh.ManagedTenant(tenantId), userid)
}

// (POST /superadmin-api/v1/tenants/{tenantid}/users/{userid}/leave)
func (uh *UserSuperAdminHandler) LeaveTenantForMemberFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.leaveTenantForMember(c, uh.ManagedTenant(tenantId), userid)
}

// (POST /superadmin-api/v1/tenants/{tenantid}/users/{userid}/deletion)
func (uh *UserSuperAdminHandler) ScheduleMemberAccountDeletionFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.scheduleMemberAccountDeletion(c, uh.ManagedTenant(tenantId), userid)
}
