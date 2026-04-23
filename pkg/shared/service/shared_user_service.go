package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

// hasCustomerAdminRole returns true if the provided roles contain CUSTOMER_ADMIN.
func hasCustomerAdminRole(roles []core.Role) bool {
	for _, r := range roles {
		if r == core.CUSTOMERADMIN {
			return true
		}
	}
	return false
}

// ADMIN and SUPER_ADMIN are global-only roles — granting them as a tenant
// membership would let them pass /admin-api and /superadmin-api gates for
// every tenant the user ever touches, breaking tenant isolation. Global
// grants must go through global_roles (metadata_public), not tenant_memberships.
func validateTenantScopedRole(role core.Role) error {
	if role == core.ADMIN || role == core.SUPERADMIN {
		return fmt.Errorf("role %s is global-only and cannot be assigned as a tenant membership", role)
	}
	return nil
}

func validateTenantScopedRoles(roles []core.Role) error {
	for _, r := range roles {
		if err := validateTenantScopedRole(r); err != nil {
			return err
		}
	}
	return nil
}

type StrategyType string

const (
	StrategyTypeGlobal StrategyType = "GLOBAL"
	StrategyTypeTenant StrategyType = "TENANT"
)

// UserStrategy defines the interface for user operations
type UserStrategy interface {
	Strategy() StrategyType
	CreateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, userRecord *auth.UserRecord, req core.NewUser, password *string) (repository.CoreUser, error)
	UpdateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, req core.UpdateUserJSONRequestBody) error
	UpdateSharedProfile(ctx context.Context, store *db.Store, userID string, req subentity.UserProfile) error
	ListUsers(c *gin.Context, store *db.Store, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error)
	AssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error
	UnAssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error
}

type SharedUserService struct {
	*BaseUserService
	authClientPool auth.AuthClientPool
}

func NewSharedUserService(store *db.Store, authClientPool auth.AuthClientPool) UserService {
	baseUserService := NewBaseUserService(store)
	userService := &SharedUserService{
		BaseUserService: baseUserService,
		authClientPool:  authClientPool,
	}
	return userService
}

func (uh *SharedUserService) getStrategy(tenantID string) UserStrategy {
	if tenantID == "" {
		return &GlobalUserStrategy{}
	}
	return &TenantUserStrategy{tenantID: tenantID}
}

// InitUserInDatabase creates a user in the database only
// Used in case the user already exists in the auth provider
func (uh *SharedUserService) InitUserInDatabase(ctx context.Context, tenantId string, userID string) (repository.CoreUser, error) {
	user, err := uh.store.CreateSharedUser(ctx, repository.CreateSharedUserParams{
		ID: userID,
	})
	return user, err
}

