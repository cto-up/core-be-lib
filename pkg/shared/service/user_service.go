package service

import (
	"context"
	"encoding/json"
	"errors"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	"github.com/google/uuid"
)

type FullUser struct {
	Disabled      bool   `json:"disabled"`
	EmailVerified bool   `json:"email_verified"`
	Email         string `json:"email"`
	core.User
}

type UserService struct {
	store          *db.Store
	authClientPool *FirebaseTenantClientConnectionPool
}

func IsAdmin(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isAdmin := claims.((map[string]interface{}))["ADMIN"] == true
	return isAdmin
}
func IsSuperAdmin(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isAdmin := claims.((map[string]interface{}))["SUPER_ADMIN"] == true
	return isAdmin
}

func NewUserService(store *db.Store, authClientPool *FirebaseTenantClientConnectionPool) *UserService {
	userService := &UserService{store: store,
		authClientPool: authClientPool}
	return userService
}

func (uh *UserService) AddUser(c context.Context, baseAuthClient BaseAuthClient, tenantId string, req core.AddUserJSONRequestBody) (repository.CoreUser, error) {
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
		//PhoneNumber("+15555550100").
		Password(req.Password).
		DisplayName(req.Name).
		PhotoURL("/images/avatar-1.jpeg").
		Disabled(false)

	userRecord, err := baseAuthClient.CreateUser(c, params)
	if err != nil {
		return user, err
	}
	user, err = qtx.CreateUser(c,
		repository.CreateUserParams{
			ID:    userRecord.UID,
			Email: req.Email,
			Profile: subentity.UserProfile{
				Name: req.Name,
			},
			TenantID: tenantId,
		})
	if err != nil {
		return user, err
	}
	err = tx.Commit(c)
	return user, err
}

func (uh *UserService) UpdateUser(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userId string, req core.UpdateUserJSONRequestBody) error {
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

	_, err = baseAuthClient.UpdateUser(c, userId, params)
	if err != nil {
		return err
	}
	_, err = qtx.UpdateUser(c, repository.UpdateUserParams{
		ID: userId,
		Email: pgtype.Text{String: req.Email,
			Valid: true},
		TenantID: tenantId,
	})
	if err != nil {
		return err
	}

	err = tx.Commit(c)

	return err
}

func (uh *UserService) DeleteUser(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userId string) error {
	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}

	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	err = baseAuthClient.DeleteUser(c, userId)
	if err != nil {
		return err
	}

	_, err = qtx.DeleteUser(c, repository.DeleteUserParams{
		ID:       userId,
		TenantID: tenantId,
	})
	if err != nil {
		return err
	}
	err = tx.Commit(c)

	return err
}

func (uh *UserService) GetUserByID(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, id string) (FullUser, error) {
	fullUser := FullUser{}
	dbUser, err := uh.store.GetUserRoleByID(c, repository.GetUserRoleByIDParams{
		ID:       id,
		TenantID: tenantId,
	})
	if err != nil {
		return fullUser, err
	}

	roles, err := unmarshalRolesFromDB(dbUser.CoreRoles)
	if err != nil {
		return fullUser, err
	}
	user := core.User{
		Id:        dbUser.ID,
		Name:      dbUser.Profile.Name,
		Email:     dbUser.Email.String,
		Roles:     &roles,
		CreatedAt: &dbUser.CreatedAt,
	}

	userFirebase, err := baseAuthClient.GetUser(c, id)
	if err != nil {
		return fullUser, err
	}
	return FullUser{
		Disabled:      userFirebase.Disabled,
		EmailVerified: userFirebase.EmailVerified,
		Email:         userFirebase.Email,
		User:          user,
	}, nil
}

