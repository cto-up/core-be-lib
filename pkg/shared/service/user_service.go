package service

import (
	"context"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
)

type UserService interface {
	// Lifecycle
	AddUser(c context.Context, authClient auth.AuthClient, tenantId string, req core.NewUser, password *string) (repository.CoreUser, error)
	UpdateUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string, req core.UpdateUserJSONRequestBody) error
	DeleteUser(c *gin.Context, authClient auth.AuthClient, tenantId string, userId string) error

	CreateUserInDatabase(ctx context.Context, tenantId string, userID string) (repository.CoreUser, error)
	UpdateUserProfileInDatabase(ctx context.Context, tenantId string, userID string, req subentity.UserProfile) error

	// Retrieval
	GetUserByID(c *gin.Context, authClient auth.AuthClient, id string) (FullUser, error)
	GetUserByEmail(c *gin.Context, tenantId string, email string) (core.User, error)
	ListUsers(c *gin.Context, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error)

	GetUserByEmailGlobal(c context.Context, email string) (*core.User, error)

	// Roles & Status
	AssignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error
	UnassignRole(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error
	UpdateUserStatus(c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, requestName string, requestValue bool) error

	// Membership (Crucial for the Multi-Tenant implementation)
	AddUserToTenant(c context.Context, authClient auth.AuthClient, tenantID, userID string, roles []core.Role) error
}
