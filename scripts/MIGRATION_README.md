# Firebase to Kratos User Migration Utility

This utility migrates users from Firebase authentication to Kratos, maintaining data integrity and user relationships.

## Migration Strategy

1. **Create Migration Mapping Table**: Tracks Firebase UID to Kratos UUID conversions
2. **Deterministic UUID Generation**: Each Firebase UID maps to the same Kratos UUID consistently
3. **Preserve User Data**: Maintains profiles, emails, roles, and tenant memberships
4. **Transaction Safety**: All operations wrapped in database transactions
5. **Idempotent**: Safe to run multiple times - skips already migrated users

## Prerequisites

- Database connection configured via `DATABASE_URL` environment variable
- Migration table created (run migration `000020_firebase_migration_mapping.up.sql`)
- Backup your database before running migration

## Usage

### 1. Run Database Migration

First, ensure the migration mapping table exists:

```bash
# Apply the migration
make migrate-up
# or manually run: pkg/core/db/migration/000020_firebase_migration_mapping.up.sql
```

### 2. Migrate All Users

```bash
export DATABASE_URL="postgresql://user:password@localhost:5432/dbname"
go run scripts/migrate_firebase_to_kratos.go migrate-all
```

This will:

- Scan all users in `core_users` table
- Identify Firebase UIDs (28-character alphanumeric strings)
- Create deterministic Kratos UUIDs
- Migrate user data with proper tenant memberships or global roles
- Create mapping entries for tracking

### 3. Migrate Single User

```bash
go run scripts/migrate_firebase_to_kratos.go migrate-user <firebase_uid>
```

Example:

```bash
go run scripts/migrate_firebase_to_kratos.go migrate-user abc123def456ghi789jkl012mnop
```

### 4. Get Mapping for a User

```bash
go run scripts/migrate_firebase_to_kratos.go get-mapping <firebase_uid>
```

Returns the Kratos UUID for a given Firebase UID.

### 5. List All Mappings

```bash
go run scripts/migrate_firebase_to_kratos.go list-mappings
```

Shows all Firebase UID → Kratos UUID mappings with migration timestamps.

## Migration Logic

### Firebase UID Detection

A Firebase UID is identified as:

- Exactly 28 characters long
- Contains only alphanumeric characters (a-z, A-Z, 0-9)

### Deterministic UUID Generation

```
SHA256("kratos-migration-" + firebase_uid) → formatted as UUID
```

This ensures:

- Same Firebase UID always produces same Kratos UUID
- No collisions
- Reproducible across environments

### User Migration Process

For each Firebase user:

1. **Check if already migrated**: Query `migration_user_mapping` table
2. **Generate/retrieve Kratos UUID**: Create deterministic UUID or use existing
3. **Determine user type**:
   - **Tenant User** (has `tenant_id`):
     - Create user with `CreateSharedUserWithTenant`
     - Create tenant membership with roles
     - Default role: `USER` if no roles specified
   - **Global User** (no `tenant_id`):
     - Create user with `CreateSharedUser`
     - Assign global roles
     - Default role: `USER` if no roles specified
4. **Record mapping**: Insert into `migration_user_mapping`
5. **Commit transaction**: All-or-nothing operation

## Database Schema

### Migration Mapping Table

```sql
CREATE TABLE migration_user_mapping (
    firebase_uid VARCHAR(28) PRIMARY KEY,
    kratos_uuid VARCHAR(128) UNIQUE NOT NULL,
    migrated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
```

## Error Handling

- **Transaction rollback**: Any error during migration rolls back changes
- **Duplicate detection**: Skips already migrated users
- **Logging**: Detailed logs for each operation
- **Summary report**: Shows success/skip/error counts

## Example Output

```
Starting Firebase to Kratos user migration...
Migrating user: abc123def456ghi789jkl012mnop (email: user@example.com)
Created mapping: Firebase UID abc123def456ghi789jkl012mnop -> Kratos UUID 12345678-1234-5678-1234-567812345678
Created user with tenant membership: 12345678-1234-5678-1234-567812345678 (tenant: tenant-123, roles: [USER ADMIN])
Successfully migrated user: abc123def456ghi789jkl012mnop -> 12345678-1234-5678-1234-567812345678

Migration complete!
Successfully migrated: 150 users
Skipped (non-Firebase): 25 users
Errors: 0 users
```

## Safety Features

1. **Idempotent**: Safe to run multiple times
2. **Transaction-based**: Atomic operations
3. **Mapping preservation**: Maintains Firebase UID → Kratos UUID relationships
4. **Non-destructive**: Original Firebase user records remain unchanged
5. **Validation**: Checks Firebase UID format before processing

## Post-Migration

After successful migration:

1. **Verify mappings**: Use `list-mappings` command
2. **Test authentication**: Ensure users can authenticate with Kratos
3. **Update application code**: Switch from Firebase to Kratos auth provider
4. **Monitor logs**: Check for any authentication issues
5. **Keep mapping table**: Useful for debugging and reference

## Rollback

To rollback the migration:

```sql
-- Remove migrated Kratos users
DELETE FROM core_users
WHERE id IN (SELECT kratos_uuid FROM migration_user_mapping);

-- Remove tenant memberships
DELETE FROM core_user_tenant_memberships
WHERE user_id IN (SELECT kratos_uuid FROM migration_user_mapping);

-- Clear mapping table
TRUNCATE migration_user_mapping;
```

## Troubleshooting

### User already exists error

- Check if user was partially migrated
- Verify mapping table for existing entry
- Use `get-mapping` to check status

### Transaction timeout

- Migrate in smaller batches
- Use `migrate-user` for individual migrations
- Increase database connection timeout

### Role assignment issues

- Verify tenant exists in `core_tenants`
- Check role names match expected values
- Review logs for specific error messages

## Integration with Existing Services

The migration utility uses existing services:

- `CreateSharedUser`: For global users
- `CreateSharedUserWithTenant`: For tenant users
- `AddSharedUserToTenant`: For adding memberships

This ensures consistency with your application's user management logic.
