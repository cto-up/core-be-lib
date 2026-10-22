-- +goose Up
-- Stamped when the inactive-account warning is sent, cleared by the next
-- request the person makes. The sweeper schedules a deletion only for an
-- account warned at least a month ago, so nobody is deleted without notice.
ALTER TABLE core_users ADD COLUMN inactivity_warned_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE core_users DROP COLUMN IF EXISTS inactivity_warned_at;
