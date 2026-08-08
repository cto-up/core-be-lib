-- +goose Up
BEGIN;

-- Closing an account schedules a deletion; it does not perform one (ADR 040).
-- The grace period is what makes an account takeover non-destructive and a
-- change of mind cheap: signing in during the window cancels it. A cron job
-- executes the deletion once deletion_scheduled_at has passed.
--
-- Two columns rather than one: requested_at is when the person asked, scheduled_at
-- is when it fires. Keeping both means changing the grace period later does not
-- rewrite history, and support can answer "when did they ask for this".
ALTER TABLE core_users
    ADD COLUMN deletion_requested_at TIMESTAMPTZ,
    ADD COLUMN deletion_scheduled_at TIMESTAMPTZ,
    ADD COLUMN deletion_reason TEXT;

-- Partial: the cron job scans for due deletions, and all but a handful of rows
-- are NULL forever.
CREATE INDEX idx_core_users_deletion_due
    ON core_users (deletion_scheduled_at)
    WHERE deletion_scheduled_at IS NOT NULL;

-- Stamped once the dormancy purge has run for a departure, so the sweeper does
-- not re-purge every night for the rest of the row's life.
ALTER TABLE core_user_tenant_memberships
    ADD COLUMN dormant_purged_at TIMESTAMPTZ;

COMMIT;

-- +goose Down
BEGIN;

DROP INDEX IF EXISTS idx_core_users_deletion_due;
ALTER TABLE core_user_tenant_memberships
    DROP COLUMN dormant_purged_at;
ALTER TABLE core_users
    DROP COLUMN deletion_requested_at,
    DROP COLUMN deletion_scheduled_at,
    DROP COLUMN deletion_reason;

COMMIT;