func (uh *UserService) ListUsers(c *gin.Context, tenantId string, pagingSql sqlservice.PagingSQL, like pgtype.Text) ([]core.User, error) {
	dbUsers, err := uh.store.ListUsersRoles(c, repository.ListUsersRolesParams{
		Limit:    pagingSql.PageSize,
		Offset:   pagingSql.Offset,
		Like:     like,
		TenantID: tenantId,
	})

	if err != nil {
		return []core.User{}, err
	}

	// create a slice of users
	users := make([]core.User, len(dbUsers))
	for j, dbUser := range dbUsers {
		roles, err := unmarshalRolesFromDB(dbUser.CoreRoles)
		if err != nil {
			return users, err
		}
		user := core.User{
			Id:        dbUser.ID,
			Name:      dbUser.Profile.Name,
			Email:     dbUser.Email.String,
			Roles:     &roles,
			CreatedAt: &dbUser.CreatedAt,
		}
		users[j] = user
	}

	return users, nil
}

func unmarshalRolesFromDB(rolesBytes []byte) ([]core.Role, error) {
	var dbRoles []repository.CoreRole
	var roles []core.Role
	if rolesBytes != nil {
		if err := json.Unmarshal(rolesBytes, &dbRoles); err != nil {
			return []core.Role{}, err
		}
		roles = make([]core.Role, len(dbRoles))
		for i, dbRole := range dbRoles {
			roles[i] = core.Role{
				Id:   dbRole.ID,
				Name: dbRole.Name,
			}
		}
		return roles, nil
	}
	return []core.Role{}, nil
}

func (uh *UserService) AssignRole(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userID string, roleID uuid.UUID) error {
	if !IsAdmin(c) && !IsSuperAdmin(c) {
		return errors.New("must be an ADMIN or SUPER_ADMIN to perform such operation")
	}
	role, err := uh.store.GetRoleByID(c, roleID)
	if err != nil {
		return err
	}
	if role.Name == "SUPER_ADMIN" && !IsSuperAdmin(c) {
		return errors.New("must be an SUPER_ADMIN to perform such operation")
	}

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.UpdateUserAddRole(c, repository.UpdateUserAddRoleParams{
		ID:       userID,
		Role:     role.ID,
		TenantID: tenantId,
	})
	if err != nil {
		return err
	}

	// Lookup the user associated with the specified uid.
	user, err := baseAuthClient.GetUser(c, userID)
	if err != nil {
		return err
	}

	var claims map[string]interface{}
	if user.CustomClaims == nil {
		claims = map[string]interface{}{}
	} else {
		claims = user.CustomClaims
	}

	claims[role.Name] = true
	err = baseAuthClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
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

func (uh *UserService) UnassignRole(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userID string, roleID uuid.UUID) error {
	if !IsAdmin(c) && !IsSuperAdmin(c) {
		return errors.New("must be an ADMIN or SUPER_ADMIN to perform such operation")
	}
	role, err := uh.store.GetRoleByID(c, roleID)
	if err != nil {
		return err
	}
	if role.Name == "SUPER_ADMIN" && !IsSuperAdmin(c) {
		return errors.New("must be an SUPER_ADMIN to perform such operation")
	}
	tenant_id, exists := c.Get(AUTH_TENANT_ID_KEY)
	if !exists {
		return errors.New("user email not found in context")
	}

	tx, err := uh.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := uh.store.Queries.WithTx(tx)

	_, err = qtx.UpdateUserRemoveRole(c, repository.UpdateUserRemoveRoleParams{
		ID:       userID,
		Role:     role.ID,
		TenantID: tenant_id.(string),
	})
	if err != nil {
		return err
	}

	// Lookup the user associated with the specified uid.
	user, err := baseAuthClient.GetUser(c, userID)
	if err != nil {
		return err
	}

	claims := user.CustomClaims
	claims[role.Name] = false
	err = baseAuthClient.SetCustomUserClaims(c.Request.Context(), userID, claims)
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
func (uh *UserService) UpdateUserStatus(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userID string, requestName string, requestValue bool) error {
	// Lookup the user associated with the specified uid.
	params := (&auth.UserToUpdate{})
	if requestName == "EMAIL_VERIFIED" {
		params = params.EmailVerified(requestValue)
	} else if requestName == "DISABLED" {
		params = params.Disabled(requestValue)
	}
	_, err := baseAuthClient.UpdateUser(c, userID, params)
	return err
}
