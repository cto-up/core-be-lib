package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *TenantHandler) GetTenantFeatures(ctx *gin.Context, id uuid.UUID) {
	tenant, err := s.store.GetTenantByID(ctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, tenant.Features)
}

func (s *TenantHandler) UpdateTenantFeatures(ctx *gin.Context, id uuid.UUID) {
	var req subentity.TenantFeatures
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	_, err := s.store.UpdateTenantFeatures(ctx, repository.UpdateTenantFeaturesParams{
		ID:       id,
		Features: req,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.Status(http.StatusNoContent)
}