// CreateUser creates a new user in the auth provider and the database
func (uh *SharedUserService) CreateUser(c context.Context, authClient auth.AuthClient, tenantId string, req core.NewUser, password *string) (repository.CoreUser, error) {
	user := repository.CoreUser{}

	if err := validateTenantScopedRoles(req.Roles); err != nil {
		return user, err
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

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

	strategy := uh.getStrategy(tenantId)
	user, err = strategy.CreateUser(c, authClient, qtx, userRecord, req, password)
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

func (uh *SharedUserService) UpdateUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string, req core.UpdateUserJSONRequestBody) error {
	if err := validateTenantScopedRoles(req.Roles); err != nil {
		return err
	}

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	strategy := uh.getStrategy(tenantId)
	err = strategy.UpdateUser(c, authClient, qtx, req)
	if err != nil {
		return err
	}

	err = tx.Commit(c)

	return err
}

func (uh *SharedUserService) UpdateUserProfileInDatabase(ctx context.Context, tenantId string, userID string, req subentity.UserProfile) error {
	strategy := uh.getStrategy(tenantId)
	err := strategy.UpdateSharedProfile(ctx, uh.store, userID, req)
	return err
}

func (uh *SharedUserService) DeleteUser(c *gin.Context, authClient auth.AuthClient, userId string) error {
	logger := util.GetLoggerFromCtx(c)

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		logger.Err(err).Msg("Failed to begin transaction")
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.DeleteSharedUser(c, userId)
	if err != nil {
		logger.Err(err).Str("user_id", userId).Msg("Failed to delete user from database")
		return err
	}
	err = authClient.DeleteUser(c, userId)

	if err != nil {
		if auth.IsUserNotFound(err) {
			logger.Err(err).Msgf("User does not exist: %v", userId)
		} else {
			return err
		}
	}

	err = tx.Commit(c)

	if err != nil {
		logger.Err(err).Msg("Failed to commit transaction")
	}

	return err
}

func (uh *SharedUserService) RemoveUserFromTenant(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string) error {
	logger := util.GetLoggerFromCtx(c)
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		logger.Err(err).Msg("Failed to begin transaction")
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.DeleteSharedUserByTenant(c, repository.DeleteSharedUserByTenantParams{
		UserID:   userId,
		TenantID: tenantId,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to delete user from tenant")
		return err
	}

	err = tx.Commit(c)

	if err != nil {
		logger.Err(err).Msg("Failed to commit transaction")
	}

	return err
}

func (uh *SharedUserService) GetFullUserByID(c *gin.Context, authClient auth.AuthClient, tenantID string, id string) (FullUser, error) {
	logger := util.GetLoggerFromCtx(c)

	fullUser := FullUser{}
	coreUser, err := uh.store.GetSharedUserByTenantByID(c, repository.GetSharedUserByTenantByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		logger.Err(err).Str("user_id", id).Msg("Failed to get user from database")
		return fullUser, err
	}

	var roles []core.Role
	strategy := uh.getStrategy(tenantID)
	if strategy.Strategy() == StrategyTypeGlobal {
		roles = convertToRoleDTOs(coreUser.Roles)
	} else {
		roles = convertToRoleDTOs(coreUser.TenantRoles)
	}

	user := core.User{
		Id:        coreUser.ID,
		Name:      coreUser.Profile.Name,
		Email:     coreUser.Email.String,
		Roles:     roles,
		CreatedAt: &coreUser.CreatedAt,
	}

	userAuth, err := authClient.GetUser(c, id)
	if err != nil {
		logger.Err(err).Str("user_id", id).Msg("Failed to get user from auth provider")
		return fullUser, err
	}
	return FullUser{
		Disabled:      userAuth.Disabled,
		EmailVerified: userAuth.EmailVerified,
		Email:         userAuth.Email,
		User:          user,
	}, nil
}

func (uh *SharedUserService) GetUserByEmail(c *gin.Context, tenantID string, email string) (core.User, error) {
	logger := util.GetLoggerFromCtx(c)
	fullUser := core.User{}
	dbUser, err := uh.store.GetSharedUserByTenantByEmail(c, repository.GetSharedUserByTenantByEmailParams{
		Email:    email,
		TenantID: tenantID,
	})
	if err != nil {
		logger.Err(err).Str("email", email).Msg("Failed to get user from database")
		return fullUser, err
	}

	var roles []core.Role
	strategy := uh.getStrategy(tenantID)
	if strategy.Strategy() == StrategyTypeGlobal {
		roles = convertToRoleDTOs(dbUser.Roles)
	} else {
		roles = convertToRoleDTOs(dbUser.TenantRoles)
	}
	user := core.User{
		Id:        dbUser.ID,
		Name:      dbUser.Profile.Name,
		Email:     dbUser.Email.String,
		Roles:     roles,
		CreatedAt: &dbUser.CreatedAt,
	}
	return user, nil
}

func (uh *SharedUserService) ListUsers(c *gin.Context, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	strategy := uh.getStrategy(tenantId)
	return strategy.ListUsers(c, uh.store, pagingSql, like)
}

func (uh *SharedUserService) AssignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	if err := validateTenantScopedRole(role); err != nil {
		return err
	}

	logger := util.GetLoggerFromCtx(c)
	strategy := uh.getStrategy(tenantId)
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		logger.Err(err).Msg("Failed to begin transaction")
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	err = strategy.AssignRole(qtx, c, authClient, tenantId, userID, role)
	if err != nil {
		logger.Err(err).Msg("Failed to assign role to user")
		return err
	}
	// The new custom claims will propagate to the user's ID token the
	err = tx.Commit(c)
	if err != nil {
		logger.Err(err).Msg("Failed to commit transaction")
		return err
	}
	return nil
}

func (uh *SharedUserService) UnassignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	logger := util.GetLoggerFromCtx(c)

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		logger.Err(err).Msg("Failed to begin transaction")
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	strategy := uh.getStrategy(tenantId)
	err = strategy.UnAssignRole(qtx, c, authClient, tenantId, userID, role)
	if err != nil {
		logger.Err(err).Msg("Failed to unassign role from user")
		return err
	}
	// The new custom claims will propagate to the user's ID token the
	err = tx.Commit(c)
	if err != nil {
		logger.Err(err).Msg("Failed to commit transaction")
		return err
	}
	return nil
}
func (uh *SharedUserService) UpdateUserStatus(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, requestName string, requestValue bool) error {
	logger := util.GetLoggerFromCtx(c)
	// Lookup the user associated with the specified uid.
	params := (&auth.UserToUpdate{})
	switch requestName {
	case "EMAIL_VERIFIED":
		params = params.EmailVerified(requestValue)
	case "DISABLED":
		params = params.Disabled(requestValue)
	}
	_, err := authClient.UpdateUser(c, userID, params)
	if err != nil {
		logger.Err(err).Str("user_id", userID).Msg("Failed to update user status")
		return err
	}
	return nil
}

