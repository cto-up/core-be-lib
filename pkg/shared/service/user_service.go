package service

import (
	"context"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
)

// Context keys for tenant role information
const (
	CONTEXT_KEY_TENANT_ROLES = "tenant_roles"
)

type UserService interface {
	// Lifecycle
	CreateUser(c context.Context, authClient auth.AuthClient, tenantId string, req core.NewUser, password *string) (repository.CoreUser, error)
	UpdateUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string, req core.UpdateUserJSONRequestBody) error
	DeleteUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string) error

	InitUserInDatabase(ctx context.Context, tenantId string, userID string) (repository.CoreUser, error)
	UpdateUserProfileInDatabase(ctx context.Context, tenantId string, userID string, req subentity.UserProfile) error

	// Retrieval
	GetFullUserByID(c *gin.Context, authClient auth.AuthClient, tenantID string, id string) (FullUser, error)
	XGetUserByID(c *gin.Context, id string) (core.User, error)
	GetUserByTenantIDByID(c *gin.Context, tenantID string, id string) (core.User, error)
	GetUserByEmail(c *gin.Context, tenantId string, email string) (core.User, error)
	ListUsers(c *gin.Context, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error)

	GetUserByEmailGlobal(c context.Context, email string) (*core.User, error)

	// Roles & Status
	AssignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error
	UnassignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error
	UpdateUserStatus(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, requestName string, requestValue bool) error

	// Membership (Crucial for the Multi-Tenant implementation)
	AddUserToTenant(c context.Context, authClient auth.AuthClient, tenantID, userID string, roles []core.Role, invitedBy string) error
}

type BaseUserService struct {
	store         *db.Store
	onUserCreated UserCreatedCallback
}

func NewBaseUserService(store *db.Store) *BaseUserService {
	return &BaseUserService{
		store: store,
	}
}

func (uh *BaseUserService) XGetUserByID(c *gin.Context, id string) (core.User, error) {

	dbUser, err := uh.store.GetSharedUserByID(c, id)
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

// GetUserByEmailGlobal gets a user by email across all tenants
func (uh *BaseUserService) GetUserByEmailGlobal(c context.Context, email string) (*core.User, error) {
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

// SetUserCreatedCallback sets an optional callback function that will be called after a user is successfully created.
func (uh *BaseUserService) SetUserCreatedCallback(callback UserCreatedCallback) {
	uh.onUserCreated = callback
}
