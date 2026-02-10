package migration

import (
	"context"
	"fmt"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	fileservice "ctoup.com/coreapp/pkg/shared/fileservice"
)

// MigrationUserMapping represents the mapping between Firebase UID and Kratos UUID
type MigrationUserMapping struct {
	FirebaseUID string
	Email       string
	KratosUUID  string
	MigratedAt  time.Time
}

type ColInfo struct {
	Table  string
	Column string
	Type   string
}

// FirebaseToKratosMigrator handles the migration of users from Firebase to Kratos
type FirebaseToKratosMigrator struct {
	store       *db.Store
	authClient  auth.AuthClient
	fileService *fileservice.FileService
	ctx         context.Context
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
func NewFirebaseToKratosMigrator(ctx context.Context, store *db.Store, authClient auth.AuthClient, fileService *fileservice.FileService) *FirebaseToKratosMigrator {
	return &FirebaseToKratosMigrator{
		store:       store,
		authClient:  authClient,
		fileService: fileService,
		ctx:         ctx,
	}
}

// EnsureMigrationTable ensures that the migration mapping table exists in the database
func (m *FirebaseToKratosMigrator) EnsureMigrationTable() error {
	log.Info().Msg("Ensuring migration mapping table exists...")
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

	log.Info().Msgf("Created/Updated mapping: %s (%s) -> Kratos UUID %s", firebaseUID, email, kratosUUID)
	return nil
}

// migrateUser migrates a single user from Firebase to Kratos
func (m *FirebaseToKratosMigrator) migrateUser(user repository.CoreUser) error {
	// Check if user ID is a Firebase UID
	if !isFirebaseUID(user.ID) {
		log.Info().Msgf("Skipping user %s - not a Firebase UID", user.ID)
		return nil
	}

	// 1. Get or create Kratos identity (source of truth for UUID)
	var kratosUUID string
	if m.authClient != nil {
		kratosUser, err := m.authClient.GetUserByEmail(m.ctx, user.Email.String)
		if err != nil {
			// If user not found, create it
			if authErr, ok := err.(*auth.AuthError); ok && authErr.Code == auth.ErrorCodeUserNotFound {
				log.Info().Msgf("Kratos identity not found for %s, creating...", user.Email.String)

				userToCreate := (&auth.UserToCreate{}).
					Email(user.Email.String)

				kratosUser, err = m.authClient.CreateUser(m.ctx, userToCreate)
				if err != nil {
					return fmt.Errorf("failed to create Kratos user: %w", err)
				}
				log.Info().Msgf("Successfully created Kratos identity: %s -> %s", user.Email.String, kratosUser.UID)
			} else {
				return fmt.Errorf("failed to check Kratos identity: %w", err)
			}
		} else {
			log.Info().Msgf("Kratos identity already exists for %s: %s", user.Email.String, kratosUser.UID)
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
		log.Info().Msgf("User already exists in core_users with UID %s, skipping DB creation", kratosUUID)
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

		log.Info().Msgf("Created user with tenant membership: %s (tenant: %s, roles: %v)",
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

		log.Info().Msgf("Added tenant membership to existing user: %s (tenant: %s, roles: %v)",
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

		log.Info().Msgf("Created global user: %s (roles: %v)", kratosUUID, globalRoles)
	}

	// Commit transaction
	if err := tx.Commit(m.ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Info().Msgf("Successfully migrated user: %s -> %s", user.ID, kratosUUID)
	return nil
}

// MigrateAllUsers migrates all Firebase users to Kratos
func (m *FirebaseToKratosMigrator) MigrateAllUsers() error {
	log.Info().Msg("Starting Firebase to Kratos user migration...")

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
			log.Info().Msgf("Error scanning user: %v", err)
			errorCount++
			continue
		}

		if !isFirebaseUID(user.ID) {
			skipCount++
			continue
		}

		err = m.migrateUser(user)
		if err != nil {
			log.Info().Msgf("Error migrating user %s in tenant %s: %v", user.ID, user.TenantID.String, err)
			errorCount++
			continue
		}

		successCount++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating users: %w", err)
	}

	log.Info().Msgf("\nMigration complete!")
	log.Info().Msgf("Successfully migrated: %d users", successCount)
	log.Info().Msgf("Skipped (non-Firebase): %d users", skipCount)
	log.Info().Msgf("Errors: %d users", errorCount)

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

// MigrateForeignKeys finds all columns that likely contain user IDs and updates them to use Kratos UUIDs
func (m *FirebaseToKratosMigrator) MigrateForeignKeys(columns []ColInfo) error {
	log.Info().Msg("Starting granular foreign key migration...")

	// 1. Build a local cache of mappings
	mappings, err := m.ListMappings()
	if err != nil {
		log.Err(err).Msg("Failed to list mappings")
		return fmt.Errorf("failed to list mappings: %w", err)
	}
	mappingCache := make(map[string]string)
	for _, mapping := range mappings {
		mappingCache[mapping.FirebaseUID] = mapping.KratosUUID
	}

	// 2. Iterate over columns
	for _, ci := range columns {
		log.Info().Msgf("Processing %s.%s (type: %s)...", ci.Table, ci.Column, ci.Type)

		// We fetch all potential tuples in one go to free the connection for updates
		query := fmt.Sprintf("SELECT ctid::text, %s FROM %s", ci.Column, ci.Table)
		rows, err := m.store.ConnPool.Query(m.ctx, query)
		if err != nil {
			log.Err(err).Msgf("Error querying %s.%s", ci.Table, ci.Column)
			continue
		}

		// Collect rows into memory to allow updating on the same connection pool
		type rowData struct {
			ctid   string
			val    pgtype.Text
			arrVal []string
		}
		var results []rowData
		for rows.Next() {
			var r rowData
			var err error
			if ci.Type == "array" {
				err = rows.Scan(&r.ctid, &r.arrVal)
			} else {
				err = rows.Scan(&r.ctid, &r.val)
			}
			if err != nil {
				log.Err(err).Msgf("Error scanning row in %s.%s", ci.Table, ci.Column)
				continue
			}
			results = append(results, r)
		}
		rows.Close()

		// 3. Process each tuple
		successCount := 0
		for _, r := range results {
			if ci.Type == "array" {
				changed := false
				newArr := make([]string, len(r.arrVal))
				copy(newArr, r.arrVal)

				for i, v := range newArr {
					if isFirebaseUID(v) {
						if kratosUUID, found := mappingCache[v]; found {
							newArr[i] = kratosUUID
							changed = true
						}
					}
				}

				if changed {
					updateSQL := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE ctid = $2", ci.Table, ci.Column)
					_, err := m.store.ConnPool.Exec(m.ctx, updateSQL, newArr, r.ctid)
					if err != nil {
						log.Err(err).Msgf("Error updating array row %s", r.ctid)
						continue
					}
					successCount++
				}
			} else {
				// Scalar field
				if !r.val.Valid || !isFirebaseUID(r.val.String) {
					continue
				}

				kratosUUID, found := mappingCache[r.val.String]
				if !found {
					continue
				}

				updateSQL := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE ctid = $2", ci.Table, ci.Column)
				_, err := m.store.ConnPool.Exec(m.ctx, updateSQL, kratosUUID, r.ctid)
				if err != nil {
					log.Err(err).Msgf("Error updating row %s", r.ctid)
					continue
				}
				successCount++
			}
		}

		if successCount > 0 {
			log.Info().Msgf("Successfully migrated %d/%d rows in %s.%s", successCount, len(results), ci.Table, ci.Column)
		}
	}

	log.Info().Msg("Foreign key migration complete!")
	return nil
}

// MigrateUserFiles migrates user profile pictures from Firebase UIDs to Kratos UUIDs
func (m *FirebaseToKratosMigrator) MigrateUserFiles() error {
	if m.fileService == nil {
		log.Warn().Msg("FileService not initialized, skipping file migration")
		return nil
	}

	log.Info().Msg("Starting user files migration...")

	// Query mappings joined with core_users to get tenant IDs
	// We look for users who have a tenant_id and whose old ID (firebase_uid) is in the core_users table
	query := `
		SELECT m.firebase_uid, m.kratos_uuid, u.tenant_id 
		FROM migration_user_mapping m
		JOIN core_users u ON m.firebase_uid = u.id
	`

	rows, err := m.store.ConnPool.Query(m.ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query user mappings for file migration: %w", err)
	}
	defer rows.Close()

	type fileMigrationTask struct {
		firebaseUID string
		kratosUUID  string
		tenantID    string
	}
	var tasks []fileMigrationTask
	for rows.Next() {
		var t fileMigrationTask
		if err := rows.Scan(&t.firebaseUID, &t.kratosUUID, &t.tenantID); err != nil {
			log.Err(err).Msg("Error scanning file migration task")
			continue
		}
		if t.tenantID == "" {
			t.tenantID = "www"
		}
		tasks = append(tasks, t)
	}
	rows.Close()

	log.Info().Msgf("Found %d users with potential files to migrate", len(tasks))

	successCount := 0
	skipCount := 0
	errorCount := 0

	for _, t := range tasks {
		// Path format: /tenants/ + tenantPart + /core/users/ + userId + "/profile-picture.jpg"
		// The path usually starts without a leading slash in many blob storages,
		// but let's follow the user's format exactly or adjust as needed for service.

		oldPath := fmt.Sprintf("tenants/%s/core/users/%s/profile-picture.jpg", t.tenantID, t.firebaseUID)
		backupPath := fmt.Sprintf("tenants/%s/backup/core/users/%s/profile-picture.jpg", t.tenantID, t.firebaseUID)
		newPath := fmt.Sprintf("core/users/%s/profile-picture.jpg", t.kratosUUID)

		// Check if old file exists
		exists, err := m.fileService.FileExists(m.ctx, oldPath)
		if err != nil {
			log.Err(err).Msgf("Error checking existence of %s", oldPath)
			errorCount++
			continue
		}

		if !exists {
			skipCount++
			continue
		}

		log.Info().Msgf("Migrating profile picture: %s -> %s", oldPath, newPath)

		// Copy backup before delete (if supported by the service, otherwise this is a rename)
		if err := m.fileService.CopyFile(m.ctx, backupPath, oldPath); err != nil {
			log.Err(err).Msgf("Failed to copy file from %s to %s", oldPath, backupPath)
			errorCount++
			continue
		}

		// Rename (Copy + Delete)
		if err := m.fileService.RenameFile(m.ctx, newPath, oldPath); err != nil {
			log.Err(err).Msgf("Failed to migrate file from %s to %s", oldPath, newPath)
			errorCount++
			continue
		}

		successCount++
	}

	log.Info().Msgf("File migration complete: %d migrated, %d skipped, %d errors", successCount, skipCount, errorCount)
	return nil
}
