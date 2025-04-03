package seedservice

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"firebase.google.com/go/auth"
)

type SeedService struct {
	store  *db.Store
	client *auth.Client
}

func NewSeedService(store *db.Store, pool *access.FirebaseTenantClientConnectionPool) *SeedService {
	return &SeedService{
		store:  store,
		client: pool.GetClient(),
	}
}

func (ss *SeedService) userExists(email string) (bool, error) {
	_, err := ss.client.GetUserByEmail(context.Background(), email)
	if err != nil {
		if auth.IsUserNotFound(err) {
			// User not found, we can create them
			return false, nil
		}
		// Other error occurred
		return false, fmt.Errorf("error checking if user exists: %v", err)
	}
	// User found
	return true, nil
}

func (ss *SeedService) Seed() error {
	userEmail := os.Getenv("SEED_USER_EMAIL")
	if userEmail == "" {
		fmt.Println("No SEED_USER_EMAIL")
		return nil
	}

	userPassword := os.Getenv("SEED_USER_PASSWORD")

	if userPassword == "" {
		fmt.Println("No SEED_USER_PASSWORD")
		return nil
	}

	c := context.Background()

	tx, err := ss.store.ConnPool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	qtx := ss.store.Queries.WithTx(tx)

	user, err := ss.seedAdminUser(c, qtx, userEmail, userPassword)
	if err != nil {
		return err
	}

	roles, err := ss.seedRoles(c, qtx, user.ID)
	if err != nil {
		return err
	}
	err = ss.seedAdminUserRoles(c, qtx, user.ID, roles)
	if err != nil {
		return err
	}
	err = tx.Commit(c)

	return err
}

func (ss *SeedService) testSeedRoles(c context.Context, queries *repository.Queries, userID string) error {
	userEmail := os.Getenv("SEED_USER_EMAIL")
	user, err := ss.client.GetUserByEmail(c, userEmail)

	if err != nil {
		return err
	}

	var claims map[string]interface{}
	if user.CustomClaims == nil {
		claims = map[string]interface{}{}
	} else {
		claims = user.CustomClaims
	}

	claims["SUPER_ADMIN"] = true

	return ss.client.SetCustomUserClaims(c, userID, claims)
}

func (ss *SeedService) seedRoles(c context.Context, queries *repository.Queries, userID string) ([]repository.CoreRole, error) {
	rolesString := []string{"USER", "ADMIN", "SUPER_ADMIN"}

	roles := make([]repository.CoreRole, len(rolesString))

	for _, roleString := range rolesString {
		role, err := queries.CreateRole(c, repository.CreateRoleParams{
			Name:   roleString,
			UserID: userID,
		})
		if err != nil {
			return roles, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (ss *SeedService) seedAdminUser(c context.Context, qtx *repository.Queries, adminEmail, adminPassword string) (repository.CoreUser, error) {
	user := repository.CoreUser{}
	adminName := "Admin User"
	// Check if admin user exists
	exists, err := ss.userExists(adminEmail)
	if err != nil {
		log.Fatal().Err(err).Msg("Error checking if admin user exists")
	}

	if exists {
		fmt.Println("Admin user already exists.")
		return user, errors.New("admin user already exists")
	} else {

		params := (&auth.UserToCreate{}).
			Email(adminEmail).
			EmailVerified(false).
			Password(adminPassword).
			DisplayName(adminName).
			PhotoURL("/images/avatar-1.jpeg").
			Disabled(false)

		userRecord, err := ss.client.CreateUser(c, params)
		if err != nil {
			return user, err
		}

		return qtx.CreateUser(c, repository.CreateUserParams{
			ID:    userRecord.UID,
			Email: adminEmail,
			Profile: subentity.UserProfile{
				Name: adminName,
			},
		})
	}
}
func (ss *SeedService) seedAdminUserRoles(c context.Context, qtx *repository.Queries, userID string, roles []repository.CoreRole) error {

	// Lookup the user associated with the specified uid.
	user, err := ss.client.GetUser(c, userID)
	if err != nil {
		return err
	}

	var claims map[string]interface{}
	if user.CustomClaims == nil {
		claims = map[string]interface{}{}
	} else {
		claims = user.CustomClaims
	}

	for _, role := range roles {
		claims[role.Name] = true

		_, err = qtx.UpdateUserAddRole(c, repository.UpdateUserAddRoleParams{
			ID:       userID,
			Role:     role.ID,
			TenantID: "",
		})
		if err != nil {
			return err
		}
	}

	err = ss.client.SetCustomUserClaims(c, userID, claims)
	return err

}
