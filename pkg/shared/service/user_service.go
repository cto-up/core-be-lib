package service

import (
	"context"
	"errors"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
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

func IsCustomerAdmin(c *gin.Context) bool {
	claims, exist := c.Get(AUTH_CLAIMS)
	if !exist {
		return false
	}
	isCustomerAdmin := claims.((map[string]interface{}))["CUSTOMER_ADMIN"] == true
	return isCustomerAdmin
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
	isSuperAdmin := claims.((map[string]interface{}))["SUPER_ADMIN"] == true
	return isSuperAdmin
}

func NewUserService(store *db.Store, authClientPool *FirebaseTenantClientConnectionPool) *UserService {
	userService := &UserService{store: store,
		authClientPool: authClientPool}
	return userService
}

func (uh *UserService) AddUser(c context.Context, baseAuthClient BaseAuthClient, tenantId string, req core.NewUser, password *string) (repository.CoreUser, error) {

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

	userRecord, err := baseAuthClient.CreateUser(c, params)
	if err != nil {
		return user, err
	}

	claims := map[string]interface{}{}
	for _, role := range req.Roles {
		claims[string(role)] = true
	}
	if len(req.Roles) > 0 {
		err = baseAuthClient.SetCustomUserClaims(c, userRecord.UID, claims)
		if err != nil {
			return user, err
		}
	}

	user, err = qtx.CreateUser(c,
		repository.CreateUserParams{
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

	claims := map[string]interface{}{}
	for _, role := range req.Roles {
		claims[string(role)] = true
	}
	err = baseAuthClient.SetCustomUserClaims(c, userId, claims)
	if err != nil {
		return err
	}
	// Display Name

	_, err = qtx.UpdateUser(c, repository.UpdateUserParams{
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

func convertToRoleDTOs(dbRoles []string) []core.Role {
	roles := make([]core.Role, len(dbRoles))
	for i, role := range dbRoles {
		roles[i] = core.Role(role)
	}
	return roles
}
func convertToRoles(roles []core.Role) []string {
	dbRoles := make([]string, len(roles))
	for i, role := range roles {
		dbRoles[i] = string(role)
	}
	return dbRoles
}

func (uh *UserService) GetUserByID(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, id string) (FullUser, error) {
	fullUser := FullUser{}
	dbUser, err := uh.store.GetUserByID(c, repository.GetUserByIDParams{
		ID:       id,
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
	dbUsers, err := uh.store.ListUsers(c, repository.ListUsersParams{
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

func (uh *UserService) AssignRole(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userID string, role core.Role) error {
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

	claims[string(role)] = true
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

func (uh *UserService) UnassignRole(c *gin.Context, baseAuthClient BaseAuthClient, tenantId string, userID string, role core.Role) error {
	if !IsAdmin(c) && !IsSuperAdmin(c) && !IsCustomerAdmin(c) {
		return errors.New("must be an CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN to perform such operation")
	}
	if role == "ADMIN" && (!IsAdmin(c) || !IsSuperAdmin(c)) {
		return errors.New("must be an CUSTOMER_ADMIN to perform such operation")
	}

	if role == "SUPER_ADMIN" && !IsSuperAdmin(c) {
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

	_, err = qtx.UnassignRoleWithRowsAffected(c, repository.UnassignRoleWithRowsAffectedParams{
		UserID:   userID,
		RoleName: string(role),
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
	claims[string(role)] = false
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
