package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxPersonalDataAccessPageSize = 100

type PersonalDataAccessHandler struct {
	store *db.Store
}

func NewPersonalDataAccessHandler(store *db.Store) *PersonalDataAccessHandler {
	return &PersonalDataAccessHandler{store: store}
}

// ListPersonalDataAccess implements core.ServerInterface. The tenant filter
// comes from the host, never from the query: an ADMIN reads their own tenant's
// rows only, and a SUPER_ADMIN gets every tenant's on a host with no tenant.
func (h *PersonalDataAccessHandler) ListPersonalDataAccess(c *gin.Context, params core.ListPersonalDataAccessParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
	if tenantID == "" && !auth.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorStringResponse("a tenant is required"))
		return
	}

	page, pageSize := int32(1), int32(20)
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		pageSize = min(*params.PageSize, maxPersonalDataAccessPageSize)
	}

	rows, err := h.store.ListPersonalDataAccess(c, repository.ListPersonalDataAccessParams{
		TenantID:      pgtype.Text{String: tenantID, Valid: tenantID != ""},
		SubjectUserID: util.ToNullableText(params.SubjectUserId),
		ActorUserID:   util.ToNullableText(params.ActorUserId),
		AccessedFrom:  util.ToNullableTimestamptz(params.From),
		AccessedTo:    util.ToNullableTimestamptz(params.To),
		RowLimit:      pageSize,
		RowOffset:     (page - 1) * pageSize,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to list personal-data access")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	out := make([]core.PersonalDataAccess, len(rows))
	for i, r := range rows {
		tenant := r.TenantID
		out[i] = core.PersonalDataAccess{
			TenantId:      &tenant,
			ActorUserId:   r.ActorUserID,
			SubjectUserId: r.SubjectUserID,
			Resource:      r.Resource,
			AccessedAt:    r.AccessedAt,
		}
		if r.ResourceID.Valid {
			id := r.ResourceID.String
			out[i].ResourceId = &id
		}
	}
	c.JSON(http.StatusOK, out)
}
