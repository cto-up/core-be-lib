package core

import (
	"errors"
	"net/http"

	api "ctoup.com/coreapp/api/openapi/core"

	"slices"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

/* TO REMOVE after Firebase migration !!!*/
type SuperAdminHandler struct {
	authProvider auth.AuthProvider
}

func NewSuperAdminHandler(authProvider auth.AuthProvider) *SuperAdminHandler {
	return &SuperAdminHandler{
		authProvider: authProvider,
	}
}

// AddAuthorizedDomains handles the request to add authorized domains for Firebase Authentication
// Firebase specific
func (exh *SuperAdminHandler) AddAuthorizedDomains(c *gin.Context) {
	// Check if user has SUPER_ADMIN role
	if !service.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("requires SUPER_ADMIN role")))
		return
	}

	// Parse request body
	var req api.AddAuthorizedDomainsJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Validate domains
	if slices.Contains(req.Domains, "") {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("domain cannot be empty")))
		return
	}

	err := service.SDKAddAuthorizedDomains(c, req.Domains)
	if err != nil {
		log.Error().Err(err).Msg("Error adding authorized domains")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Return success response
	c.Status(http.StatusOK)
}

// RemoveAuthorizedDomains handles the request to add authorized domains for Firebase Authentication
// Firebase specific
func (exh *SuperAdminHandler) RemoveAuthorizedDomains(c *gin.Context) {
	// Check if user has SUPER_ADMIN role
	if !service.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("requires SUPER_ADMIN role")))
		return
	}

	// Parse request body
	var req api.RemoveAuthorizedDomainsJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Validate domains
	if slices.Contains(req.Domains, "") {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("domain cannot be empty")))
		return
	}

	err := service.SDKRemoveAuthorizedDomains(c, req.Domains)
	if err != nil {
		log.Error().Err(err).Msg("Error removing authorized domains")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Return success response
	c.Status(http.StatusOK)
}
