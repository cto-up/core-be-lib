package service

import (
	"context"
	"errors"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
)

type SharedUserService struct {
	store          *db.Store
	authClientPool auth.AuthClientPool
	onUserCreated  UserCreatedCallback
}

func NewSharedUserService(store *db.Store, authClientPool auth.AuthClientPool) UserService {
	userService := &SharedUserService{
		store:          store,
		authClientPool: authClientPool,
	}
	return userService
}

// SetUserCreatedCallback sets an optional callback function that will be called after a user is successfully created.
func (uh *SharedUserService) SetUserCreatedCallback(callback UserCreatedCallback) {
	uh.onUserCreated = callback
}

// AddUser creates a new user in the auth provider and the database
func (uh *SharedUserService) AddUser(c context.Context, authClient auth.AuthClient, tenantId string, req core.NewUser, password *string) (repository.CoreUser, error) {

	user := repository.CoreUser{}
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return user, err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	params := (&auth.UserToCreate{}).
		Email(req.Email).
		EmailVerified(false).
		DisplayName(req.Name).
		PhotoURL("/images/avatar-1.jpeg").
		Disabled(false)

	if password != nil {
		params = params.Password(*password)
	}

	userRecord, err := authClient.CreateUser(c, params)
	if err != nil {
		return user, err
	}

	claims := map[string]interface{}{}
	for _, role := range req.Roles {
		claims[string(role)] = true
	}
	if len(req.Roles) > 0 {
		err = authClient.SetCustomUserClaims(c, userRecord.UID, claims)
		if err != nil {
			return user, err
		}
	}

	user, err = qtx.CreateUserByTenant(c,
		repository.CreateUserByTenantParams{
			ID:    userRecord.UID,
			Email: req.Email,
			Profile: subentity.UserProfile{
				Name: req.Name,
			},
			Roles:    convertToRoles(req.Roles),
			TenantID: tenantId,
		})
	if err != nil {
		return user, err
	}
	err = tx.Commit(c)
	if err != nil {
		return user, err
	}

	// Call the optional callback if it's set
	if uh.onUserCreated != nil {
		uh.onUserCreated(c, tenantId, user)
	}

	return user, err
}

// CreateUserInDatabase creates a user in the database only
func (uh *SharedUserService) CreateUserInDatabase(ctx context.Context, tenantId string, userID string) (repository.CoreUser, error) {
	user, err := uh.store.CreateUserByTenant(ctx, repository.CreateUserByTenantParams{
		ID:       userID,
		TenantID: tenantId,
	})
	return user, err
}

func (uh *SharedUserService) UpdateUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string, req core.UpdateUserJSONRequestBody) error {
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	params := (&auth.UserToUpdate{}).
		Email(req.Email).
		EmailVerified(false).
		DisplayName(req.Name).
		PhotoURL("/images/avatar-1.jpeg").
		Disabled(false)

	_, err = authClient.UpdateUser(c, userId, params)
	if err != nil {
		return err
	}

	claims := map[string]interface{}{}
	for _, role := range req.Roles {
		claims[string(role)] = true
	}
	err = authClient.SetCustomUserClaims(c, userId, claims)
	if err != nil {
		return err
	}
	// Display Name

	_, err = qtx.UpdateUserByTenant(c, repository.UpdateUserByTenantParams{
		ID:       userId,
		Roles:    convertToRoles(req.Roles),
		Name:     req.Name,
		TenantID: tenantId,
	})
	if err != nil {
		return err
	}

	err = tx.Commit(c)

	return err
}

// CreateUserInDatabase creates a user in the database only
func (uh *SharedUserService) UpdateUserProfileInDatabase(ctx context.Context, tenantId string, userID string, req subentity.UserProfile) error {
	_, err := uh.store.UpdateProfileByTenant(ctx, repository.UpdateProfileByTenantParams{
		ID:       userID,
		Profile:  req,
		TenantID: tenantId,
	})
	return err
}

