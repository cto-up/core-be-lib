-- Drop tables in reverse order to avoid foreign key constraint errors
DROP TABLE IF EXISTS core_api_token_audit_logs;
DROP TABLE IF EXISTS core_api_tokens;
DROP TABLE IF EXISTS core_client_applications;