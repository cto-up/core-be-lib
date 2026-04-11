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
	"ctoup.com/coreapp/pkg/shared/util"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	logger := util.GetLoggerFromCtx(c.Request.Context())
	subdomain, err := utils.GetSubdomain(c)
	if err != nil {
		logger.Err(err).Msg("Error getting subdomain")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if utils.IsAdminSubdomain(subdomain) {
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
		logger.Err(err).Str("subdomain", subdomain).Msg("Error getting tenant by subdomain")
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
		IsReseller:  tenant.IsReseller,
		ResellerID:  tenant.ResellerID,
	})
}

// AddTenant implements api.ServerInterface.
func (exh *TenantHandler) AddTenant(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	var req api.AddTenantJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind request body")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Get the tenant manager
	tenantManager := exh.authProvider.GetTenantManager()
	if tenantManager == nil {
		logger.Error().Msg("Tenant manager not supported by this provider")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("tenant operations not supported")))
		return
	}

	tenantConfig := &auth.TenantConfig{
		DisplayName:           req.Name,
		Subdomain:             req.Subdomain,
		EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   req.AllowPasswordSignUp,
	}

	newTenant, err := tenantManager.CreateTenant(c, tenantConfig)
	if err != nil {
		logger.Err(err).Msg("Failed to create tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	userID, exist := c.Get(auth.AUTH_USER_ID)
	if !exist {
		// should not happen as the middleware ensures that the user is authenticated
		logger.Error().Msg("User not authenticated")
		c.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}

	// If current user is a TENANT_IS_RESELLER of a reseller, set reseller_id
	var resellerID pgtype.Text
	claims, exists := c.Get(auth.AUTH_CLAIMS)
	if exists {
		claimsMap := claims.(map[string]interface{})
		if claimsMap[auth.TENANT_IS_RESELLER] == true {
			authTenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
			if authTenantID != "" {
				resellerID = pgtype.Text{String: authTenantID, Valid: true}
			}
		}
	}

	// Override with value from request if provided (e.g. by SUPER_ADMIN)
	if req.ResellerId != nil {
		resellerID = pgtype.Text{String: *req.ResellerId, Valid: true}
	}

	var isReseller bool
	if req.IsReseller != nil && auth.IsSuperAdmin(c) {
		isReseller = *req.IsReseller
	}

	// SUPER_ADMIN, ADMIN, or a reseller can set contract_end_date and is_disabled at creation time.
	// For resellers the new tenant's reseller_id will be their own tenant_id, so they always qualify.
	var contractEndDate pgtype.Timestamptz
	var isDisabled bool
	canUpdateContract := auth.IsSuperAdmin(c) || auth.IsAdmin(c)
	if !canUpdateContract {
		authTenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
		isCallerReseller, _ := exh.multiTenantService.IsReseller(c, authTenantID)
		if isCallerReseller && resellerID.Valid && resellerID.String == authTenantID {
			canUpdateContract = true
		}
	}
	if canUpdateContract {
		if req.ContractEndDate != nil {
			contractEndDate = pgtype.Timestamptz{Time: *req.ContractEndDate, Valid: true}
		}
		if req.IsDisabled != nil {
			isDisabled = *req.IsDisabled
		}
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
			ResellerID:            resellerID,
			IsReseller:            isReseller,
			ContractEndDate:       contractEndDate,
			IsDisabled:            isDisabled,
		})
	if err != nil {
		logger.Err(err).Msg("Failed to create tenant")
		err := tenantManager.DeleteTenant(c, newTenant.ID)
		if err != nil {
			logger.Err(err).Msg("Failed to rollback tenant creation in auth provider")
		}
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
		logger.Err(err).Str("tenantID", newTenant.ID).Msg("Failed to set tenant profile on create")
	}
	c.JSON(http.StatusCreated, tenant)
}

