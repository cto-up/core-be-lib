package core

import (
	"encoding/csv"
	"io"
	"strings"
	"time"

	"errors"
	"fmt"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	api "ctoup.com/coreapp/api/openapi/core"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	auth "ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/event"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserAdminHandler struct {
	// The thirteen operations themselves live on userOps, written once and
	// parameterised by TenantScope — see user_scope.go. What stays here is the
	// generated surface: one line per route, naming the scope.
	userOps
}

func NewUserAdminHandler(store *db.Store, authProvider auth.AuthProvider) *UserAdminHandler {

	factory := access.NewUserServiceStrategyFactory()
	userService := factory.CreateUserServiceStrategy(store, authProvider)

	// Try to initialize user event callback if available
	// This allows the realtime module to set up the callback for user creation events
	if initFunc := access.GetUserEventInitFunc(); initFunc != nil {
		initFunc(userService)
	}

	return &UserAdminHandler{userOps{
		store:        store,
		authProvider: authProvider,
		userService:  userService,
	}}
}

// AddUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) AddUser(c *gin.Context) {
	uh.addUser(c, uh.SessionTenant)
}

// (PUT /api/v1/users/{userid})
func (uh *UserAdminHandler) UpdateUser(c *gin.Context, userid string) {
	uh.updateUser(c, uh.SessionTenant, userid)
}

// DeleteUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) DeleteUser(c *gin.Context, userid string) {
	uh.deleteUser(c, uh.SessionTenant, userid)
}

// RemoveUserFromTenant removes a user from the current tenant (deletes membership only)
// (DELETE /api/v1/users/{userid}/remove-from-tenant)
func (uh *UserAdminHandler) RemoveUserFromTenant(c *gin.Context, userid string) {
	uh.removeUserFromTenant(c, uh.SessionTenant, userid)
}

// GetUserByID implements openapi.ServerInterface.
func (uh *UserAdminHandler) GetUserByID(c *gin.Context, id string) {
	uh.getUserByID(c, uh.SessionTenant, id)
}

// GetUsers implements openapi.ServerInterface.
func (uh *UserAdminHandler) ListUsers(c *gin.Context, params core.ListUsersParams) {
	uh.listUsers(c, uh.SessionTenant, listParams{
		Page:     params.Page,
		PageSize: params.PageSize,
		SortBy:   params.SortBy,
		Order:    (*string)(params.Order),
		Q:        params.Q,
		Scope:    params.Scope,
		Detail:   params.Detail,
	})
}

// AssignRole implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) AssignRole(c *gin.Context, userID string, role core.Role) {
	uh.assignRole(c, uh.SessionTenant, userID, role)
}

// UnassignRole implements openapi.ServerInterface.
func (uh *UserAdminHandler) UnassignRole(c *gin.Context, userID string, role core.Role) {
	uh.unassignRole(c, uh.SessionTenant, userID, role)
}

// UpdateUserStatus implements openapi.ServerInterface.
func (uh *UserAdminHandler) UpdateUserStatus(c *gin.Context, userID string) {
	uh.updateUserStatus(c, uh.SessionTenant, userID)
}

// ReactivateUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) ReactivateUser(c *gin.Context, userID string) {
	uh.reactivateUser(c, uh.SessionTenant, userID)
}

