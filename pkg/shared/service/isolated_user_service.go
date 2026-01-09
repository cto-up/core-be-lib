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

type IsolatedUserService struct {
	*BaseUserService
	authClientPool auth.AuthClientPool
}

func NewIsolatedUserService(store *db.Store, authClientPool auth.AuthClientPool) UserService {
	baseUserService := NewBaseUserService(store)
	userService := &IsolatedUserService{
		BaseUserService: baseUserService,
		authClientPool:  authClientPool,
	}
	return userService
}

// CreateUser creates a new user in the auth provider and the database
func (uh *IsolatedUserService) CreateUser(c context.Context, authClient auth.AuthClient, tenantId string, req core.NewUser, password *string) (repository.CoreUser, error) {

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

// InitUserInDatabase creates a user in the database only
// Used in case the user already exists in the auth provider
func (uh *IsolatedUserService) InitUserInDatabase(ctx context.Context, tenantId string, userID string) (repository.CoreUser, error) {
	user, err := uh.store.CreateUserByTenant(ctx, repository.CreateUserByTenantParams{
		ID:       userID,
		TenantID: tenantId,
	})
	return user, err
}

func (uh *IsolatedUserService) UpdateUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string, req core.UpdateUserJSONRequestBody) error {
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

// InitUserInDatabase creates a user in the database only
func (uh *IsolatedUserService) UpdateUserProfileInDatabase(ctx context.Context, tenantId string, userID string, req subentity.UserProfile) error {
	_, err := uh.store.UpdateProfileByTenant(ctx, repository.UpdateProfileByTenantParams{
		ID:       userID,
		Profile:  req,
		TenantID: tenantId,
	})
	return err
}

func (uh *IsolatedUserService) DeleteUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string) error {
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

func (uh *IsolatedUserService) GetFullUserByID(c *gin.Context, authClient auth.AuthClient, tenantID string, id string) (FullUser, error) {
	fullUser := FullUser{}

	userDB, err := uh.store.GetUserByTenantByID(c, repository.GetUserByTenantByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return fullUser, err
	}
	user := core.User{
		Id:        userDB.ID,
		Name:      userDB.Profile.Name,
		Email:     userDB.Email.String,
		Roles:     convertToRoleDTOs(userDB.Roles),
		CreatedAt: &userDB.CreatedAt,
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

func (uh *IsolatedUserService) GetUserByEmail(c *gin.Context, tenantId string, email string) (core.User, error) {
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

func (uh *IsolatedUserService) ListUsers(c *gin.Context, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Delegate to the appropriate strategy
	// Query via core_users table with tenant_id column
	dbUsers, err := uh.store.ListUsersByTenant(c, repository.ListUsersByTenantParams{
		Limit:    pagingSql.PageSize,
		Offset:   pagingSql.Offset,
		Like:     like,
		TenantID: tenantId,
	})

	if err != nil {
		return []core.User{}, err
	}

	// Convert db users to core users
	users := make([]core.User, len(dbUsers))
	for j, dbUser := range dbUsers {
		user := core.User{
			Id:        dbUser.ID,
			Name:      dbUser.Profile.Name,
			Email:     dbUser.Email.String,
			Roles:     convertToRoleDTOs(dbUser.Roles),
			CreatedAt: &dbUser.CreatedAt,
		}
		users[j] = user
	}

	return users, nil
}

func (uh *IsolatedUserService) AssignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
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

func (uh *IsolatedUserService) UnassignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	if !IsAdmin(c) && !IsSuperAdmin(c) && !IsCustomerAdmin(c) {
		return errors.New("must be an CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == "ADMIN" && (!IsAdmin(c) || !IsSuperAdmin(c)) {
		return errors.New("must be an CUSTOMER_ADMIN to perform such operation")
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

	_, err = qtx.UnassignRoleWithRowsAffected(c, repository.UnassignRoleWithRowsAffectedParams{
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
func (uh *IsolatedUserService) UpdateUserStatus(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, requestName string, requestValue bool) error {
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

// AddUserToTenant is not applicable for IsolatedUserService
func (uh *IsolatedUserService) AddUserToTenant(c context.Context, authClient auth.AuthClient, tenantID, userID string, roles []core.Role) error {
	return nil
}
