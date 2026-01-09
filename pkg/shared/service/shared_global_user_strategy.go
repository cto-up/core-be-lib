package service

import (
	"context"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// GlobalUserStrategy handles operations for global users
type GlobalUserStrategy struct{}

func (g *GlobalUserStrategy) CreateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, userRecord *auth.UserRecord, req core.NewUser, password *string) (repository.CoreUser, error) {
	claims := map[string]interface{}{}
	for _, role := range req.Roles {
		claims[string(role)] = true
	}
	user := repository.CoreUser{}
	if len(req.Roles) > 0 {
		err := authClient.SetCustomUserClaims(c, userRecord.UID, claims)
		if err != nil {
			return user, err
		}
	}

	user, err := qtx.CreateSharedUser(c,
		repository.CreateSharedUserParams{
			ID:    userRecord.UID,
			Email: req.Email,
			Profile: subentity.UserProfile{
				Name: req.Name,
			},
			Roles: convertToRoles(req.Roles),
		})
	return user, err
}

func (g *GlobalUserStrategy) UpdateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, req core.UpdateUserJSONRequestBody) error {

	claims := map[string]interface{}{}

	for _, role := range req.Roles {
		claims[string(role)] = true
	}

	params := (&auth.UserToUpdate{}).
		Email(req.Email).
		EmailVerified(false).
		DisplayName(req.Name).
		PhotoURL("/images/avatar-1.jpeg").
		Disabled(false)

	_, err := authClient.UpdateUser(c, req.Id, params)
	if err != nil {
		return err
	}

	_, err = qtx.UpdateSharedUser(c,
		repository.UpdateSharedUserParams{
			ID:    req.Id,
			Name:  req.Name,
			Roles: convertToRoles(req.Roles),
		})
	return err
}

func (g *GlobalUserStrategy) UpdateSharedProfile(ctx context.Context, store *db.Store, userID string, req subentity.UserProfile) error {
	_, err := store.UpdateSharedProfile(ctx, repository.UpdateSharedProfileParams{
		ID:      userID,
		Profile: req,
	})
	return err
}

func (g *GlobalUserStrategy) DeleteUser(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, userId string) error {
	_, err := qtx.DeleteSharedUser(c, userId)
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
	return nil
}

func (g *GlobalUserStrategy) ListUsers(c *gin.Context, store *db.Store, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Query via user_tenant_memberships table
	adminUsers, err := store.ListSharedUsersByRoles(c, repository.ListSharedUsersByRolesParams{
		RequestedRoles: []string{"SUPER_ADMIN", "ADMIN"},
		Limit:          pagingSql.PageSize,
		Offset:         pagingSql.Offset,
		Like:           like,
	})
	if err != nil {
		return []core.User{}, err
	}

	// Convert memberships to users
	users := make([]core.User, len(adminUsers))
	for j, membership := range adminUsers {
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

func (g *GlobalUserStrategy) AssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	err := hasRightsForRole(c, role)
	if err != nil {
		return err
	}

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
	return nil
}

func (g *GlobalUserStrategy) UnAssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	err := hasRightsForRole(c, role)
	if err != nil {
		return err
	}

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
	return authClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
}
