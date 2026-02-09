# Firebase to Kratos Migration - Summary

## What Was Created

### 1. Database Migration

**File**: `pkg/core/db/migration/000020_firebase_migration_mapping.up.sql`

Creates the `migration_user_mapping` table to track Firebase UID → Kratos UUID conversions:

- `firebase_uid` (VARCHAR(28), PRIMARY KEY): Original Firebase user ID
- `kratos_uuid` (VARCHAR(128), UNIQUE): New Kratos user ID
- `migrated_at`: Timestamp of migration
- `created_at`: Record creation timestamp

### 2. Migration Utility

**File**: `scripts/migrate_firebase_to_kratos.go`

Go program that handles the migration process with the following features:

#### Commands

- `migrate-all`: Migrate all Firebase users to Kratos
- `migrate-user <uid>`: Migrate a specific user by Firebase UID
- `get-mapping <uid>`: Retrieve Kratos UUID for a Firebase UID
- `list-mappings`: List all migration mappings

#### Key Functions

- **isFirebaseUID()**: Validates if an ID is a Firebase UID (28 chars, alphanumeric)
- **generateDeterministicKratosUUID()**: Creates consistent UUID from Firebase UID using SHA256
- **getOrCreateMapping()**: Manages mapping table entries
- **migrateUser()**: Migrates individual user with transaction safety
- **MigrateAllUsers()**: Batch migration with error handling

### 3. Documentation

- **MIGRATION_README.md**: Complete documentation with examples
- **MIGRATION_QUICK_START.md**: Quick reference guide
- **MIGRATION_SUMMARY.md**: This file

### 4. Makefile Targets

Added convenient commands to Makefile:

- `make migrate-firebase-all`: Migrate all users
- `make migrate-firebase-user UID=<uid>`: Migrate specific user
- `make migrate-firebase-mapping UID=<uid>`: Check mapping
- `make migrate-firebase-list`: List all mappings

## Migration Strategy Implementation

### ✅ Step 1: Create Migration Mapping Table

```sql
CREATE TABLE migration_user_mapping (
    firebase_uid VARCHAR(28) PRIMARY KEY,
    kratos_uuid VARCHAR(128) UNIQUE NOT NULL
);
```

### ✅ Step 2: Detect Firebase UIDs

- Checks if user ID is exactly 28 characters
- Validates alphanumeric characters only
- Skips non-Firebase users automatically

### ✅ Step 3: Generate Deterministic Kratos UUID

```go
SHA256("kratos-migration-" + firebase_uid) → UUID format
```

- Same Firebase UID always produces same Kratos UUID
- No collisions
- Reproducible across environments

### ✅ Step 4: Create Users with Proper Context

#### For Tenant Users (has `tenant_id`):

```go
CreateSharedUserWithTenant(
    ID: kratosUUID,
    Email: email,
    Profile: profile,
    TenantID: tenantID,
    TenantRoles: roles,
    InvitedBy: "migration",
    InvitedAt: now()
)
```

- Creates user in `core_users`
- Creates membership in `core_user_tenant_memberships`
- Assigns tenant-specific roles
- Default role: `USER` if none specified

#### For Global Users (no `tenant_id`):

```go
CreateSharedUser(
    ID: kratosUUID,
    Email: email,
    Profile: profile,
    Roles: globalRoles
)
```

- Creates user in `core_users`
- Assigns global roles
- Default role: `USER` if none specified

### ✅ Step 5: Record Mapping

```sql
INSERT INTO migration_user_mapping (firebase_uid, kratos_uuid)
VALUES ($1, $2)
ON CONFLICT (firebase_uid) DO NOTHING
```

### ✅ Step 6: Transaction Safety

- All operations wrapped in database transactions
- Rollback on any error
- Atomic operations ensure data consistency

## Usage Examples

### Basic Migration

```bash
# Set database connection
export DATABASE_URL="postgresql://user:pass@localhost:5432/db"

# Run migration
make migrate-firebase-all
```

### Single User Migration

```bash
make migrate-firebase-user UID=abc123def456ghi789jkl012mnop
```

### Verify Migration

```bash
# Check specific mapping
make migrate-firebase-mapping UID=abc123def456ghi789jkl012mnop

# List all mappings
make migrate-firebase-list
```

## Safety Features

1. **Idempotent**: Safe to run multiple times
2. **Transaction-based**: All-or-nothing operations
3. **Non-destructive**: Original Firebase records preserved
4. **Validation**: Checks Firebase UID format
5. **Error handling**: Detailed logging and error reporting
6. **Mapping preservation**: Maintains Firebase UID → Kratos UUID relationships

## What Gets Migrated

✅ User ID (Firebase UID → Kratos UUID)  
✅ Email address  
✅ User profile (name, photo, metadata)  
✅ Roles (global or tenant-specific)  
✅ Tenant memberships  
✅ Created timestamps

## What Doesn't Change

❌ Original Firebase user records (preserved for reference)  
❌ Existing Kratos users (skipped automatically)  
❌ Tenant configurations  
❌ Other database tables

## Integration with Existing Services

The migration utility uses your existing services:

- `CreateSharedUser`: For global users
- `CreateSharedUserWithTenant`: For tenant users
- Uses existing `repository.Queries` for database operations
- Follows same patterns as your application code

## Post-Migration Steps

1. **Verify mappings**: `make migrate-firebase-list`
2. **Test authentication**: Ensure users can authenticate with Kratos
3. **Update application**: Switch from Firebase to Kratos auth provider
4. **Monitor logs**: Check for authentication issues
5. **Keep mapping table**: Useful for debugging and reference

## Rollback Procedure

If needed, rollback the migration:

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

## Technical Details

### Dependencies

- Go 1.x+
- PostgreSQL database
- pgx/v5 driver
- Existing codebase services

### Performance

- Processes users sequentially for safety
- Transaction per user ensures atomicity
- Suitable for databases with thousands of users
- For very large databases, consider batching

### Error Handling

- Detailed error messages
- Logs each operation
- Summary report at end
- Non-fatal errors don't stop migration

## Next Steps

1. **Backup database** before running migration
2. **Run database migration**: `make migrateup`
3. **Test with single user**: `make migrate-firebase-user UID=<test_uid>`
4. **Review logs** for any issues
5. **Run full migration**: `make migrate-firebase-all`
6. **Verify results**: `make migrate-firebase-list`
7. **Update application** to use Kratos

## Support

For issues or questions:

- Check `MIGRATION_README.md` for detailed documentation
- Review `MIGRATION_QUICK_START.md` for quick reference
- Check logs for specific error messages
- Verify database connection and permissions
