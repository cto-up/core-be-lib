package core

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	utils "ctoup.com/coreapp/pkg/shared/util"
)

func (s *TenantHandler) GetTenantProfile(ctx *gin.Context) {
	subdomain, err := utils.GetSubdomain(ctx)

	// get tenant from context using subdomain
	tenantID, err := s.multiTenantService.GetTenantIDWithSubdomain(ctx, subdomain)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	tenant, err := s.store.GetTenantByTenantID(ctx, tenantID)
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			ctx.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, tenant.Profile)
}

func (s *TenantHandler) UpdateTenantProfile(ctx *gin.Context) {
	tenantID, exists := ctx.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		ctx.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var req subentity.TenantProfile
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	_, err := s.store.UpdateTenantProfile(ctx, repository.UpdateTenantProfileParams{
		TenantID: tenantID.(string),
		Profile:  req,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.Status(http.StatusNoContent)
}
