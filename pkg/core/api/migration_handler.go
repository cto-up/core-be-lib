package core

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	api "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// MigrationHandler handles migration-related API requests
type MigrationHandler struct {
	store *db.Store
}

// NewMigrationHandler creates a new migration handler
func NewMigrationHandler(store *db.Store) *MigrationHandler {
	return &MigrationHandler{
		store: store,
	}
}

// GetCoreMigration handles the request to get core migration information
func (h *MigrationHandler) GetCoreMigration(c *gin.Context) {
	// Check if user has SUPER_ADMIN role
	if !service.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("requires SUPER_ADMIN role")))
		return
	}

	migration, err := h.store.GetCoreMigration(c)
	if err != nil {
		log.Error().Err(err).Msg("Error getting core migration")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	apiMigration := api.Migration{
		Version: int(migration.Version),
		Dirty:   migration.Dirty,
	}

	c.JSON(http.StatusOK, apiMigration)
}

// UpdateCoreMigration handles the request to update core migration information
func (h *MigrationHandler) UpdateCoreMigration(c *gin.Context) {
	// Check if user has SUPER_ADMIN role
	if !service.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("requires SUPER_ADMIN role")))
		return
	}

	var req = api.UpdateCoreMigrationJSONBody{}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	// Get current migration
	currentMigration, err := h.store.GetCoreMigration(c)
	if err != nil {
		log.Error().Err(err).Msg("Error getting current core migration")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	err = h.store.UpdateCoreMigration(c, repository.UpdateCoreMigrationParams{
		Version: int64(req.Version),
		Dirty:   req.Dirty,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error updating core migration")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	logger := service.GetLoggerFromContext(c)
	// Log prvious and new version
	logger.Info().Int64("old_version", currentMigration.Version).Int("new_version", req.Version).Bool("dirty", req.Dirty).Msg("Core migration updated")

	c.Status(http.StatusNoContent)
}
