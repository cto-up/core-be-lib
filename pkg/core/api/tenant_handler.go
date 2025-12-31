package core

import (
	"fmt"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	api "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	fileservice "ctoup.com/coreapp/pkg/shared/fileservice"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"ctoup.com/coreapp/pkg/shared/service"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type TenantHandler struct {
	authProvider       auth.AuthProvider
	multiTenantService *service.MultitenantService
	FileService        *fileservice.FileService
	store              *db.Store
}

// (GET /public-api/v1/tenant)
func (exh *TenantHandler) GetPublicTenant(c *gin.Context) {
	subdomain, err := utils.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if subdomain == "" || subdomain == "www" {
		c.JSON(http.StatusOK, repository.CoreTenant{
			Subdomain: "www",
			Name:      "Administration",
			Features:  subentity.TenantFeatures{},
			Profile: subentity.TenantProfile{
				DisplayName: "Administration",
				LightColors: core.ColorSchema{},
				DarkColors:  core.ColorSchema{},
			},
		})
		return
	}

	tenant, err := exh.store.GetTenantBySubdomain(c, subdomain)
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// write the tenant id to the response
	c.JSON(http.StatusOK, repository.CoreTenant{
		Subdomain:   tenant.Subdomain,
		Name:        tenant.Name,
		TenantID:    tenant.TenantID,
		Features:    tenant.Features,
		Profile:     tenant.Profile,
		AllowSignUp: tenant.AllowSignUp,
		//AllowPasswordSignUp:   tenant.AllowPasswordSignUp,
		//EnableEmailLinkSignIn: tenant.EnableEmailLinkSignIn,
	})
}

// AddTenant implements api.ServerInterface.
func (exh *TenantHandler) AddTenant(c *gin.Context) {
	var req api.AddTenantJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind request body")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Get the tenant manager
	tenantManager := exh.authProvider.GetTenantManager()
	if tenantManager == nil {
		log.Error().Msg("Tenant manager not supported by this provider")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("tenant operations not supported")))
		return
	}

	tenantConfig := &auth.TenantConfig{
		DisplayName:           req.Name,
		EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   req.AllowPasswordSignUp,
	}

	newTenant, err := tenantManager.CreateTenant(c, tenantConfig)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	userID, exist := c.Get(auth.AUTH_USER_ID)
	if !exist {
		// should not happen as the middleware ensures that the user is authenticated
		log.Error().Msg("User not authenticated")
		c.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}
	tenant, err := exh.store.CreateTenant(c,
		repository.CreateTenantParams{
			UserID:                userID.(string),
			Name:                  req.Name,
			TenantID:              newTenant.ID,
			Subdomain:             req.Subdomain,
			EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
			AllowPasswordSignUp:   req.AllowPasswordSignUp,
			AllowSignUp:           req.AllowSignUp,
		})
	if err != nil {
		tenantManager.DeleteTenant(c, newTenant.ID)
		log.Error().Err(err).Msg("Failed to create tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	profile := subentity.TenantProfile{
		DisplayName: req.Name,
		LightColors: core.ColorSchema{},
		DarkColors:  core.ColorSchema{},
	}

	_, err = exh.store.UpdateTenantProfile(c, repository.UpdateTenantProfileParams{
		TenantID: newTenant.ID,
		Profile:  profile,
	})

	if err != nil {
		log.Error().Err(err).Str("tenantID", newTenant.ID).Msg("Failed to set tenant profile on create")
	}
	c.JSON(http.StatusCreated, tenant)
}

// UpdateTenant implements api.ServerInterface.
func (exh *TenantHandler) UpdateTenant(c *gin.Context, id uuid.UUID) {
	var req api.UpdateTenantJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Get the tenant manager
	tenantManager := exh.authProvider.GetTenantManager()
	if tenantManager == nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("tenant operations not supported")))
		return
	}

	tenantConfig := &auth.TenantConfig{
		DisplayName:           req.Name,
		EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   req.AllowPasswordSignUp,
	}

	_, err := tenantManager.UpdateTenant(c, req.TenantId, tenantConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	_, err = exh.store.UpdateTenant(c,
		repository.UpdateTenantParams{
			ID:                    id,
			Name:                  req.Name,
			Subdomain:             req.Subdomain,
			EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
			AllowPasswordSignUp:   req.AllowPasswordSignUp,
			AllowSignUp:           req.AllowSignUp,
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteTenant implements api.ServerInterface.
func (exh *TenantHandler) DeleteTenant(c *gin.Context, id uuid.UUID) {
	tenant, err := exh.store.GetTenantByID(c, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Get the tenant manager
	tenantManager := exh.authProvider.GetTenantManager()
	if tenantManager == nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("tenant operations not supported")))
		return
	}

	err = tenantManager.DeleteTenant(c, tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	_, err = exh.store.DeleteTenant(c, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindTenantByID implements api.ServerInterface.
func (exh *TenantHandler) GetTenantByID(c *gin.Context, id uuid.UUID) {

	tenant, err := exh.store.GetTenantByID(c, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, tenant)
}

// ListTenants implements api.ServerInterface.
func (exh *TenantHandler) ListTenants(c *gin.Context, params api.ListTenantsParams) {
	pagingRequest := helpers.PagingRequest{
		MaxPageSize:     50,
		DefaultPage:     1,
		DefaultPageSize: 10,
		DefaultSortBy:   "name",
		DefaultOrder:    "asc",
		Page:            params.Page,
		PageSize:        params.PageSize,
		SortBy:          params.SortBy,
		Order:           (*string)(params.Order),
	}

	pagingSql := helpers.GetPagingSQL(pagingRequest)

	like := pgtype.Text{
		Valid: false,
	}

	if params.Q != nil {
		like.String = *params.Q + "%"
		like.Valid = true
	}

	query := repository.ListTenantsParams{
		Limit:  pagingSql.PageSize,
		Offset: pagingSql.Offset,
		Like:   like,
		SortBy: pagingSql.SortBy,
		Order:  pagingSql.Order,
	}

	tenants, err := exh.store.ListTenants(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, tenants)
}

func NewTenantHandler(store *db.Store, authProvider auth.AuthProvider, multiTenantService *service.MultitenantService) *TenantHandler {
	fileService := fileservice.NewFileService()
	return &TenantHandler{
		store:              store,
		authProvider:       authProvider,
		FileService:        fileService,
		multiTenantService: multiTenantService,
	}
}
