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

const maxAdminPageSize = 100

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

	limit, offset := adminPage(params.Page, params.PageSize)
	rows, err := h.store.ListPersonalDataAccess(c, repository.ListPersonalDataAccessParams{
		TenantID:      pgtype.Text{String: tenantID, Valid: tenantID != ""},
		SubjectUserID: util.ToNullableText(params.SubjectUserId),
		ActorUserID:   util.ToNullableText(params.ActorUserId),
		AccessedFrom:  util.ToNullableTimestamptz(params.From),
		AccessedTo:    util.ToNullableTimestamptz(params.To),
		RowLimit:      limit,
		RowOffset:     offset,
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

// adminPage turns the admin lists' page/pageSize into a limit and offset:
// 20 per page by default, at most maxAdminPageSize.
func adminPage(page, pageSize *int32) (limit, offset int32) {
	p, size := int32(1), int32(20)
	if page != nil && *page > 0 {
		p = *page
	}
	if pageSize != nil && *pageSize > 0 {
		size = min(*pageSize, maxAdminPageSize)
	}
	return size, (p - 1) * size
}
