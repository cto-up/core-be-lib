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
)

// GlobalUserStrategy handles operations for global users
type GlobalUserStrategy struct{}

func (g *GlobalUserStrategy) Strategy() StrategyType {
	return StrategyTypeGlobal
}

func (g *GlobalUserStrategy) CreateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, userRecord *auth.UserRecord, req core.NewUser, password *string) (repository.CoreUser, error) {
	user := repository.CoreUser{}
	// Use the provider-specific format (Kratos: metadata_public.global_roles)
	// so the session loader actually picks these roles up. Top-level boolean
	// keys are ignored by KratosAuthClient.SetCustomUserClaims.
	if len(req.Roles) > 0 {
		claims := authClient.BuildGlobalRoleClaims(convertToRoles(req.Roles))
		if err := authClient.SetCustomUserClaims(c, userRecord.UID, claims); err != nil {
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
	params := (&auth.UserToUpdate{}).
		Email(req.Email).
		EmailVerified(false).
		DisplayName(req.Name).
		PhotoURL("/images/avatar-1.jpeg").
		Disabled(false)

	if _, err := authClient.UpdateUser(c, req.Id, params); err != nil {
		return err
	}

	// Mirror the requested role set into Kratos via the provider-specific
	// global_roles claim (top-level booleans are silently dropped by Kratos).
	claims := authClient.BuildGlobalRoleClaims(convertToRoles(req.Roles))
	if err := authClient.SetCustomUserClaims(c, req.Id, claims); err != nil {
		return err
	}

	_, err := qtx.UpdateSharedUser(c,
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

func (g *GlobalUserStrategy) ListUsers(c *gin.Context, store *db.Store, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Query via user_tenant_memberships table
	adminUsers, err := store.ListSharedUsersByRoles(c, repository.ListSharedUsersByRolesParams{
		RequestedRoles: []string{string(core.SUPERADMIN), string(core.ADMIN)},
		Limit:          pagingSql.PageSize,
		Offset:         pagingSql.Offset,
		SearchPrefix:   like,
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

// AssignRole grants a global role. core_users.roles is the source of truth: we
// read it, merge the new role in, write it back, then mirror the full set into
// Kratos via global_roles. The previous AssignRoleWithRowsAffected path filtered
// by tenant_id and silently no-op'd for global users (tenant_id NULL/empty),
// which let core_users.roles and Kratos drift.
func (g *GlobalUserStrategy) AssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	if err := auth.HasRightsForRole(c, role); err != nil {
		return err
	}

	current, err := qtx.GetSharedUserByID(c, userID)
	if err != nil {
		return err
	}
	merged := mergeRoleStrings(current.Roles, []string{string(role)})
	if _, err := qtx.UpdateSharedUserGlobalRoles(c, repository.UpdateSharedUserGlobalRolesParams{
		ID:    userID,
		Roles: merged,
	}); err != nil {
		return err
	}

	claims := authClient.BuildGlobalRoleClaims(merged)
	return authClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
}

// UnAssignRole removes a global role. Same source-of-truth flow as AssignRole
// — operate on core_users.roles, then push the pruned set to Kratos.
func (g *GlobalUserStrategy) UnAssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	if err := auth.HasRightsForRole(c, role); err != nil {
		return err
	}

	current, err := qtx.GetSharedUserByID(c, userID)
	if err != nil {
		return err
	}
	pruned := make([]string, 0, len(current.Roles))
	for _, r := range current.Roles {
		if r != string(role) {
			pruned = append(pruned, r)
		}
	}
	if _, err := qtx.UpdateSharedUserGlobalRoles(c, repository.UpdateSharedUserGlobalRolesParams{
		ID:    userID,
		Roles: pruned,
	}); err != nil {
		return err
	}

	claims := authClient.BuildGlobalRoleClaims(pruned)
	return authClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
}
