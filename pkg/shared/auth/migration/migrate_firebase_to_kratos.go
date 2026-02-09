package migration

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/auth/kratos"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	ory "github.com/ory/kratos-client-go"
)

// MigrationUserMapping represents the mapping between Firebase UID and Kratos UUID
type MigrationUserMapping struct {
	FirebaseUID string
	Email       string
	KratosUUID  string
	MigratedAt  time.Time
}

// FirebaseToKratosMigrator handles the migration of users from Firebase to Kratos
type FirebaseToKratosMigrator struct {
	store      *db.Store
	authClient auth.AuthClient
	ctx        context.Context
}

const migrationMappingSQL = `
CREATE TABLE IF NOT EXISTS migration_user_mapping (
    firebase_uid VARCHAR(128) PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    kratos_uuid VARCHAR(128) NOT NULL,
    migrated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
`

// NewFirebaseToKratosMigrator creates a new migrator instance
func NewFirebaseToKratosMigrator(ctx context.Context, store *db.Store, authClient auth.AuthClient) *FirebaseToKratosMigrator {
	return &FirebaseToKratosMigrator{
		store:      store,
		authClient: authClient,
		ctx:        ctx,
	}
}

// EnsureMigrationTable ensures that the migration mapping table exists in the database
func (m *FirebaseToKratosMigrator) EnsureMigrationTable() error {
	log.Println("Ensuring migration mapping table exists...")
	_, err := m.store.ConnPool.Exec(m.ctx, migrationMappingSQL)
	if err != nil {
		return fmt.Errorf("failed to create migration mapping table: %w", err)
	}
	return nil
}

