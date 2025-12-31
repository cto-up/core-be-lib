package core

import (
	"errors"
	"net/http"

	api "ctoup.com/coreapp/api/openapi/core"

	"slices"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/service"
	firebaseauth "firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
)

type SuperAdminHandler struct {
	authClientPool *auth.AuthProviderAdapter
}

func NewSuperAdminHandler(authClientPool *auth.AuthProviderAdapter) *SuperAdminHandler {
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
	client := exh.authClientPool.GetClient()
	firebaseClient, ok := client.(*firebaseauth.Client)
	if !ok {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(errors.New("requires Firebase auth provider")))
		return
	}

	err := service.SDKAddAuthorizedDomains(c, firebaseClient, req.Domains)
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
	client := exh.authClientPool.GetClient()
	firebaseClient, ok := client.(*firebaseauth.Client)
	if !ok {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(errors.New("requires Firebase auth provider")))
		return
	}

	err := service.SDKRemoveAuthorizedDomains(c, firebaseClient, req.Domains)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Return success response
	c.Status(http.StatusOK)
}