// UpdateTenant implements api.ServerInterface.
func (exh *TenantHandler) UpdateTenant(c *gin.Context, id uuid.UUID) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	var req api.UpdateTenantJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to parse request body")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Get the tenant manager
	tenantManager := exh.authProvider.GetTenantManager()
	if tenantManager == nil {
		logger.Error().Msg("Tenant manager not supported by this provider")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("tenant operations not supported")))
		return
	}

	tenantConfig := &auth.TenantConfig{
		DisplayName:           req.Name,
		EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   req.AllowPasswordSignUp,
	}

	// Authorization check
	isAllowed, err := auth.IsAllowedToManageTenantByID(c, exh.store, id)
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant management permissions")
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
		return
	}
	_, err = tenantManager.UpdateTenant(c, req.TenantId, tenantConfig)
	if err != nil {
		logger.Err(err).Msg("Failed to update tenant in auth provider")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Fetch existing tenant once for field-level permission checks and fallback values
	existing, err := exh.store.GetTenantByID(c, id)
	if err != nil {
		logger.Err(err).Msg("Failed to get existing tenant for update")
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}

	updateParams := repository.UpdateTenantParams{
		ID:                    id,
		Name:                  req.Name,
		Subdomain:             req.Subdomain,
		EnableEmailLinkSignIn: req.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   req.AllowPasswordSignUp,
		AllowSignUp:           req.AllowSignUp,
		// Preserve existing values by default; overridden below based on role
		IsReseller:      existing.IsReseller,
		ContractEndDate: existing.ContractEndDate,
		IsDisabled:      existing.IsDisabled,
	}

	// Only SUPER_ADMIN can change is_reseller
	if auth.IsSuperAdmin(c) && req.IsReseller != nil {
		updateParams.IsReseller = *req.IsReseller
	}

	// SUPER_ADMIN, ADMIN, or a reseller managing this specific tenant can update
	// contract_end_date and is_disabled
	canUpdateContract := auth.IsSuperAdmin(c) || auth.IsAdmin(c)
	if !canUpdateContract {
		authTenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
		isReseller, _ := exh.multiTenantService.IsReseller(c, authTenantID)
		if isReseller && existing.ResellerID.Valid && existing.ResellerID.String == authTenantID {
			canUpdateContract = true
		}
	}

	if canUpdateContract {
		if req.ContractEndDate != nil {
			updateParams.ContractEndDate = pgtype.Timestamptz{Time: *req.ContractEndDate, Valid: true}
		} else {
			updateParams.ContractEndDate = pgtype.Timestamptz{}
		}
		if req.IsDisabled != nil {
			updateParams.IsDisabled = *req.IsDisabled
		}
	}

	_, err = exh.store.UpdateTenant(c, updateParams)
	if err != nil {
		logger.Err(err).Msg("Failed to update tenant in database")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteTenant implements api.ServerInterface.
func (exh *TenantHandler) DeleteTenant(c *gin.Context, id uuid.UUID) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := exh.store.GetTenantByID(c, id)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Authorization check
	isAllowed, err := auth.IsAllowedToManageTenantByID(c, exh.store, id)
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant management permissions")
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
		return
	}

	// Get the tenant manager
	tenantManager := exh.authProvider.GetTenantManager()
	if tenantManager == nil {
		logger.Error().Msg("Tenant manager not supported by this provider")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("tenant operations not supported")))
		return
	}

	err = tenantManager.DeleteTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to delete tenant in auth provider")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	_, err = exh.store.DeleteTenant(c, id)
	if err != nil {
		logger.Err(err).Msg("Failed to delete tenant in database")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindTenantByID implements api.ServerInterface.
func (exh *TenantHandler) GetTenantByID(c *gin.Context, id uuid.UUID) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenant, err := exh.store.GetTenantByID(c, id)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant by ID")
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	// Authorization check
	isAllowed, err := auth.IsAllowedToManageTenantByID(c, exh.store, id)
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant management permissions")
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}
	if !isAllowed {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, "Not allowed to manage this tenant")
		return
	}
	c.JSON(http.StatusOK, tenant)
}

// ListTenants implements api.ServerInterface.
func (exh *TenantHandler) ListTenants(c *gin.Context, params api.ListTenantsParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
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

	if params.ResellerId != nil {
		query.ResellerID = pgtype.Text{String: *params.ResellerId, Valid: true}
	}

	// If user is TENANT_IS_RESELLER of a reseller, force reseller_id filter
	claims, exists := c.Get(auth.AUTH_CLAIMS)
	if exists {
		claimsMap := claims.(map[string]interface{})
		if claimsMap[auth.TENANT_IS_RESELLER] == true {
			authTenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
			isReseller, _ := exh.multiTenantService.IsReseller(c, authTenantID)
			if isReseller && authTenantID != "" {
				query.ResellerID = pgtype.Text{String: authTenantID, Valid: true}
			}
		}
	}

	tenants, err := exh.store.ListTenants(c, query)
	if err != nil {
		logger.Err(err).Msg("Failed to list tenants")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, tenants)
}

// (GET /api/v1/reseller/tenants)
func (exh *TenantHandler) ListResellerTenants(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	// Caller must be a CUSTOMER_ADMIN of a reseller tenant
	if !auth.IsActingReseller(c) && !auth.IsReseller(c) {
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(fmt.Errorf("forbidden: must be a CUSTOMER_ADMIN of a reseller tenant")))
		return
	}

	userID := c.GetString(auth.AUTH_USER_ID)

	tenants, err := exh.store.ListResellerTenants(c, userID)
	if err != nil {
		logger.Err(err).Msg("Failed to list reseller tenants")
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
