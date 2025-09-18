package core

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/service"
	fileservice "ctoup.com/coreapp/pkg/shared/fileservice"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"

	"github.com/jackc/pgx/v5"
)

type UserHandler struct {
	store                    *db.Store
	authClientPool           *access.FirebaseTenantClientConnectionPool
	userService              *access.UserService
	emailVerificationService *service.EmailVerificationService
	fileService              *fileservice.FileService
}

func NewUserHandler(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *UserHandler {
	userService := access.NewUserService(store, authClientPool)
	emailVerificationService := service.NewEmailVerificationService(store, authClientPool)
	fileService := fileservice.NewFileService()
	handler := &UserHandler{
		store:                    store,
		authClientPool:           authClientPool,
		userService:              userService,
		fileService:              fileService,
		emailVerificationService: emailVerificationService,
	}
	return handler
}

func getProfilePictureFilePath(tenantId string, userId any) string {
	var tenantPart string
	if tenantId != "" {
		tenantPart = tenantId
	} else {
		tenantPart = "www"
	}

	newFilePath := `/tenants/` + tenantPart + `/core/users/` + userId.(string) + "/profile-picture.jpg"
	return newFilePath
}

/**
* in case user was created in firebase but not in the store
 */
func (s *UserHandler) CreateMeUser(ctx *gin.Context) {
	userID, exist := ctx.Get(access.AUTH_USER_ID)
	if !exist {
		ctx.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}

	userEmail, exist := ctx.Get(access.AUTH_EMAIL)
	if !exist {
		ctx.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}
	user, err := s.store.CreateUser(ctx,
		repository.CreateUserParams{
			ID:      userID.(string),
			Email:   userEmail.(string),
			Profile: subentity.UserProfile{},
		})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.JSON(http.StatusCreated, user)
}

func (s *UserHandler) GetMeProfile(ctx *gin.Context) {
	tenantID, exists := ctx.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		ctx.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	authUserID, exists := ctx.Get(access.AUTH_USER_ID)
	if !exists {
		ctx.JSON(http.StatusBadRequest, "Not Authenticated")
		return
	}
	user, err := s.store.GetUserByID(ctx, repository.GetUserByIDParams{
		ID:       authUserID.(string),
		TenantID: tenantID.(string)})
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			// user does not exist yet create it
			user, err := s.store.CreateUser(ctx, repository.CreateUserParams{
				ID: authUserID.(string),
			})
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
				return
			}
			ctx.JSON(http.StatusOK, user)
			return
		} else {
			ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
	}
	ctx.JSON(http.StatusOK, user.Profile)
}

func (s *UserHandler) UpdateMeProfile(ctx *gin.Context) {

	authUserID, exists := ctx.Get(access.AUTH_USER_ID)
	if !exists {
		ctx.JSON(http.StatusBadRequest, "Not Authenticated")
		return
	}

	var req subentity.UserProfile
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	_, err := s.store.UpdateProfile(ctx, repository.UpdateProfileParams{
		ID:       authUserID.(string),
		Profile:  req,
		TenantID: ctx.GetString(access.AUTH_TENANT_ID_KEY),
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (s *UserHandler) UploadProfilePicture(c *gin.Context) {
	// Get the file from the request
	file, err := c.FormFile("file")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get file from request")
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// Save the file to a temporary location
	tmpFile, err := os.CreateTemp("", file.Filename)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create temporary file")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer func() {
		tmpFile.Close()
		err = os.Remove(tmpFile.Name())
		if err != nil {
			log.Error().Msg(err.Error())
		}
	}()

	// Retrieve file information
	//extension := filepath.Ext(file.Filename)
	// Generate random file name for the new uploaded file so it doesn't override the old file with same name
	//newFileName := uuid.New().String() + extension
	userId, exist := c.Get(access.AUTH_USER_ID)
	if !exist {
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	tenantId, _ := c.Get(access.AUTH_TENANT_ID_KEY)
	newFilePath := getProfilePictureFilePath(tenantId.(string), userId)

	fileContent, err := file.Open()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open file")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	byteContainer, err := io.ReadAll(fileContent) // you may want to handle the error
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// The file is received, so let's save it
	if err := s.fileService.SaveFile(c, byteContainer, newFilePath); err != nil {
		log.Err(err).Msg("Failed to save file")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"message": "Unable to save the file",
		})
		return
	}

	// File saved successfully. Return proper result
	c.JSON(http.StatusOK, gin.H{
		"message": "Your file has been successfully uploaded.",
	})
}

func (s *UserHandler) GetProfilePicture(c *gin.Context, userId string, params core.GetProfilePictureParams) {
	tenantId, _ := c.Get(access.AUTH_TENANT_ID_KEY)
	filePath := getProfilePictureFilePath(tenantId.(string), userId)

	s.fileService.GetFile(c, filePath)
}

func (uh *UserHandler) ResetPasswordRequest(c *gin.Context) {
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

func (uh *UserHandler) Signup(c *gin.Context) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	tenant, err := uh.store.GetTenantByTenantID(c, tenantID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !tenant.AllowSignUp {
		c.JSON(http.StatusForbidden, gin.H{"error": "Sign up not allowed"})
		return
	}

	var req core.NewSignUp
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	newUser := core.NewUser{
		Email: req.Email,
		Name:  req.Name,
		Roles: []core.Role{"USER"},
	}

	user, err := uh.userService.AddUser(c, baseAuthClient, tenantID.(string), newUser, &req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	// create email verification token
	token, err := uh.emailVerificationService.CreateEmailVerificationToken(c, user.ID, tenantID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	url, err := getConfirmationEmailURL(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = sendConfirmationEmail(url, req.Email, token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, user)

	c.JSON(http.StatusOK, gin.H{"message": "Verification email sent"})
}

// VerifyEmail handles email verification using token
func (uh *UserHandler) VerifyEmail(c *gin.Context) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := uh.emailVerificationService.VerifyEmailToken(c, req.Token, tenantID.(string)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Email verified successfully",
		"email_verified": true,
	})
}

// ResendEmailVerification resends verification email to authenticated user
func (uh *UserHandler) ResendEmailVerification(c *gin.Context) {
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TenantID not found"})
		return
	}

	userEmail, exists := c.Get(access.AUTH_EMAIL)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User email not found"})
		return
	}

	// Check if email is already verified
	isVerified, err := uh.emailVerificationService.GetUserVerificationStatus(c, userID.(string), tenantID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check verification status"})
		return
	}

	if isVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
		return
	}

	// Get base URL for verification link
	url, err := getConfirmationEmailURL(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate verification URL"})
		return
	}

	// Resend verification email (includes rate limiting)
	if err := uh.emailVerificationService.ResendVerificationEmail(c, userID.(string), tenantID.(string), userEmail.(string), url); err != nil {
		// Check if it's a rate limit error
		if fmt.Sprintf("%v", err) == "rate limit exceeded" ||
			(len(fmt.Sprintf("%v", err)) > 20 && fmt.Sprintf("%v", err)[:20] == "rate limit exceeded.") {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Verification email sent successfully"})
}

// GetMyEmailVerificationStatus returns current user's email verification status
func (uh *UserHandler) GetMyEmailVerificationStatus(c *gin.Context) {
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TenantID not found"})
		return
	}

	userEmail, exists := c.Get(access.AUTH_EMAIL)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User email not found"})
		return
	}

	// Get verification status
	isVerified, err := uh.emailVerificationService.GetUserVerificationStatus(c, userID.(string), tenantID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get verification status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email":          userEmail.(string),
		"email_verified": isVerified,
	})
}
