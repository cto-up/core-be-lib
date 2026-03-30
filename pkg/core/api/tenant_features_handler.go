package core

import (
	"fmt"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
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
	isAllowed, err := auth.IsAllowedToManageTenantByID(ctx, s.store, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		ctx.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
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

	isAllowed, err := auth.IsAllowedToManageTenantByID(ctx, s.store, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		ctx.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
		return
	}

	_, err = s.store.UpdateTenantFeatures(ctx, repository.UpdateTenantFeaturesParams{
		ID:       id,
		Features: req,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (s *TenantHandler) GetTenantFeatureLicenses(ctx *gin.Context, id uuid.UUID) {
	tenant, err := s.store.GetTenantByID(ctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	isAllowed, err := auth.IsAllowedToManageTenantByID(ctx, s.store, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		ctx.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
		return
	}
	ctx.JSON(http.StatusOK, tenant.FeatureLicenses)
}

func (s *TenantHandler) UpdateTenantFeatureLicenses(ctx *gin.Context, id uuid.UUID) {
	var req subentity.TenantFeatureLicenses
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	isAllowed, err := auth.IsAllowedToManageTenantByID(ctx, s.store, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		ctx.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
		return
	}

	tenant, err := s.store.GetTenantByID(ctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	for featureName := range req {
		if !tenant.Features[featureName] {
			ctx.JSON(http.StatusBadRequest, helpers.ErrorResponse(
				fmt.Errorf("feature %q is not enabled for this tenant", featureName),
			))
			return
		}
	}

	_, err = s.store.UpdateTenantFeatureLicenses(ctx, repository.UpdateTenantFeatureLicensesParams{
		ID:              id,
		FeatureLicenses: req,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.Status(http.StatusNoContent)
}
