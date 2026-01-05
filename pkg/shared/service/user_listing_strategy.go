package service

import (
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserListingStrategy defines the interface for different user listing strategies
type UserListingStrategy interface {
	ListUsers(c *gin.Context, store *db.Store, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error)
}

// FirebaseUserListingStrategy implements user listing for Firebase (legacy)
type FirebaseUserListingStrategy struct{}

func (s *FirebaseUserListingStrategy) ListUsers(c *gin.Context, store *db.Store, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Query via core_users table with tenant_id column
	dbUsers, err := store.ListUsers(c, repository.ListUsersParams{
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

// KratosUserListingStrategy implements user listing for Kratos
type KratosUserListingStrategy struct{}

func (s *KratosUserListingStrategy) ListUsers(c *gin.Context, store *db.Store, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Query via user_tenant_memberships table
	memberships, err := store.ListUsersWithMemberships(c, repository.ListUsersWithMembershipsParams{
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

// UserListingStrategyFactory creates the appropriate strategy based on provider
type UserListingStrategyFactory struct{}

func (f *UserListingStrategyFactory) CreateStrategy(providerName string) UserListingStrategy {
	switch providerName {
	case "kratos":
		return &KratosUserListingStrategy{}
	case "firebase":
		return &FirebaseUserListingStrategy{}
	default:
		// Default to Firebase for backward compatibility
		return &FirebaseUserListingStrategy{}
	}
}
