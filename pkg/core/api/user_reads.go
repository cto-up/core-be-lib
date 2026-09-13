package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

// The three read operations, written once. Each was a near-exact copy of the
// other with a different first twelve lines; see user_scope.go.

// listParams is the union of the two generated parameter types.
//
// They are generated from two OpenAPI operations, so they are two Go types with
// identical fields — plus Scope and Detail, which only the admin operation
// declares. Normalising here rather than making the shared implementation
// generic keeps the OpenAPI spec untouched, which is the point: no consumer's
// client regenerates.
type listParams struct {
	Page     *int32
	PageSize *int32
	SortBy   *string
	Order    *string
	Q        *string

	// Scope "all" lists every user system-wide. Admin surface only.
	Scope *core.ListUsersParamsScope
	// Detail "basic" returns id+name instead of the full user. Admin only.
	Detail *string
}

func (o *userOps) listUsers(c *gin.Context, scope TenantScope, params listParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	pagingSQL := helpers.GetPagingSQL(helpers.PagingRequest{
		MaxPageSize:     50,
		DefaultPage:     1,
		DefaultPageSize: 10,
		DefaultSortBy:   "email",
		DefaultOrder:    "asc",
		Page:            params.Page,
		PageSize:        params.PageSize,
		SortBy:          params.SortBy,
		Order:           params.Order,
	})

	like := pgtype.Text{Valid: false}
	if params.Q != nil {
		like.String = *params.Q + "%"
		like.Valid = true
	}

	var users []core.User
	var err error
	if params.Scope != nil && *params.Scope == core.All {
		// Listing every user system-wide exposes cross-tenant PII — restrict to
		// super admins (used by the admin domain to find a user to promote to a
		// global role).
		if !auth.IsSuperAdmin(c) {
			logger.Error().Msg("Only super admins may list all users")
			c.JSON(http.StatusForbidden, helpers.ErrorResponse(errOnlySuperAdminMayListAll))
			return
		}
		users, err = o.userService.ListAllUsers(c, pagingSQL, like)
	} else {
		users, err = o.userService.ListUsers(c, target.id, pagingSQL, like)
	}
	if err != nil {
		logger.Err(err).Msg("Failed to list users")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if params.Detail != nil && *params.Detail == "basic" {
		basic := make([]subentity.BasicEntity, 0, len(users))
		for _, user := range users {
			basic = append(basic, subentity.BasicEntity{ID: user.Id, Name: user.Profile.Name})
		}
		c.JSON(http.StatusOK, basic)
		return
	}

	o.userService.EnrichWithAuthActivity(c, users)
	c.JSON(http.StatusOK, users)
}

func (o *userOps) getUserByID(c *gin.Context, scope TenantScope, id string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	var user core.User
	var err error
	if target.id == "" {
		// The root domain: an admin signed in with no tenant. Only a
		// SUPER_ADMIN may look a user up without one, because the lookup is
		// then across every tenant. ManagedTenant never yields an empty id, so
		// this branch belongs to the session scope alone.
		if !auth.IsSuperAdmin(c) {
			logger.Error().Msg("Only SUPER_ADMIN can get user without tenant")
			c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errOnlySuperAdminWithoutTenant))
			return
		}
		user, err = o.userService.GetUserByID(c, id)
	} else {
		user, err = o.userService.GetUserByTenantIDByID(c, target.id, id)
	}
	if err != nil {
		logger.Err(err).Msg("Failed to get user by ID")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, user)
}

// checkUserExists answers "is there an account for this email, and is it
// already in this tenant?" — the lookup behind the add-user dialog.
//
// The two copies returned the same payload under two different key names for
// the membership flag: isMemberOfCurrentTenant on the admin path,
// isMemberOfTenant on the super-admin one. Nothing pinned either — the response
// is an untyped object in the spec — so each frontend learned a different word
// for one thing.
//
// Both keys are emitted. Dropping either would break a caller today; emitting
// both lets the two frontends converge on isMemberOfTenant and the other key
// then go, which is a change somebody can make deliberately rather than one
// this refactor makes for them.
func (o *userOps) checkUserExists(c *gin.Context, scope TenantScope, email string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	user, err := o.userService.GetUserByEmailGlobal(c, email)
	if err != nil {
		logger.Err(err).Msg("Failed to get user by email")
		c.JSON(http.StatusOK, gin.H{"exists": false})
		return
	}

	isMember, err := o.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   user.Id,
		TenantID: target.id,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	tenantCount, err := o.store.CountUserTenants(c, user.Id)
	if err != nil {
		logger.Err(err).Msg("Failed to count user tenants")
		tenantCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"exists": true,
		"user": gin.H{
			"id":                      user.Id,
			"name":                    user.Profile.Name,
			"email":                   user.Email,
			"tenantCount":             tenantCount,
			"isMemberOfTenant":        isMember,
			"isMemberOfCurrentTenant": isMember,
		},
	})
}
