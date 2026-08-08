package core

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// MembershipLifecycleHandler serves the caller's own membership lifecycle:
// leaving one organization, and closing the account entirely. Two doors, kept
// apart on purpose — the overwhelmingly common intent is "I'm done with this
// organization", and offering only the nuclear option converts a reversible
// membership change into an irreversible deletion (ADR 040).
//
// No operation here takes a tenant id. The tenant is the one the request is
// addressed to, resolved by TenantMiddleware from Origin/Host like every other
// tenant-scoped call.
type MembershipLifecycleHandler struct {
	service *service.MembershipLifecycleService
}

func NewMembershipLifecycleHandler(store *db.Store, authProvider auth.AuthProvider) *MembershipLifecycleHandler {
	return &MembershipLifecycleHandler{
		service: service.NewMembershipLifecycleService(store, authProvider),
	}
}

// GetMyLeaveTenantPreview implements core.ServerInterface.
// (GET /api/v1/users/me/leave-preview)
func (h *MembershipLifecycleHandler) GetMyLeaveTenantPreview(c *gin.Context) {
	userID, tenantID, ok := callerAndTenant(c)
	if !ok {
		return
	}

	preview, err := h.service.LeavePreview(c.Request.Context(), userID, tenantID)
	if err != nil {
		if errors.Is(err, service.ErrNoActiveMembership) {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		logCtxErr(c, err).Msg("leave preview failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, preview)
}

// LeaveTenant implements core.ServerInterface.
// (POST /api/v1/users/me/leave)
func (h *MembershipLifecycleHandler) LeaveTenant(c *gin.Context) {
	userID, tenantID, ok := callerAndTenant(c)
	if !ok {
		return
	}

	var req core.LeaveTenantRequest
	// An empty body is legitimate: a membership with no decisions to make.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
			return
		}
	}

	result, err := h.service.Leave(c.Request.Context(), userID, tenantID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNoActiveMembership):
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		case errors.Is(err, service.ErrDecisionsOutstanding):
			// The current preview comes back with the 409 so the dialog can
			// re-render from the error rather than making a second call.
			preview, previewErr := h.service.LeavePreview(c.Request.Context(), userID, tenantID)
			if previewErr != nil {
				c.JSON(http.StatusConflict, helpers.ErrorResponse(err))
				return
			}
			c.JSON(http.StatusConflict, preview)
		default:
			logCtxErr(c, err).Msg("leave failed")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		}
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetMyAccountDeletion implements core.ServerInterface.
// (GET /api/v1/users/me/deletion)
func (h *MembershipLifecycleHandler) GetMyAccountDeletion(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no caller")))
		return
	}
	status, err := h.service.DeletionStatus(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, status)
}

// GetMyAccountClosurePreview implements core.ServerInterface.
// (GET /api/v1/users/me/closure-preview)
func (h *MembershipLifecycleHandler) GetMyAccountClosurePreview(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no caller")))
		return
	}
	preview, err := h.service.ClosurePreview(c.Request.Context(), userID)
	if err != nil {
		logCtxErr(c, err).Msg("closure preview failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, preview)
}

// ScheduleMyAccountDeletion implements core.ServerInterface.
// (POST /api/v1/users/me/deletion)
func (h *MembershipLifecycleHandler) ScheduleMyAccountDeletion(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no caller")))
		return
	}

	var req core.AccountDeletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	status, err := h.service.ScheduleDeletion(c.Request.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConfirmEmailMismatch):
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		case errors.Is(err, service.ErrDeletionScheduled):
			c.JSON(http.StatusConflict, status)
		default:
			logCtxErr(c, err).Msg("schedule deletion failed")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		}
		return
	}
	c.JSON(http.StatusOK, status)
}

// CancelMyAccountDeletion implements core.ServerInterface.
// (DELETE /api/v1/users/me/deletion)
func (h *MembershipLifecycleHandler) CancelMyAccountDeletion(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no caller")))
		return
	}
	status, err := h.service.CancelDeletion(c.Request.Context(), userID)
	if err != nil {
		logCtxErr(c, err).Msg("cancel deletion failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, status)
}

// ExportMyData implements core.ServerInterface.
// (GET /api/v1/users/me/export)
func (h *MembershipLifecycleHandler) ExportMyData(c *gin.Context) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no caller")))
		return
	}
	export, err := h.service.Export(c.Request.Context(), userID)
	if err != nil {
		logCtxErr(c, err).Msg("export failed")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="my-data.json"`)
	c.JSON(http.StatusOK, export)
}

// logCtxErr starts an error log line from the request context. The logger is
// returned by value and Err has a pointer receiver, so it cannot be called on
// the call expression directly.
func logCtxErr(c *gin.Context, err error) *zerolog.Event {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	return logger.Err(err)
}

// callerAndTenant resolves who is asking and which tenant the request is
// addressed to, answering the client itself when either is missing.
func callerAndTenant(c *gin.Context) (string, string, bool) {
	userID := c.GetString(auth.AUTH_USER_ID)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no caller")))
		return "", "", false
	}
	tenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("no tenant for this host")))
		return "", "", false
	}
	return userID, tenantID, true
}