func (uh *UserAdminHandler) ResetPasswordRequestByAdmin(c *gin.Context, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		logger.Error().Msg("TenantID not found")
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// check if authorized user is admin
	if !auth.HasAdminPrivileges(c) {
		logger.Error().Msg("Only RESELLER, admin or super admin can reset password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only admin or super admin can reset password"})
		return
	}

	user, err := uh.userService.GetUserByTenantIDByID(c, tenantID.(string), userID)
	if err != nil {
		logger.Err(err).Msg("Failed to get user by ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if user.Email != req.Email {
		logger.Error().Msg("Email does not match user ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}

	url, err := getResetPasswordURL(c)
	if err != nil {
		logger.Err(err).Msg("Failed to get reset password URL")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		logger.Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get auth client"})
		return
	}

	err = resetPasswordRequest(c, baseAuthClient, url, req.Email)
	if err != nil {
		logger.Err(err).Msg("Failed to send password reset email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

// CheckUserExists checks if a user exists globally by email
func (uh *UserAdminHandler) CheckUserExists(c *gin.Context, params core.CheckUserExistsParams) {
	uh.checkUserExists(c, uh.SessionTenant, string(params.Email))
}

// AddUserMembership adds an existing user to the current tenant
func (uh *UserAdminHandler) AddUserMembership(c *gin.Context, userid string) {
	var req core.AddUserMembershipJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger := util.GetLoggerFromCtx(c.Request.Context())
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	uh.addUserMembership(c, uh.SessionTenant, userid, req.Roles)
}

func (uh *UserAdminHandler) ImportUsersFromAdmin(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		logger.Error().Msg("TenantID not found")
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// Get auth client for tenant
	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		logger.Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		logger.Err(err).Msg("Failed to get uploaded file")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("file upload error: %v", err)))
		return
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		logger.Err(err).Msg("Failed to open uploaded file")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("error opening file: %v", err)))
		return
	}
	defer src.Close()

	// Parse CSV
	reader := csv.NewReader(src)
	reader.Comma = ';' // Set semicolon as delimiter

	// Read header
	header, err := reader.Read()
	if err != nil {
		logger.Err(err).Msg("Failed to read CSV header")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("error reading CSV header: %v", err)))
		return
	}

	// Strip BOM from the first header column if present
	if len(header) > 0 {
		header[0] = util.StripBOM(header[0])
	}

	// Validate header
	requiredColumns := []string{"lastname", "firstname", "email", "is_customer_admin"}
	missingColumns := []string{}

	// Create a map of header columns for easy lookup
	headerMap := make(map[string]int)
	for i, col := range header {
		headerMap[strings.ToLower(col)] = i
	}

	// Check for missing required columns
	for _, required := range requiredColumns {
		if _, exists := headerMap[required]; !exists {
			missingColumns = append(missingColumns, required)
		}
	}

	if len(missingColumns) > 0 {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("invalid CSV format. Missing required columns: %v", missingColumns)))
		return
	}

	// Process records
	type ImportError struct {
		Line  int    `json:"line"`
		Email string `json:"email"`
		Error string `json:"error"`
	}

	var (
		total         int
		success       int
		alreadyExists int
		failed        int
		errors        []ImportError
	)

	// Handle streaming case
	clientChan := make(chan event.ProgressEvent)
	errorChan := make(chan error, 1)

	// Set headers for SSE before any data is written
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Process each line
	lineNum := 1 // Start from 1 to account for header
	// get total number of lines
	// Start generation in goroutine
	go func() {
		defer close(clientChan)

		for {
			lineNum++
			message := fmt.Sprintf(`Processing 
			line: %d,
			success: %d,
			already exists: %d,
			failed: %d,
			errors: %v`, lineNum, success, alreadyExists, failed, errors)
			clientChan <- event.NewProgressEvent("INFO", message, 50)

			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Error: fmt.Sprintf("error reading line: %v", err),
				})
				failed++
				continue
			}

			total++

			// Extract user data
			if len(record) < 4 {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Error: fmt.Sprintf("invalid record format, expected at least 4 fields, got %d", len(record)),
				})
				failed++
				continue
			}

			lastname := record[headerMap["lastname"]]
			firstname := record[headerMap["firstname"]]
			email := record[headerMap["email"]]
			isCustomerAdmin := parseBoolFlag(record[headerMap["is_customer_admin"]])

			silent := false
			if idx, ok := headerMap["silent"]; ok && idx < len(record) {
				silent = parseBoolFlag(record[idx])
			}

			var req core.AddUserJSONRequestBody
			req.Email = email
			req.Name = firstname + " " + lastname
			req.Roles = []core.Role{}
			if silent {
				silentTrue := true
				req.Silent = &silentTrue
			}
			MarkSilent(c, silent)

			// check if user has rights to assign roles
			if !auth.HasAdminPrivileges(c) {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: "must be an RESELLER, CUSTOMER_ADMIN or SUPER_ADMIN to assign CUSTOMER_ADMIN role to a user.",
				})
				failed++
				continue
			}
			if isCustomerAdmin {
				req.Roles = []core.Role{api.CUSTOMERADMIN}
			}
			_, err = uh.userService.CreateUser(c, baseAuthClient, tenantID.(string), req, nil)
			if err != nil {
				logger.Err(err).Msg("Failed to create user")
				// check if error is a auth provider error and if so, check if it is a duplicate email error
				if auth.IsEmailAlreadyExists(err) {
					errors = append(errors, ImportError{
						Line:  lineNum,
						Email: email,
						Error: "email already exists",
					})
					alreadyExists++
					continue
				} else {
					errors = append(errors, ImportError{
						Line:  lineNum,
						Email: email,
						Error: fmt.Sprintf("error creating user: %v", err),
					})
					failed++
					continue
				}
			}

			if !silent {
				url, err := getWelcomeEmailURL(c)
				if err != nil {
					errors = append(errors, ImportError{
						Line:  lineNum,
						Email: email,
						Error: fmt.Sprintf("error getting welcome email url: %v", err),
					})
					failed++
					continue
				}
				err = sendWelcomeEmail(c, baseAuthClient, url, req.Email)
				if err != nil {
					errors = append(errors, ImportError{
						Line:  lineNum,
						Email: email,
						Error: fmt.Sprintf("error sending welcome email: %v", err),
					})
					failed++
					continue
				}
			}

			success++
		}

		// Return results
		result := fmt.Sprintf(`Finished processing Users. Results:
			total: %d,
			success: %d,
			already exists: %d,
			failed: %d,
			errors: %v`,
			total, success, alreadyExists, failed, errors)

		clientChan <- event.NewProgressEvent("INFO", result, 100)
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-clientChan:
			if !ok {
				return false
			}
			c.SSEvent("message", msg)
			return msg.EventType != "ERROR" && msg.Progress != 100
		case err := <-errorChan:
			// Send error as SSE event instead of trying to change status code
			logger.Printf("Error in streaming: %v", err)
			errEvent := event.NewProgressEvent("ERROR", err.Error(), 100)
			c.SSEvent("message", errEvent)
			return false
		case <-time.After(60 * time.Second):
			// Send timeout as SSE event
			timeoutEvent := event.NewProgressEvent("ERROR", "Generation timeout", 100)
			c.SSEvent("message", timeoutEvent)
			return false
		}
	})
	// Commit transaction if there were successful imports
}
