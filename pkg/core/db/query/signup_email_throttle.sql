-- name: ClaimSignupEmailSend :one
-- Claims the right to send one sign-up email to an address. Returns no row when
-- the address is still cooling down or has used up its window. The row lock
-- taken by ON CONFLICT serialises concurrent claims across replicas.
INSERT INTO core_signup_email_throttle AS t (
    email_hash, last_sent_at, window_started_at, window_count
) VALUES (
    @email_hash, clock_timestamp(), clock_timestamp(), 1
)
ON CONFLICT (email_hash) DO UPDATE SET
    last_sent_at = clock_timestamp(),
    window_started_at = CASE
        WHEN t.window_started_at <= clock_timestamp() - make_interval(secs => @window_seconds::int)
        THEN clock_timestamp() ELSE t.window_started_at END,
    window_count = CASE
        WHEN t.window_started_at <= clock_timestamp() - make_interval(secs => @window_seconds::int)
        THEN 1 ELSE t.window_count + 1 END
WHERE t.last_sent_at <= clock_timestamp() - make_interval(secs => @cooldown_seconds::int)
  AND (
    t.window_started_at <= clock_timestamp() - make_interval(secs => @window_seconds::int)
    OR t.window_count < @max_per_window::int
  )
RETURNING t.email_hash;