func (uh *SharedUserService) DeleteUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string) error {
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}

	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.DeleteUserByTenant(c, repository.DeleteUserByTenantParams{
		ID:       userId,
		TenantID: tenantId,
	})
	if err != nil {
		return err
	}

	err = authClient.DeleteUser(c, userId)

	if err != nil {
		if auth.IsUserNotFound(err) {
			log.Error().Err(err).Msgf("User does not exist: %v", userId)
		} else {
			return err
		}
	}

	err = tx.Commit(c)

	return err
}

func (uh *SharedUserService) GetUserByID(c *gin.Context, authClient auth.AuthClient, id string) (FullUser, error) {
	fullUser := FullUser{}
	dbUser, err := uh.store.GetUserByID(c, id)
	if err != nil {
		return fullUser, err
	}

	user := core.User{
		Id:        dbUser.ID,
		Name:      dbUser.Profile.Name,
		Email:     dbUser.Email.String,
		Roles:     convertToRoleDTOs(dbUser.Roles),
		CreatedAt: &dbUser.CreatedAt,
	}

	userAuth, err := authClient.GetUser(c, id)
	if err != nil {
		return fullUser, err
	}
	return FullUser{
		Disabled:      userAuth.Disabled,
		EmailVerified: userAuth.EmailVerified,
		Email:         userAuth.Email,
		User:          user,
	}, nil
}

func (uh *SharedUserService) GetUserByEmail(c *gin.Context, tenantId string, email string) (core.User, error) {
	fullUser := core.User{}
	dbUser, err := uh.store.GetUserByTenantByEmail(c, repository.GetUserByTenantByEmailParams{
		Email:    email,
		TenantID: tenantId,
	})
	if err != nil {
		return fullUser, err
	}

	user := core.User{
		Id:        dbUser.ID,
		Name:      dbUser.Profile.Name,
		Email:     dbUser.Email.String,
		Roles:     convertToRoleDTOs(dbUser.Roles),
		CreatedAt: &dbUser.CreatedAt,
	}
	return user, nil
}

func (uh *SharedUserService) ListUsers(c *gin.Context, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Query via user_tenant_memberships table
	memberships, err := uh.store.ListUsersWithMemberships(c, repository.ListUsersWithMembershipsParams{
		TenantID: tenantId,
		Limit:    pagingSql.PageSize,
		Offset:   pagingSql.Offset,
		Like:     like,
	})

	if err != nil {
		return []core.User{}, err
	}

	// Convert memberships to users
	users := make([]core.User, len(memberships))
	for j, membership := range memberships {
		user := core.User{
			Id:        membership.ID,
			Name:      membership.Profile.Name,
			Email:     membership.Email.String,
			Roles:     convertToRoleDTOs(membership.Roles),
			CreatedAt: &membership.CreatedAt,
		}
		users[j] = user
	}

	return users, nil
}

