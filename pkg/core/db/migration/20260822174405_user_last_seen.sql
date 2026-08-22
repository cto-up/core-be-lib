-- +goose Up
-- Kratos records authenticated_at — when credentials were last entered — and
-- nothing that moves when a page is loaded. With a 30-day session lifespan a
-- user who is active daily still shows a sign-in weeks old, so "last active"
-- has to be stamped by us, on the request path.
ALTER TABLE core_users ADD COLUMN last_seen_at timestamptz;

-- Only the users list reads this, always for a page of ids it already has, so
-- no index: an unindexed nullable column costs nothing to write, and writes
-- are what this column does constantly.

-- +goose Down
ALTER TABLE core_users DROP COLUMN IF EXISTS last_seen_at;
