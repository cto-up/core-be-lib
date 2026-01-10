package service

import (
	"context"
	"errors"
	"time"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
)

// UserStrategy defines the interface for user operations
type UserStrategy interface {
	CreateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, userRecord *auth.UserRecord, req core.NewUser, password *string) (repository.CoreUser, error)
	UpdateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, req core.UpdateUserJSONRequestBody) error
	UpdateSharedProfile(ctx context.Context, store *db.Store, userID string, req subentity.UserProfile) error
	DeleteUser(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, userId string) error
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

func (uh *SharedUserService) DeleteUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string) error {
	strategy := uh.getStrategy(tenantId)

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	err = strategy.DeleteUser(qtx, c, authClient, userId)
	if err != nil {
		return err
	}

	err = tx.Commit(c)

	return err
}

func (uh *SharedUserService) GetFullUserByID(c *gin.Context, authClient auth.AuthClient, tenantID string, id string) (FullUser, error) {
	fullUser := FullUser{}
	coreUser, err := uh.store.GetSharedUserByTenantByID(c, repository.GetSharedUserByTenantByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return fullUser, err
	}
	user := core.User{
		Id:        coreUser.ID,
		Name:      coreUser.Profile.Name,
		Email:     coreUser.Email.String,
		Roles:     convertToRoleDTOs(coreUser.Roles),
		CreatedAt: &coreUser.CreatedAt,
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
	dbUser, err := uh.store.GetSharedUserByTenantByEmail(c, repository.GetSharedUserByTenantByEmailParams{
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
	strategy := uh.getStrategy(tenantId)
	return strategy.ListUsers(c, uh.store, pagingSql, like)
}

func (uh *SharedUserService) AssignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	strategy := uh.getStrategy(tenantId)
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	err = strategy.AssignRole(qtx, c, authClient, tenantId, userID, role)
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

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	strategy := uh.getStrategy(tenantId)
	err = strategy.UnAssignRole(qtx, c, authClient, tenantId, userID, role)
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

// AddUserToTenant adds an existing user to a tenant (creates membership)
func (uh *SharedUserService) AddUserToTenant(c context.Context, authClient auth.AuthClient, tenantID, userID string, roles []core.Role, invitedBy string) error {
	// Check if user exists
	_, err := authClient.GetUser(c, userID)
	if err != nil {
		return errors.New("user not found in auth provider")
	}
	claims := map[string]interface{}{}
	// For tenant-scoped users, add tenant_memberships to metadata_public which includes tenant_id and assigned roles
	claims["tenant_memberships"] = map[string]interface{}{
		"tenant_id": tenantID,
		"roles":     roles,
	}
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
		return err
	}

	return nil
}

func (uh *SharedUserService) GetUserByTenantIDByID(c *gin.Context, tenantID string, id string) (core.User, error) {

	dbUser, err := uh.store.GetSharedUserByTenantByID(c, repository.GetSharedUserByTenantByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return core.User{}, err
	}

	user := core.User{
		Id:        dbUser.ID,
		Name:      dbUser.Profile.Name,
		Email:     dbUser.Email.String,
		Roles:     convertToRoleDTOs(dbUser.Roles),
		CreatedAt: &dbUser.CreatedAt,
	}

	return user, err
}
