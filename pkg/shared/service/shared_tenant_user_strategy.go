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

// TenantUserStrategy handles operations for tenant users
type TenantUserStrategy struct {
	tenantID string
}

func (g *TenantUserStrategy) Strategy() StrategyType {
	return StrategyTypeTenant
}

func (g *TenantUserStrategy) CreateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, userRecord *auth.UserRecord, req core.NewUser, password *string) (repository.CoreUser, error) {
	claims := map[string]interface{}{}
	// For tenant-scoped users, add tenant_memberships to metadata_public which includes tenant_id and assigned roles
	claims["tenant_memberships"] = map[string]interface{}{
		"tenant_id": g.tenantID,
		"roles":     req.Roles,
	}
	user := repository.CoreUser{}
	err := authClient.SetCustomUserClaims(c, userRecord.UID, claims)
	if err != nil {
		return user, err
	}

	profile := subentity.UserProfile{
		Name: req.Name,
	}

	sharedUser, err := qtx.CreateSharedUserWithTenant(c,
		repository.CreateSharedUserWithTenantParams{
			ID:          userRecord.UID,
			Email:       req.Email,
			Profile:     profile,
			TenantRoles: convertToRoles(req.Roles),
			TenantID:    g.tenantID,
		})
	if err != nil {
		return user, err
	}
	user.CreatedAt = sharedUser.CreatedAt
	user.Email = sharedUser.Email
	user.ID = sharedUser.ID
	user.Profile = profile
	user.Roles = sharedUser.TenantRoles

	return user, err
}

func (g *TenantUserStrategy) UpdateUser(c context.Context, authClient auth.AuthClient, qtx *repository.Queries, req core.UpdateUserJSONRequestBody) error {
	claims := map[string]interface{}{}
	// For tenant-scoped users, add tenant_memberships to metadata_public which includes tenant_id and assigned roles
	claims["tenant_memberships"] = map[string]interface{}{
		"tenant_id": g.tenantID,
		"roles":     req.Roles,
	}
	err := authClient.SetCustomUserClaims(c, req.Id, claims)
	if err != nil {
		return err
	}
	_, err = qtx.UpdateSharedUserByTenant(c,
		repository.UpdateSharedUserByTenantParams{
			ID:          req.Id,
			Name:        req.Name,
			TenantRoles: convertToRoles(req.Roles),
			TenantID:    g.tenantID,
		})
	if err != nil {
		return err
	}
	return err
}
func (g *TenantUserStrategy) UpdateSharedProfile(ctx context.Context, store *db.Store, userID string, req subentity.UserProfile) error {
	_, err := store.UpdateSharedProfile(ctx, repository.UpdateSharedProfileParams{
		ID:      userID,
		Profile: req,
	})
	return err
}

func (g *TenantUserStrategy) ListUsers(c *gin.Context, store *db.Store, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	// Query via user_tenant_memberships table
	memberships, err := store.ListSharedUsersByTenant(c, repository.ListSharedUsersByTenantParams{
		TenantID:     g.tenantID,
		Limit:        pagingSql.PageSize,
		Offset:       pagingSql.Offset,
		SearchPrefix: like,
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
			Roles:     convertToRoleDTOs(membership.TenantRoles),
			CreatedAt: &membership.CreatedAt,
		}
		users[j] = user
	}

	return users, nil
}

func (g *TenantUserStrategy) AssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	err := HasRightsForRole(c, role)
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

func (g *TenantUserStrategy) UnAssignRole(qtx *repository.Queries, c *gin.Context, authClient auth.AuthClient, tenantId string, userID string, role core.Role) error {
	err := HasRightsForRole(c, role)
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
