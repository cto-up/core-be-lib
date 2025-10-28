package core

import (
	"encoding/csv"
	"io"
	"slices"
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
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
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
func checkAuthorizedRoles(c *gin.Context, roles []core.Role) error {
	// Check if new user has CUSTOMER_ADMIN role
	if slices.Contains(roles, "CUSTOMER_ADMIN") && !access.IsCustomerAdmin(c) && !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		return errors.New("must be an CUSTOMER_ADMIN to assign CUSTOMER_ADMIN role to a user")
	}
	// Check if new user has ADMIN role
	if slices.Contains(roles, "ADMIN") && !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		return errors.New("must be an ADMIN to assign ADMIN role to a user")
	}
	// Check if new user has SUPER_ADMIN role
	if slices.Contains(roles, "SUPER_ADMIN") && !access.IsSuperAdmin(c) {
		return errors.New("must be an SUPER_ADMIN to assign SUPER_ADMIN role to a user")
	}
	return nil
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

	if err := checkAuthorizedRoles(c, req.Roles); err != nil {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.AddUser(c, baseAuthClient, tenantID.(string), req, nil)
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
	if err := checkAuthorizedRoles(c, req.Roles); err != nil {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
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

	// check if user is deleting self
	if userid == c.GetString(access.AUTH_USER_ID) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Cannot delete self"})
		return
	}
	// check if user has rights to delete user CUSTOMER_ADMIN, ADMIN, SUPER_ADMIN
	if !access.IsCustomerAdmin(c) && !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can delete user"})
		return
	}
	// check if user is deleting another customer admin
	user, err := uh.store.GetUserByID(c, repository.GetUserByIDParams{
		ID:       userid,
		TenantID: tenantID.(string),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	// Only CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can delete CUSTOMER_ADMIN
	if slices.Contains(user.Roles, "CUSTOMER_ADMIN") && !access.IsAdmin(c) && !access.IsSuperAdmin(c) && !access.IsCustomerAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Cannot delete CUSTOMER_ADMIN. Must be ADMIN or SUPER_ADMIN."})
		return
	}
	// Only ADMIN or SUPER_ADMIN can delete ADMIN
	if slices.Contains(user.Roles, "ADMIN") && !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Cannot delete ADMIN. Must be SUPER_ADMIN."})
		return
	}
	// Only SUPER_ADMIN can delete SUPER_ADMIN
	if slices.Contains(user.Roles, "SUPER_ADMIN") && !access.IsSuperAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Cannot delete SUPER_ADMIN. Must be SUPER_ADMIN."})
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
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

// GetUserByID implements openapi.ServerInterface.
func (uh *UserAdminHandler) GetUserByID(c *gin.Context, id string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
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
func (uh *UserAdminHandler) AssignRole(c *gin.Context, userID string, role core.Role) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.AssignRole(c, baseAuthClient, tenantID.(string), userID, role)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnassignRole implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) UnassignRole(c *gin.Context, userID string, role core.Role) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UnassignRole(c, baseAuthClient, tenantID.(string), userID, role)
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

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
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

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
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
	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c, subdomain)
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
			isCustomerAdminStr := strings.ToLower(record[headerMap["is_customer_admin"]])

			// Parse is_customer_admin value
			isCustomerAdmin := false
			if isCustomerAdminStr == "y" || isCustomerAdminStr == "yes" || isCustomerAdminStr == "Y" || isCustomerAdminStr == "YES" || isCustomerAdminStr == "Yes" {
				isCustomerAdmin = true
			}

			var req core.AddUserJSONRequestBody
			req.Email = email
			req.Name = firstname + " " + lastname

			// check if user has rights to assign roles
			if isCustomerAdmin && (!access.IsSuperAdmin(c) && !access.IsAdmin(c) && !access.IsCustomerAdmin(c)) {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: "must be an CUSTOMER_ADMIN or SUPER_ADMIN to assign CUSTOMER_ADMIN role to a user.",
				})
				failed++
				continue
			}
			if isCustomerAdmin {
				req.Roles = []core.Role{"CUSTOMER_ADMIN"}
			}
			_, err = uh.userService.AddUser(c, baseAuthClient, tenantID.(string), req, nil)
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
