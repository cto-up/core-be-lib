package seedservice

import (
	"context"
	"fmt"
	"os"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/rs/zerolog/log"
)

type SeedService struct {
	store  *db.Store
	client auth.AuthClient
}

func NewSeedService(store *db.Store, authProvider auth.AuthProvider) *SeedService {
	return &SeedService{
		store:  store,
		client: authProvider.GetAuthClient(),
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

	err = ss.seedAdminUser(c, qtx, userEmail, userPassword)
	if err != nil {
		log.Err(err).Msg("Error seeding admin user")
		return err
	}

	err = tx.Commit(c)

	return err
}

func (ss *SeedService) seedAdminUser(c context.Context, qtx *repository.Queries, adminEmail, adminPassword string) error {
	adminName := "Admin User"
	// Check if admin user exists
	exists, err := ss.userExists(adminEmail)
	if err != nil {
		log.Err(err).Msg("Error checking if admin user exists")
	}

	if exists {
		fmt.Println("Admin user already exists.")
		return nil
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
			log.Err(err).Msg("Error creating admin user")
			return err
		}

		// Set global roles using provider-specific format
		// Kratos: {"global_roles": ["SUPER_ADMIN", "ADMIN"]}
		claims := ss.client.BuildGlobalRoleClaims([]string{string(core.SUPERADMIN), string(core.ADMIN)})
		err = ss.client.SetCustomUserClaims(c, userRecord.UID, claims)

		if err != nil {
			log.Err(err).Msg("Error setting custom user claims")
			return err
		}

		_, err = qtx.CreateUserByTenant(c, repository.CreateUserByTenantParams{
			ID:    userRecord.UID,
			Email: adminEmail,
			Profile: subentity.UserProfile{
				Name: adminName,
			},
			Roles: []string{string(core.SUPERADMIN), string(core.ADMIN)},
		})
		if err != nil {
			log.Err(err).Msg("Error creating user in database")
			return err
		}

		fmt.Println("Admin user created successfully.")
		return nil
	}
}
