package core

import (
	"encoding/csv"
	"io"
	"strings"
	"time"

	"errors"
	"fmt"
	"net/http"

	"firebase.google.com/go/auth"
	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/event"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserAdminHandler struct {
	store          *db.Store
	authClientPool *access.FirebaseTenantClientConnectionPool
	userService    *access.UserService
}

func NewUserAdminHandler(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *UserAdminHandler {
	userService := access.NewUserService(store, authClientPool)
	handler := &UserAdminHandler{store: store,
		authClientPool: authClientPool,
		userService:    userService}
	return handler
}

// AddUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) AddUser(c *gin.Context) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.AddUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.AddUser(c, baseAuthClient, tenantID.(string), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	url, err := getResetPasswordURL(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = sendWelcomeEmail(c, baseAuthClient, url, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, user)
}

// (PUT /api/v1/users/{userid})
func (uh *UserAdminHandler) UpdateUser(c *gin.Context, userid string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.UpdateUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUser(c, baseAuthClient, tenantID.(string), userid, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) DeleteUser(c *gin.Context, userid string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.DeleteUser(c, baseAuthClient, tenantID.(string), userid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindUserByID implements openapi.ServerInterface.
func (uh *UserAdminHandler) GetUserByID(c *gin.Context, id string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	user, err := uh.userService.GetUserByID(c, baseAuthClient, tenantID.(string), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, user)
}

// GetUsers implements openapi.ServerInterface.
func (u *UserAdminHandler) ListUsers(c *gin.Context, params core.ListUsersParams) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	pagingRequest := helpers.PagingRequest{
		MaxPageSize:     50,
		DefaultPage:     1,
		DefaultPageSize: 10,
		DefaultSortBy:   "email",
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

	users, err := u.userService.ListUsers(c, tenantID.(string), pagingSql, like)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if params.Detail != nil && *params.Detail == "basic" {
		basicEntities := make([]subentity.BasicEntity, 0)
		for _, user := range users {
			basicEntity := subentity.BasicEntity{
				ID:   user.Id,
				Name: user.Profile.Name,
			}
			basicEntities = append(basicEntities, basicEntity)
		}
		c.JSON(http.StatusOK, basicEntities)
	} else {
		c.JSON(http.StatusOK, users)
	}
}

// AssignRole implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) AssignRole(c *gin.Context, userID string, roleID uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.AssignRole(c, baseAuthClient, tenantID.(string), userID, roleID)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnassignRole implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) UnassignRole(c *gin.Context, userID string, roleID uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.UnassignRole(c, baseAuthClient, tenantID.(string), userID, roleID)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateUserStatus implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) UpdateUserStatus(c *gin.Context, userID string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var req core.UpdateUserStatusJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUserStatus(c, baseAuthClient, tenantID.(string), userID, (string)(req.Name), req.Value)

	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (uh *UserAdminHandler) ResetPasswordRequest(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Firebase client"})
		return
	}
	url, err := getResetPasswordURL(c, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	err = resetPasswordRequest(c, baseAuthClient, url, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

func (uh *UserAdminHandler) ResetPasswordRequestByAdmin(c *gin.Context, userID string) {

	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// check if authorized user is admin
	if !access.IsAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only admin can reset password"})
		return
	}

	user, err := uh.store.GetUserByID(c, repository.GetUserByIDParams{
		ID:       userID,
		TenantID: c.GetString(access.AUTH_TENANT_ID_KEY),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if user.Email.String != req.Email {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}

	url, err := getResetPasswordURL(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Firebase client"})
		return
	}
	err = resetPasswordRequest(c, baseAuthClient, url, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

// ImportUsersFromAdmin implements the CSV import functionality
func (uh *UserAdminHandler) ImportUsersFromAdmin(c *gin.Context) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// Get Firebase auth client for tenant
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("file upload error: %v", err)))
		return
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
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
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("error reading CSV header: %v", err)))
		return
	}

	// Validate header
	requiredColumns := []string{"lastname", "firstname", "email", "roles"}
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

	// Fetch all available roles for validation
	roles, err := uh.store.Queries.ListRoles(c, repository.ListRolesParams{
		Limit:  100, // Assuming we won't have more than 100 roles
		Offset: 0,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("error fetching roles: %v", err)))
		return
	}

	// Create a map of role names to IDs for quick lookup
	roleMap := make(map[string]uuid.UUID)
	for _, role := range roles {
		roleMap[role.Name] = role.ID
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
					Error: fmt.Sprintf("invalid record format, expected 4 fields, got %d", len(record)),
				})
				failed++
				continue
			}

			lastname := record[0]
			firstname := record[1]
			email := record[2]
			roleNames := strings.Split(record[3], ",")

			var req core.AddUserJSONRequestBody
			req.Email = email
			req.Name = firstname + " " + lastname

			user, err := uh.userService.AddUser(c, baseAuthClient, tenantID.(string), req)
			if err != nil {
				// check if error is a firebase error and if so, check if it is a duplicate email error
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

			// Assign roles
			roleAssignErrors := []string{}
			for _, roleName := range roleNames {
				roleName = strings.TrimSpace(roleName)
				if roleName == "" {
					continue
				}

				roleID, exists := roleMap[roleName]
				if !exists {
					roleAssignErrors = append(roleAssignErrors, fmt.Sprintf("role '%s' not found", roleName))
					continue
				}

				err = uh.userService.AssignRole(c, baseAuthClient, tenantID.(string), user.ID, roleID)
				if err != nil {
					roleAssignErrors = append(roleAssignErrors, fmt.Sprintf("error assigning role '%s': %v", roleName, err))
				}
			}
			url, err := getResetPasswordURL(c)
			if err != nil {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: fmt.Sprintf("error getting reset password url: %v", err),
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

			if len(roleAssignErrors) > 0 {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: fmt.Sprintf("user created but role assignment failed: %s", strings.Join(roleAssignErrors, "; ")),
				})
			}
			success++
		}

		// Return results
		result := fmt.Sprintf(`Finished processing CSV file. Results:
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
			log.Printf("Error in streaming: %v", err)
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
