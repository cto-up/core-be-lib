use .env

1. CREATE TABLE migration_user_mapping (
   firebase_uid VARCHAR(28) PRIMARY KEY,
   kratos_uuid UUID UNIQUE NOT NULL
   );

2. For each user, check if id is a firebase uid

- create a kratos_uuid,
- create the core_users with same value with but with kratos_uuid
- upsert migration_user_mapping

3. if user.tenantID
   create membership with roles
   else
   create roles with roles

4. Migrate all foreign keys
find them
```sql
SELECT table_name, column_name
FROM information_schema.columns
WHERE (column_name LIKE '%user_id%' OR column_name LIKE '%owner_id%' OR column_name = 'created_by')
  AND table_schema = 'public';
```
