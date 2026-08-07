package core

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/service"
	fileservice "ctoup.com/coreapp/pkg/shared/fileservice"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/jackc/pgx/v5"
)

type UserHandler struct {
	store                    *db.Store
	authProvider             auth.AuthProvider
	userService              access.UserService
	emailVerificationService *service.EmailVerificationService
	fileService              *fileservice.FileService
}

func NewUserHandler(store *db.Store, authProvider auth.AuthProvider) *UserHandler {
	factory := access.NewUserServiceStrategyFactory()
	userService := factory.CreateUserServiceStrategy(store, authProvider)

	// Try to initialize user event callback if available
	// This allows the realtime module to set up the callback for user creation events
	if initFunc := access.GetUserEventInitFunc(); initFunc != nil {
		initFunc(userService)
	}

	emailVerificationService := service.NewEmailVerificationService(store, authProvider)
	fileService := fileservice.NewFileService()
	handler := &UserHandler{
		store:                    store,
		authProvider:             authProvider,
		userService:              userService,
		fileService:              fileService,
		emailVerificationService: emailVerificationService,
	}
	return handler
}

func getProfilePictureFilePath(userId any) string {
	return fileservice.ProfilePictureFilePath(userId.(string))
}

/**
* in case user was created in auth provider but not in the store
 */
