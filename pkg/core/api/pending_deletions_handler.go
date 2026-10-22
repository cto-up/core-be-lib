package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

// inactivityDeletesAfter is how long after the inactivity warning a still-quiet
// account is deleted: the sweeper schedules it once the warning period is over,
// and the deletion runs after the grace.
var inactivityDeletesAfter = pgtype.Interval{
	Months:       service.InactivityWarningMonths,
	Microseconds: service.DeletionGrace.Microseconds(),
	Valid:        true,
}

type PendingDeletionsHandler struct {
	store *db.Store
}

func NewPendingDeletionsHandler(store *db.Store) *PendingDeletionsHandler {
	return &PendingDeletionsHandler{store: store}
}

// ListPendingDeletions implements core.ServerInterface. Scoped by the host like
// ListPersonalDataAccess: an ADMIN reads their tenant's members only, a
// SUPER_ADMIN on a host with no tenant reads every account.
func (h *PendingDeletionsHandler) ListPendingDeletions(c *gin.Context, params core.ListPendingDeletionsParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
	if tenantID == "" && !auth.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorStringResponse("a tenant is required"))
		return
	}

	limit, offset := adminPage(params.Page, params.PageSize)
	rows, err := h.store.ListPendingDeletions(c, repository.ListPendingDeletionsParams{
		InactivityDeletesAfter: inactivityDeletesAfter,
		TenantID:               pgtype.Text{String: tenantID, Valid: tenantID != ""},
		RowLimit:               limit,
		RowOffset:              offset,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to list pending deletions")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	out := make([]core.PendingDeletion, len(rows))
	for i, r := range rows {
		stage := core.InactivityWarned
		if r.DeletionScheduledAt.Valid {
			stage = core.ClosureRequested
		}
		out[i] = core.PendingDeletion{
			UserId:      r.ID,
			Email:       r.Email.String,
			Name:        r.Profile.Name,
			Stage:       stage,
			Reason:      util.FromNullableText(r.DeletionReason),
			RequestedAt: util.FromNullableTimestamptz(r.DeletionRequestedAt),
			WarnedAt:    util.FromNullableTimestamptz(r.InactivityWarnedAt),
			LastSeenAt:  util.FromNullableTimestamptz(r.LastSeenAt),
			DeletesOn:   r.DeletesOn,
			Tenants:     r.Tenants,
		}
	}
	c.JSON(http.StatusOK, out)
}