// AddUserToTenant adds an existing user to a tenant (creates membership)
func (uh *SharedUserService) AddUserToTenant(c context.Context, authClient auth.AuthClient, tenantID, userID string, roles []core.Role, invitedBy string) error {
	if err := validateTenantScopedRoles(roles); err != nil {
		return err
	}

	logger := util.GetLoggerFromCtx(c)
	// Check if user exists
	_, err := authClient.GetUser(c, userID)
	if err != nil {
		logger.Err(err).Str("user_id", userID).Msg("Failed to get user from auth provider")
		return errors.New("user not found in auth provider")
	}

	claims := map[string]interface{}{}
	membership := map[string]interface{}{
		"tenant_id": tenantID,
		"roles":     roles,
	}
	claims["tenant_memberships"] = membership
	err = authClient.SetCustomUserClaims(c, userID, claims)
	if err != nil {
		return err
	}

	// Convert roles to string array
	roleStrings := make([]string, len(roles))
	for i, role := range roles {
		roleStrings[i] = string(role)
	}

	// Create membership
	_, err = uh.store.AddSharedUserToTenant(c, repository.AddSharedUserToTenantParams{
		UserID:      userID,
		TenantID:    tenantID,
		TenantRoles: roleStrings,
		Status:      "active",
		InvitedBy:   pgtype.Text{String: invitedBy, Valid: invitedBy != ""},
		InvitedAt: pgtype.Timestamptz{
			Time:  time.Now(),
			Valid: true,
		},
	})
	if err != nil {
		logger.Err(err).Str("user_id", userID).Str("tenant_id", tenantID).Msg("Failed to add user to tenant in database")
		return err
	}

	return nil
}

func (uh *SharedUserService) GetUserByTenantIDByID(c *gin.Context, tenantID string, id string) (core.User, error) {
	logger := util.GetLoggerFromCtx(c)
	dbUser, err := uh.store.GetSharedUserByTenantByID(c, repository.GetSharedUserByTenantByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		logger.Err(err).Str("user_id", id).Str("tenant_id", tenantID).Msg("Failed to get user from database")
		return core.User{}, err
	}

	var roles []core.Role
	strategy := uh.getStrategy(tenantID)
	if strategy.Strategy() == StrategyTypeGlobal {
		roles = convertToRoleDTOs(dbUser.Roles)
	} else {
		roles = convertToRoleDTOs(dbUser.TenantRoles)
	}

	user := core.User{
		Id:    dbUser.ID,
		Name:  dbUser.Profile.Name,
		Email: dbUser.Email.String,
		Profile: &core.UserProfileSchema{
			Name:                 dbUser.Profile.Name,
			Title:                &dbUser.Profile.Title,
			About:                &dbUser.Profile.About,
			PictureURL:           &dbUser.Profile.PictureURL,
			BackgroundPictureURL: &dbUser.Profile.BackgroundPictureURL,
			SocialMedias:         &dbUser.Profile.SocialMedias,
			Interests:            &dbUser.Profile.Interests,
			Skills:               &dbUser.Profile.Skills,
			PhoneNumber:          &dbUser.Profile.PhoneNumber,
			Function:             &dbUser.Profile.Function,
			Company:              &dbUser.Profile.Company,
		},
		Roles:     roles,
		CreatedAt: &dbUser.CreatedAt,
	}

	return user, err
}
