package core

import (
	"errors"
	"net/http"

	api "ctoup.com/coreapp/api/openapi/core"

	"slices"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
)

type SuperAdminHandler struct {
	authClientPool *service.FirebaseTenantClientConnectionPool
}

func NewSuperAdminHandler(authClientPool *service.FirebaseTenantClientConnectionPool) *SuperAdminHandler {
	return &SuperAdminHandler{
		authClientPool: authClientPool,
	}
}

// AddAuthorizedDomains handles the request to add authorized domains for Firebase Authentication
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

	// Call the AuthorizeDomains function
	err := service.SDKAddAuthorizedDomains(c, exh.authClientPool.GetClient(), req.Domains)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Return success response
	c.Status(http.StatusOK)
}

// RemoveAuthorizedDomains handles the request to add authorized domains for Firebase Authentication
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

	// Call the AuthorizeDomains function
	err := service.SDKRemoveAuthorizedDomains(c, exh.authClientPool.GetClient(), req.Domains)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Return success response
	c.Status(http.StatusOK)
}