func (s *UserHandler) CreateMeUser(ctx *gin.Context) {
	logger := util.GetLoggerFromCtx(ctx.Request.Context())
	userID, exist := ctx.Get(auth.AUTH_USER_ID)
	if !exist {
		ctx.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}

	userEmail, exist := ctx.Get(auth.AUTH_EMAIL)
	if !exist {
		ctx.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}
	user, err := s.store.CreateUserByTenant(ctx,
		repository.CreateUserByTenantParams{
			ID:      userID.(string),
			Email:   userEmail.(string),
			Profile: subentity.UserProfile{},
		})
	if err != nil {
		logger.Err(err).Msg("Error creating user in database")
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.JSON(http.StatusCreated, user)
}

func (s *UserHandler) GetMeProfile(ctx *gin.Context) {
	logger := util.GetLoggerFromCtx(ctx.Request.Context())

	tenantID := ctx.GetString(auth.AUTH_TENANT_ID_KEY)

	authUserID, exists := ctx.Get(auth.AUTH_USER_ID)
	if !exists {
		ctx.JSON(http.StatusBadRequest, "Not Authenticated")
		return
	}

	user, err := s.userService.GetUserByID(ctx, authUserID.(string))
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			// user does not exist yet create it
			user, err := s.userService.InitUserInDatabase(ctx, tenantID, authUserID.(string))
			if err != nil {
				logger.Err(err).Msg("Error initializing user in database")
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
	profile := user.Profile
	if profile == nil {
		profile = &core.UserProfileSchema{}
	}
	isReseller := ctx.GetBool(auth.AUTH_IS_RESELLER)
	isActingReseller := ctx.GetBool(auth.AUTH_IS_ACTING_RESELLER)
	profile.IsReseller = &isReseller
	profile.IsActingReseller = &isActingReseller

	ctx.JSON(http.StatusOK, profile)
}

// GetMyFeatureLicenses returns the authenticated user's per-user feature
// licenses (seats) within their tenant. An empty object means no per-user
// restriction — the user inherits the tenant entitlement. The frontoffice
// combines this with the tenant's feature licenses to gate access.
// (GET /api/v1/me/feature-licenses)
func (s *UserHandler) GetMyFeatureLicenses(ctx *gin.Context) {
	tenantID := ctx.GetString(auth.AUTH_TENANT_ID_KEY)

	authUserID, exists := ctx.Get(auth.AUTH_USER_ID)
	if !exists {
		ctx.JSON(http.StatusBadRequest, "Not Authenticated")
		return
	}

	licenses, err := s.store.GetUserFeatureLicenses(ctx, repository.GetUserFeatureLicensesParams{
		UserID:   authUserID.(string),
		TenantID: tenantID,
	})
	// No active membership / no row means no per-user restriction: default-allow
	// by returning an empty map so the user inherits the tenant entitlement.
	if err != nil || licenses == nil {
		ctx.JSON(http.StatusOK, subentity.TenantFeatureLicenses{})
		return
	}
	ctx.JSON(http.StatusOK, licenses)
}

func (s *UserHandler) UpdateMeProfile(ctx *gin.Context) {
	logger := util.GetLoggerFromCtx(ctx.Request.Context())

	tenantID := ctx.GetString(auth.AUTH_TENANT_ID_KEY)

	authUserID, exists := ctx.Get(auth.AUTH_USER_ID)
	if !exists {
		ctx.JSON(http.StatusBadRequest, "Not Authenticated")
		return
	}

	var req subentity.UserProfile
	if err := ctx.BindJSON(&req); err != nil {
		logger.Err(err).Msg("Error binding JSON")
		ctx.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err := s.userService.UpdateUserProfileInDatabase(ctx, tenantID, authUserID.(string), req)
	if err != nil {
		logger.Err(err).Msg("Error updating user profile in database")
		ctx.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, req)
}

func (s *UserHandler) UploadProfilePicture(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	// Get the file from the request
	file, err := c.FormFile("file")
	if err != nil {
		logger.Err(err).Msg("Failed to get file from request")
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// Save the file to a temporary location
	tmpFile, err := os.CreateTemp("", file.Filename)
	if err != nil {
		logger.Err(err).Msg("Failed to create temporary file")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer func() {
		tmpFile.Close()
		err = os.Remove(tmpFile.Name())
		if err != nil {
			logger.Error().Msg(err.Error())
		}
	}()

	// Retrieve file information
	//extension := filepath.Ext(file.Filename)
	// Generate random file name for the new uploaded file so it doesn't override the old file with same name
	//newFileName := uuid.New().String() + extension
	userId, exist := c.Get(auth.AUTH_USER_ID)
	if !exist {
		if err != nil {
			logger.Err(err).Msg("User not authenticated")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	newFilePath := getProfilePictureFilePath(userId)

	fileContent, err := file.Open()
	if err != nil {
		logger.Err(err).Msg("Failed to open file")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	byteContainer, err := io.ReadAll(fileContent) // you may want to handle the error
	if err != nil {
		logger.Err(err).Msg("Failed to read file content")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// The file is received, so let's save it
	if err := s.fileService.SaveFile(c, byteContainer, newFilePath); err != nil {
		logger.Err(err).Msg("Failed to save file")
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
	filePath := getProfilePictureFilePath(userId)

	s.fileService.GetFile(c, filePath)
}

func (uh *UserHandler) ResetPasswordRequest(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		logger.Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get Authorization client")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Authorization client"})
		return
	}
	url, err := getResetPasswordURL(c, "")
	if err != nil {
		logger.Err(err).Msg("Failed to get reset password URL")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
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

func (uh *UserHandler) Signup(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	tenant, err := uh.store.GetTenantByTenantID(c, tenantID.(string))
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !tenant.AllowSignUp {
		logger.Error().Str("tenantID", tenantID.(string)).Msg("Signup not allowed for tenant")
		c.JSON(http.StatusForbidden, gin.H{"error": "Sign up not allowed"})
		return
	}

	var req core.NewSignUp
	if err := c.BindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	welcomeURL, err := getWelcomeEmailURL(c, tenant.Subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get welcome email URL")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Check if user already exists globally (any tenant)
	existingUser, err := uh.userService.GetUserByEmailGlobal(c, req.Email)
	if err == nil && existingUser != nil {
		// Case 3: user exists and is already a member of this tenant -> no-op
		isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
			UserID:   existingUser.Id,
			TenantID: tenantID.(string),
		})
		if err != nil {
			logger.Err(err).Msg("Failed to check tenant membership")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
		if isMember {
			// Case 3: already a member. Don't leak this via the response, but
			// send an email to the real owner so a confused/legit user gets a
			// helpful nudge (sign in or reset password).
			if err := sendAlreadyRegisteredEmail(c, baseAuthClient, welcomeURL, req.Email, tenant.Name); err != nil {
				logger.Err(err).Msg("Failed to send already-registered email")
			}
			c.JSON(http.StatusOK, existingUser)
			return
		}

		// Case 2: user exists globally but not a member -> add membership and notify
		err = uh.userService.AddUserToTenant(c, baseAuthClient, tenantID.(string), existingUser.Id, []core.Role{core.USER}, "")
		if err != nil {
			logger.Err(err).Msg("Failed to add existing user to tenant")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
		if err := sendTenantAddedEmail(c, baseAuthClient, welcomeURL, req.Email, tenant.Name); err != nil {
			logger.Err(err).Msg("Failed to send tenant added email")
		}
		c.JSON(http.StatusOK, existingUser)
		return
	}

	// Case 1: new user -> create + send welcome email (password reset link)
	newUser := core.NewUser{
		Email: req.Email,
		Name:  req.Name,
		Roles: []core.Role{core.USER},
	}
	user, err := uh.userService.CreateUser(c, baseAuthClient, tenantID.(string), newUser, nil)
	if err != nil {
		logger.Err(err).Msg("Failed to create user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Update profile with extra signup fields (phoneNumber, function, company)
	if req.PhoneNumber != nil || req.Function != nil || req.Company != nil {
		profile := user.Profile
		if req.PhoneNumber != nil {
			profile.PhoneNumber = *req.PhoneNumber
		}
		if req.Function != nil {
			profile.Function = *req.Function
		}
		if req.Company != nil {
			profile.Company = *req.Company
		}
		if updateErr := uh.userService.UpdateUserProfileInDatabase(c, tenantID.(string), user.ID, profile); updateErr != nil {
			logger.Err(updateErr).Msg("Failed to update profile with signup fields")
		}
	}

	if err := sendWelcomeEmail(c, baseAuthClient, welcomeURL, req.Email); err != nil {
		logger.Err(err).Msg("Failed to send welcome email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Call the optional signed-up callback (e.g. provision default credits)
	if cb := uh.userService.GetUserSignedUpCallback(); cb != nil {
		cb(c, tenantID.(string), user)
	}

	c.JSON(http.StatusCreated, user)
}

// VerifyEmail handles email verification using token
func (uh *UserHandler) VerifyEmail(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.BindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := uh.emailVerificationService.VerifyEmailToken(c, req.Token, tenantID.(string)); err != nil {
		logger.Err(err).Msg("Failed to verify email token")
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
	logger := util.GetLoggerFromCtx(c.Request.Context())
	userID, exists := c.Get(auth.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TenantID not found"})
		return
	}

	userEmail, exists := c.Get(auth.AUTH_EMAIL)
	if !exists {
		logger.Error().Msg("User email not found")
		c.JSON(http.StatusBadRequest, gin.H{"error": "User email not found"})
		return
	}

	// Check if email is already verified
	isVerified, err := uh.emailVerificationService.GetUserVerificationStatus(c, userID.(string), tenantID.(string))
	if err != nil {
		logger.Err(err).Msg("Failed to check verification status")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check verification status"})
		return
	}

	if isVerified {
		logger.Err(err).Msg("Email already verified")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
		return
	}

	// Get base URL for verification link
	url, err := getConfirmationEmailURL(c)
	if err != nil {
		logger.Err(err).Msg("Failed to generate verification URL")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate verification URL"})
		return
	}

	// Resend verification email (includes rate limiting)
	if err := uh.emailVerificationService.ResendVerificationEmail(c, userID.(string), tenantID.(string), userEmail.(string), url); err != nil {
		logger.Err(err).Msg("Failed to send verification email")
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
	logger := util.GetLoggerFromCtx(c.Request.Context())
	userID, exists := c.Get(auth.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TenantID not found"})
		return
	}

	userEmail, exists := c.Get(auth.AUTH_EMAIL)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User email not found"})
		return
	}

	// Get verification status
	isVerified, err := uh.emailVerificationService.GetUserVerificationStatus(c, userID.(string), tenantID.(string))
	if err != nil {
		logger.Err(err).Msg("Failed to get verification status")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get verification status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email":          userEmail.(string),
		"email_verified": isVerified,
	})
}

// GetUserByEmail implements openapi.ServerInterface.
func (uh *UserHandler) GetUserByEmail(c *gin.Context, email string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		logger.Error().Msg("TenantID not found")
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	user, err := uh.userService.GetUserByEmail(c, tenantID.(string), email)
	if err != nil {
		logger.Err(err).Msg("Failed to get user by email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, user)
}

// IdentifyUser implements openapi.ServerInterface.
func (uh *UserHandler) IdentifyUser(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	if uh.authProvider.GetProviderName() != "kratos" {
		logger.Warn().Str("provider", uh.authProvider.GetProviderName()).Msg("Identify not implemented for this provider")
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Identification processed (not supported for provider)"})
		return
	}

	var req core.Identify
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	req.Email = openapi_types.Email(strings.ToLower(strings.TrimSpace(string(req.Email))))

	origin := c.Request.Header.Get("Origin")
	if origin == "" {
		// Fallback to Referer if Origin is missing
		referer := c.Request.Header.Get("Referer")
		if referer != "" {
			if u, err := url.Parse(referer); err == nil {
				origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
			}
		}
	}

	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		logger.Error().Msg("TenantID not found")
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	tenant, err := uh.store.GetTenantByTenantID(c, tenantID.(string))
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if !tenant.AllowSignUp {
		logger.Error().Str("tenantID", tenantID.(string)).Msg("Signup not allowed for tenant")
		// POLA: return 200 as requested
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Signup not allowed"})
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
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Check if user exists globally
	globalUser, err := uh.userService.GetUserByEmailGlobal(c, string(req.Email))
	if err != nil {

		newUserReq := core.NewUser{
			Email: string(req.Email),
			Name:  string(req.Email), // Default name to email
			Roles: []core.Role{core.USER},
		}
		user, err := uh.userService.CreateUser(c, baseAuthClient, tenantID.(string), newUserReq, nil)
		if err != nil {
			logger.Err(err).Str("email", string(req.Email)).Msg("Failed to create user during identification")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

		// Send magic link
		err = sendMagicLink(c, baseAuthClient, origin, string(req.Email))
		if err != nil {
			logger.Err(err).Str("email", string(req.Email)).Msg("Failed to send magic link")
		}
		logger.Info().Str("email", user.Email.String).Msg("Magic link sent for new user")
	} else {
		// User exists globally
		// Check if member of tenant
		isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
			UserID:   globalUser.Id,
			TenantID: tenantID.(string),
		})
		if err != nil {
			logger.Err(err).Msg("Failed to check tenant membership")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

		if !isMember {
			// Add to tenant
			err = uh.userService.AddUserToTenant(c, baseAuthClient, tenantID.(string), globalUser.Id, []core.Role{core.USER}, "")
			if err != nil {
				logger.Err(err).Str("email", string(req.Email)).Msg("Failed to add user to tenant during identification")
				c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
				return
			}
		}

		// Send sign-in link
		err = sendSigninEmail(c, origin, string(req.Email))
		if err != nil {
			logger.Err(err).Str("email", string(req.Email)).Msg("Failed to send sign-in email")
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Verification flow initiated"})
}

// CompleteSocialSignIn implements openapi.ServerInterface.
// (POST /public-api/v1/auth/social/complete)
//
// Password sign-up goes through IdentifyUser, where the BACKEND creates the
// identity and therefore controls which tenant it belongs to. Social sign-up
// does not: Kratos mints the identity itself the moment the provider hands back
// a verified email, and Kratos knows nothing about tenants. The result is a live
// session whose identity carries no metadata_public.tenant_memberships entry —
// which the auth middleware rejects with a 401 on every subsequent call. To the
// user that reads as "sign-in worked, then the app broke".
//
// This closes that gap from the app side. The tenant comes from the browser's
// own request — TenantMiddleware resolves it from the Origin header, falling
// back to Host (util.GetHost). Origin is what carries it in practice: the SPA
// calls this on the tenant-NEUTRAL api.<domain> ingress, whose Host names no
// tenant, while Origin is the tenant site the user is on.
//
// That is preferred over a Kratos web_hook registration hook, which fires
// inside Kratos — one shared host serving every tenant — and would have to
// infer the tenant by parsing the flow's return_to. It would also miss an
// existing user signing into a second tenant, since no registration occurs.
func (uh *UserHandler) CompleteSocialSignIn(c *gin.Context) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	sessionToken := socialSessionToken(c)
	if sessionToken == "" {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("no session")))
		return
	}

	authClient := uh.authProvider.GetAuthClient()
	token, err := authClient.VerifyIDToken(c.Request.Context(), sessionToken)
	if err != nil {
		logger.Warn().Err(err).Msg("Social sign-in: session verification failed")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(errors.New("invalid session")))
		return
	}

	tenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
	if tenantID == "" {
		// Root domain: there is no tenant to join, and only SUPER_ADMIN belongs
		// there at all — nothing to provision either way.
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("no tenant for this host")))
		return
	}

	if hasTenantMembership(token.Claims, tenantID) {
		c.JSON(http.StatusOK, core.SocialSignInResult{Provisioned: false})
		return
	}

	tenant, err := uh.store.GetTenantByTenantID(c, tenantID)
	if err != nil {
		logger.Err(err).Str("tenant_id", tenantID).Msg("Social sign-in: failed to load tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// The same gate IdentifyUser applies. A social identity is not a membership:
	// without this, anyone with a Google account could join any tenant simply by
	// clicking the button on its sign-in page.
	if !tenant.AllowSignUp {
		logger.Warn().
			Str("tenant_id", tenantID).
			Str("user_id", token.UID).
			Msg("Social sign-in: tenant does not allow self-service signup")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("signup not allowed")))
		return
	}

	email, _ := token.Claims["email"].(string)
	name, _ := token.Claims["name"].(string)
	if name == "" {
		name = email
	}

	// The identity exists in the auth provider but the app's own user row may
	// not — every other creation path writes both at once, this one cannot.
	if _, err := uh.store.GetSharedUserByID(c, token.UID); err != nil {
		if _, createErr := uh.store.CreateSharedUser(c, repository.CreateSharedUserParams{
			ID:      token.UID,
			Email:   email,
			Profile: subentity.UserProfile{Name: name},
		}); createErr != nil {
			logger.Err(createErr).Str("user_id", token.UID).Msg("Social sign-in: failed to create user row")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(createErr))
			return
		}
	}

	// Writes the membership row AND the identity's metadata_public claims, and
	// runs the seat guard plus the per-tenant onboarding callbacks — the same
	// choke point admin-side invites go through.
	if err := uh.userService.AddUserToTenant(c, authClient, tenantID, token.UID, []core.Role{core.USER}, ""); err != nil {
		logger.Err(err).
			Str("user_id", token.UID).
			Str("tenant_id", tenantID).
			Msg("Social sign-in: failed to attach user to tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	logger.Info().
		Str("user_id", token.UID).
		Str("tenant_id", tenantID).
		Msg("Social sign-in: user provisioned into tenant")

	c.JSON(http.StatusOK, core.SocialSignInResult{Provisioned: true})
}

// socialSessionToken mirrors the extraction order of the Kratos auth provider,
// so a request this endpoint accepts is one the auth middleware would also have
// accepted had the membership existed.
func socialSessionToken(c *gin.Context) string {
	if token := c.GetHeader("X-Session-Token"); token != "" {
		return token
	}
	if cookie, err := c.Cookie("ory_kratos_session"); err == nil && cookie != "" {
		return cookie
	}
	if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

// hasTenantMembership reports whether the session's own claims already place
// this identity in the tenant. Reading the claims rather than the database is
// deliberate: the claims are what the auth middleware will consult on the next
// request, so this answers "will the app work now?", not "is there a row?".
func hasTenantMembership(claims map[string]interface{}, tenantID string) bool {
	memberships, ok := claims[auth.AUTH_TENANT_MEMBERSHIPS].([]interface{})
	if !ok {
		return false
	}
	for _, m := range memberships {
		membership, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if id, _ := membership["tenant_id"].(string); id == tenantID {
			return true
		}
	}
	return false
}