// isFirebaseUID checks if the ID is a Firebase UID (28 characters alphanumeric)
func isFirebaseUID(id string) bool {
	if len(id) != 28 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// createMapping creates a new mapping record
func (m *FirebaseToKratosMigrator) createMapping(tx pgx.Tx, firebaseUID string, email string, kratosUUID string) error {
	_, err := tx.Exec(m.ctx,
		`INSERT INTO migration_user_mapping (firebase_uid, email, kratos_uuid) 
		 VALUES ($1, $2, $3) 
		 ON CONFLICT (firebase_uid) DO UPDATE SET kratos_uuid = EXCLUDED.kratos_uuid, email = EXCLUDED.email`,
		firebaseUID, email, kratosUUID)

	if err != nil {
		return fmt.Errorf("failed to create mapping: %w", err)
	}

	log.Printf("Created/Updated mapping: %s (%s) -> Kratos UUID %s", firebaseUID, email, kratosUUID)
	return nil
}

// migrateUser migrates a single user from Firebase to Kratos
func (m *FirebaseToKratosMigrator) migrateUser(user repository.CoreUser) error {
	// Check if user ID is a Firebase UID
	if !isFirebaseUID(user.ID) {
		log.Printf("Skipping user %s - not a Firebase UID", user.ID)
		return nil
	}

	// 1. Get or create Kratos identity (source of truth for UUID)
	var kratosUUID string
	if m.authClient != nil {
		kratosUser, err := m.authClient.GetUserByEmail(m.ctx, user.Email.String)
		if err != nil {
			// If user not found, create it
			if authErr, ok := err.(*auth.AuthError); ok && authErr.Code == auth.ErrorCodeUserNotFound {
				log.Printf("Kratos identity not found for %s, creating...", user.Email.String)

				userToCreate := (&auth.UserToCreate{}).
					Email(user.Email.String)

				kratosUser, err = m.authClient.CreateUser(m.ctx, userToCreate)
				if err != nil {
					return fmt.Errorf("failed to create Kratos user: %w", err)
				}
				log.Printf("Successfully created Kratos identity: %s -> %s", user.Email.String, kratosUser.UID)
			} else {
				return fmt.Errorf("failed to check Kratos identity: %w", err)
			}
		} else {
			log.Printf("Kratos identity already exists for %s: %s", user.Email.String, kratosUser.UID)
		}
		kratosUUID = kratosUser.UID
	} else {
		return fmt.Errorf("authClient is required for migration")
	}

	// 2. Start transaction for DB updates
	tx, err := m.store.ConnPool.Begin(m.ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(m.ctx)

	// 3. Create or Update mapping
	err = m.createMapping(tx, user.ID, user.Email.String, kratosUUID)
	if err != nil {
		return fmt.Errorf("failed to update mapping: %w", err)
	}

	qtx := m.store.Queries.WithTx(tx)

	// Determine if user has tenant or is global
	hasTenant := user.TenantID.Valid && user.TenantID.String != ""

	// 4. Check if user already exists in core_users
	existingUser := false
	_, err = qtx.GetSharedUserByID(m.ctx, kratosUUID)
	if err == nil {
		log.Printf("User already exists in core_users with UID %s, skipping DB creation", kratosUUID)
		existingUser = true
	}

	if hasTenant && !existingUser {
		// User belongs to a tenant - create with tenant membership
		tenantRoles := user.Roles
		if len(tenantRoles) == 0 {
			tenantRoles = []string{"USER"} // Default role
		}

		_, err = qtx.CreateSharedUserWithTenant(m.ctx, repository.CreateSharedUserWithTenantParams{
			ID:          kratosUUID,
			Email:       user.Email.String,
			Profile:     user.Profile,
			TenantID:    user.TenantID.String,
			TenantRoles: tenantRoles,
			InvitedBy:   pgtype.Text{String: "migration", Valid: true},
			InvitedAt: pgtype.Timestamptz{
				Time:  time.Now(),
				Valid: true,
			},
		})

		if err != nil {
			return fmt.Errorf("failed to create user with tenant: %w", err)
		}

		log.Printf("Created user with tenant membership: %s (tenant: %s, roles: %v)",
			kratosUUID, user.TenantID.String, tenantRoles)
	}
	if hasTenant && existingUser {
		// Add tenant membership to existing user
		tenantRoles := user.Roles
		if len(tenantRoles) == 0 {
			tenantRoles = []string{"USER"} // Default role
		}

		_, err = qtx.AddSharedUserToTenant(m.ctx, repository.AddSharedUserToTenantParams{
			UserID:      kratosUUID,
			TenantID:    user.TenantID.String,
			TenantRoles: tenantRoles,
			Status:      "active",
			InvitedBy:   pgtype.Text{String: "migration", Valid: true},
			InvitedAt: pgtype.Timestamptz{
				Time:  time.Now(),
				Valid: true,
			},
		})

		if err != nil {
			return fmt.Errorf("failed to add tenant membership to existing user: %w", err)
		}

		log.Printf("Added tenant membership to existing user: %s (tenant: %s, roles: %v)",
			kratosUUID, user.TenantID.String, tenantRoles)
	}

	if !hasTenant && !existingUser {
		// Global user - create without tenant
		globalRoles := user.Roles
		if len(globalRoles) == 0 {
			globalRoles = []string{}
		}

		_, err = qtx.CreateSharedUser(m.ctx, repository.CreateSharedUserParams{
			ID:      kratosUUID,
			Email:   user.Email.String,
			Profile: user.Profile,
			Roles:   globalRoles,
		})

		if err != nil {
			return fmt.Errorf("failed to create global user: %w", err)
		}

		log.Printf("Created global user: %s (roles: %v)", kratosUUID, globalRoles)
	}

	// Commit transaction
	if err := tx.Commit(m.ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully migrated user: %s -> %s", user.ID, kratosUUID)
	return nil
}

// MigrateAllUsers migrates all Firebase users to Kratos
func (m *FirebaseToKratosMigrator) MigrateAllUsers() error {
	log.Println("Starting Firebase to Kratos user migration...")

	// Get all users
	rows, err := m.store.ConnPool.Query(m.ctx,
		"SELECT id, profile, email, created_at, tenant_id, roles FROM core_users ORDER BY created_at")
	if err != nil {
		return fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	successCount := 0
	skipCount := 0
	errorCount := 0

	for rows.Next() {
		var user repository.CoreUser
		err := rows.Scan(
			&user.ID,
			&user.Profile,
			&user.Email,
			&user.CreatedAt,
			&user.TenantID,
			&user.Roles,
		)
		if err != nil {
			log.Printf("Error scanning user: %v", err)
			errorCount++
			continue
		}

		if !isFirebaseUID(user.ID) {
			skipCount++
			continue
		}

		err = m.migrateUser(user)
		if err != nil {
			log.Printf("Error migrating user %s in tenant %s: %v", user.ID, user.TenantID.String, err)
			errorCount++
			continue
		}

		successCount++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating users: %w", err)
	}

	log.Printf("\nMigration complete!")
	log.Printf("Successfully migrated: %d users", successCount)
	log.Printf("Skipped (non-Firebase): %d users", skipCount)
	log.Printf("Errors: %d users", errorCount)

	return nil
}

// MigrateSingleUser migrates a specific user by Firebase UID
func (m *FirebaseToKratosMigrator) MigrateSingleUser(firebaseUID string) error {
	if !isFirebaseUID(firebaseUID) {
		return fmt.Errorf("invalid Firebase UID: %s", firebaseUID)
	}

	user, err := m.store.GetSharedUserByID(m.ctx, firebaseUID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	return m.migrateUser(user)
}

// GetMapping retrieves the Kratos UUID for a given Firebase UID or Email
func (m *FirebaseToKratosMigrator) GetMapping(identifier string) (string, error) {
	var kratosUUID string
	err := m.store.ConnPool.QueryRow(m.ctx,
		"SELECT kratos_uuid FROM migration_user_mapping WHERE firebase_uid = $1 OR email = $1",
		identifier).Scan(&kratosUUID)

	if err != nil {
		return "", fmt.Errorf("mapping not found: %w", err)
	}

	return kratosUUID, nil
}

// ListMappings lists all migration mappings
func (m *FirebaseToKratosMigrator) ListMappings() ([]MigrationUserMapping, error) {
	rows, err := m.store.ConnPool.Query(m.ctx,
		"SELECT firebase_uid, email, kratos_uuid, migrated_at FROM migration_user_mapping ORDER BY migrated_at DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to query mappings: %w", err)
	}
	defer rows.Close()

	var mappings []MigrationUserMapping
	for rows.Next() {
		var mapping MigrationUserMapping
		err := rows.Scan(&mapping.FirebaseUID, &mapping.Email, &mapping.KratosUUID, &mapping.MigratedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mapping: %w", err)
		}
		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

func main() {
	// Example usage
	ctx := context.Background()

	// Initialize database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	connPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer connPool.Close()

	store := db.NewStore(connPool)

	// Initialize auth client
	adminURL := os.Getenv("KRATOS_ADMIN_URL")
	if adminURL == "" {
		adminURL = "http://localhost:4434"
	}
	publicURL := os.Getenv("KRATOS_PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://localhost:4433"
	}

	adminCfg := ory.NewConfiguration()
	adminCfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
	adminClient := ory.NewAPIClient(adminCfg)

	publicCfg := ory.NewConfiguration()
	publicCfg.Servers = ory.ServerConfigurations{{URL: publicURL}}
	publicClient := ory.NewAPIClient(publicCfg)

	authClient := kratos.NewKratosAuthClient(adminClient, publicClient)

	// Create migrator
	migrator := NewFirebaseToKratosMigrator(ctx, store, authClient)

	// Ensure migration table exists
	if err := migrator.EnsureMigrationTable(); err != nil {
		log.Fatalf("Failed to ensure migration table: %v", err)
	}

	// Run migration
	if len(os.Args) > 1 {
		command := os.Args[1]
		switch command {
		case "migrate-all":
			if err := migrator.MigrateAllUsers(); err != nil {
				log.Fatalf("Migration failed: %v", err)
			}
		case "migrate-user":
			if len(os.Args) < 3 {
				log.Fatal("Usage: migrate-user <firebase_uid>")
			}
			firebaseUID := os.Args[2]
			if err := migrator.MigrateSingleUser(firebaseUID); err != nil {
				log.Fatalf("Migration failed: %v", err)
			}
		case "get-mapping":
			if len(os.Args) < 3 {
				log.Fatal("Usage: get-mapping <uid_or_email>")
			}
			identifier := os.Args[2]
			kratosUUID, err := migrator.GetMapping(identifier)
			if err != nil {
				log.Fatalf("Failed to get mapping: %v", err)
			}
			fmt.Printf("Identifier: %s -> Kratos UUID: %s\n", identifier, kratosUUID)
		case "list-mappings":
			mappings, err := migrator.ListMappings()
			if err != nil {
				log.Fatalf("Failed to list mappings: %v", err)
			}
			fmt.Printf("Total mappings: %d\n\n", len(mappings))
			for _, m := range mappings {
				fmt.Printf("%s (%s) -> %s (migrated at: %s)\n",
					m.FirebaseUID, m.Email, m.KratosUUID, m.MigratedAt.Format(time.RFC3339))
			}
		default:
			log.Fatalf("Unknown command: %s\nAvailable commands: migrate-all, migrate-user, get-mapping, list-mappings", command)
		}
	} else {
		log.Println("Usage:")
		log.Println("  migrate-all              - Migrate all Firebase users")
		log.Println("  migrate-user <uid>       - Migrate a specific user")
		log.Println("  get-mapping <uid>        - Get Kratos UUID for Firebase UID")
		log.Println("  list-mappings            - List all migration mappings")
	}
}
