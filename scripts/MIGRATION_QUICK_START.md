# Firebase to Kratos Migration - Quick Start

## Quick Commands

### Using Makefile (Recommended)

```bash
# 1. Migrate all users
make migrate-firebase-all

# 2. Migrate single user
make migrate-firebase-user UID=abc123def456ghi789jkl012mnop

# 3. Check mapping for a user
make migrate-firebase-mapping UID=abc123def456ghi789jkl012mnop

# 4. List all mappings
make migrate-firebase-list
```

### Using Go directly

```bash
# Set database connection
export DATABASE_URL="postgresql://user:password@localhost:5432/dbname"

# Migrate all users
go run scripts/migrate_firebase_to_kratos.go migrate-all

# Migrate single user
go run scripts/migrate_firebase_to_kratos.go migrate-user <firebase_uid>

# Get mapping
go run scripts/migrate_firebase_to_kratos.go get-mapping <firebase_uid>

# List mappings
go run scripts/migrate_firebase_to_kratos.go list-mappings
```

## Pre-Migration Checklist

- [ ] Backup database
- [ ] Run migration: `make migrateup` (applies 000020_firebase_migration_mapping.up.sql)
- [ ] Verify `DATABASE_URL` environment variable is set
- [ ] Test with single user first: `make migrate-firebase-user UID=<test_uid>`
- [ ] Review logs for any errors

## Migration Flow

```
Firebase User (28-char UID)
         ↓
   [Detect Firebase UID]
         ↓
   [Generate Kratos UUID] (deterministic SHA256-based)
         ↓
   [Check if has tenant_id]
         ↓
    ┌────┴────┐
    ↓         ↓
[Tenant]  [Global]
    ↓         ↓
[Create    [Create
 with       user
 tenant     with
 member]    roles]
    ↓         ↓
[Record mapping]
    ↓
[Success]
```

## What Gets Migrated

✅ User ID (Firebase UID → Kratos UUID)  
✅ Email address  
✅ User profile (name, photo, etc.)  
✅ Roles (global or tenant-specific)  
✅ Tenant memberships  
✅ Created timestamps

## What Doesn't Change

❌ Original Firebase user records (preserved)  
❌ Existing Kratos users (skipped)  
❌ Tenant configurations  
❌ Other database tables

## Example Session

```bash
$ make migrate-firebase-all

Starting Firebase to Kratos user migration...
Migrating user: abc123def456ghi789jkl012mnop (email: john@example.com)
Created mapping: Firebase UID abc123def456ghi789jkl012mnop -> Kratos UUID 12345678-1234-5678-1234-567812345678
Created user with tenant membership: 12345678-1234-5678-1234-567812345678 (tenant: tenant-123, roles: [USER])
Successfully migrated user: abc123def456ghi789jkl012mnop -> 12345678-1234-5678-1234-567812345678

Migrating user: xyz789abc012def345ghi678jkl901 (email: jane@example.com)
Created mapping: Firebase UID xyz789abc012def345ghi678jkl901 -> Kratos UUID 87654321-4321-8765-4321-876543218765
Created global user: 87654321-4321-8765-4321-876543218765 (roles: [USER ADMIN])
Successfully migrated user: xyz789abc012def345ghi678jkl901 -> 87654321-4321-8765-4321-876543218765

Skipping user 550e8400-e29b-41d4-a716-446655440000 - not a Firebase UID

Migration complete!
Successfully migrated: 2 users
Skipped (non-Firebase): 1 users
Errors: 0 users
```

## Verification

After migration, verify:

```bash
# Check mapping was created
make migrate-firebase-mapping UID=abc123def456ghi789jkl012mnop

# List all mappings
make migrate-firebase-list

# Query database directly
psql $DATABASE_URL -c "SELECT * FROM migration_user_mapping LIMIT 10;"
psql $DATABASE_URL -c "SELECT id, email FROM core_users WHERE id IN (SELECT kratos_uuid FROM migration_user_mapping) LIMIT 10;"
```

## Troubleshooting

### "User not found" error

- Verify Firebase UID is correct (28 characters, alphanumeric)
- Check user exists: `SELECT * FROM core_users WHERE id = '<firebase_uid>';`

### "Mapping already exists"

- User was already migrated
- Check with: `make migrate-firebase-mapping UID=<firebase_uid>`
- Safe to ignore - migration is idempotent

### "Tenant not found" error

- Verify tenant exists in `core_tenants` table
- Check tenant_id in user record matches existing tenant

### Database connection error

- Verify `DATABASE_URL` is set correctly
- Test connection: `psql $DATABASE_URL -c "SELECT 1;"`
- Check database is running: `make postgresup`

## Need Help?

See full documentation: [MIGRATION_README.md](./MIGRATION_README.md)