func (uh *SharedUserService) AssignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	if !IsAdmin(c) || !IsSuperAdmin(c) {
		return errors.New("must be an ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == "CUSTOMER_ADMIN" && (!IsCustomerAdmin(c) && !IsSuperAdmin(c) && !IsAdmin(c)) {
		return errors.New("must be at a CUSTOMER_ADMIN or SUPER_ADMIN or ADMIN to perform such operation")
	}
	if role == "ADMIN" && (!IsSuperAdmin(c) && !IsAdmin(c)) {
		return errors.New("must be an ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == "SUPER_ADMIN" && !IsSuperAdmin(c) {
		return errors.New("must be an SUPER_ADMIN to perform such operation")
	}

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.AssignRoleWithRowsAffected(c, repository.AssignRoleWithRowsAffectedParams{
		UserID:   userID,
		RoleName: string(role),
		TenantID: tenantId,
	})
	if err != nil {
		return err
	}

	// Lookup the user associated with the specified uid.
	user, err := authClient.GetUser(c, userID)
	if err != nil {
		return err
	}

	var claims map[string]interface{}
	if user.CustomClaims == nil {
		claims = map[string]interface{}{}
	} else {
		claims = user.CustomClaims
	}

	claims[string(role)] = true
	err = authClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
	if err != nil {
		return err
	}
	// The new custom claims will propagate to the user's ID token the
	err = tx.Commit(c)
	if err != nil {
		return err
	}
	return nil
}

func (uh *SharedUserService) UnassignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	if !IsAdmin(c) && !IsSuperAdmin(c) && !IsCustomerAdmin(c) {
		return errors.New("must be an CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == "ADMIN" && (!IsAdmin(c) || !IsSuperAdmin(c)) {
		return errors.New("must be an CUSTOMER_ADMIN to perform such operation")
	}

	if role == "SUPER_ADMIN" && !IsSuperAdmin(c) {
		return errors.New("must be an SUPER_ADMIN to perform such operation")
	}
	tenant_id, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		return errors.New("user email not found in context")
	}

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.UnassignRoleWithRowsAffected(c, repository.UnassignRoleWithRowsAffectedParams{
		UserID:   userID,
		RoleName: string(role),
		TenantID: tenant_id.(string),
	})
	if err != nil {
		return err
	}

	// Lookup the user associated with the specified uid.
	user, err := authClient.GetUser(c, userID)
	if err != nil {
		return err
	}

	claims := user.CustomClaims
	claims[string(role)] = false
	err = authClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
	if err != nil {
		return err
	}
	// The new custom claims will propagate to the user's ID token the
	err = tx.Commit(c)
	if err != nil {
		return err
	}
	return nil
}
func (uh *SharedUserService) UpdateUserStatus(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, requestName string, requestValue bool) error {
	// Lookup the user associated with the specified uid.
	params := (&auth.UserToUpdate{})
	switch requestName {
	case "EMAIL_VERIFIED":
		params = params.EmailVerified(requestValue)
	case "DISABLED":
		params = params.Disabled(requestValue)
	}
	_, err := authClient.UpdateUser(c, userID, params)
	return err
}

// GetUserByEmailGlobal gets a user by email across all tenants
func (uh *SharedUserService) GetUserByEmailGlobal(c context.Context, email string) (*core.User, error) {
	userRow, err := uh.store.GetUserByEmailGlobal(c, email)
	if err != nil {
		return nil, err
	}

	// Convert to core.User
	user := &core.User{
		Id:    userRow.ID,
		Email: userRow.Email.String,
		Profile: &core.UserProfileSchema{
			Name: userRow.Profile.Name,
		},
		CreatedAt: &userRow.CreatedAt,
	}

	return user, nil
}

// AddUserToTenant adds an existing user to a tenant (creates membership)
func (uh *SharedUserService) AddUserToTenant(c context.Context, authClient auth.AuthClient, tenantID, userID string, roles []core.Role) error {
	// Check if user exists
	_, err := authClient.GetUser(c, userID)
	if err != nil {
		return errors.New("user not found in auth provider")
	}

	// Convert roles to string array
	roleStrings := make([]string, len(roles))
	for i, role := range roles {
		roleStrings[i] = string(role)
	}

	// Create membership
	_, err = uh.store.CreateUserTenantMembership(c, repository.CreateUserTenantMembershipParams{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    roleStrings,
		Status:   "active",
	})
	if err != nil {
		return err
	}

	// Create user record in core_users for this tenant if it doesn't exist
	_, err = uh.store.GetUserByID(c, userID)
	if err != nil {
		// User doesn't exist for this tenant, create it
		// Get user info from auth provider
		authUser, err := authClient.GetUser(c, userID)
		if err != nil {
			return err
		}

		profile := subentity.UserProfile{}
		if authUser.DisplayName != "" {
			profile.Name = authUser.DisplayName
		}

		_, err = uh.store.CreateUserByTenant(c, repository.CreateUserByTenantParams{
			ID:       userID,
			Email:    authUser.Email,
			Profile:  profile,
			Roles:    roleStrings,
			TenantID: tenantID,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to create user record for tenant")
			// Don't fail the whole operation if this fails
		}
	}
	return nil
}
