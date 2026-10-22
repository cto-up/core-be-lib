//go:build testutils

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sentWarning struct {
	userID   string
	closesOn time.Time
}

func TestProcessInactiveAccounts_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()

	var sent []sentWarning
	svc := NewMembershipLifecycleService(store, nil)
	svc.warnInactive = func(u repository.CoreUser, _ int, closesOn time.Time) error {
		sent = append(sent, sentWarning{u.ID, closesOn})
		return nil
	}
	reset := func() {
		sent = nil
		_, err := store.ConnPool.Exec(ctx, `DELETE FROM core_users; DELETE FROM core_global_configs`)
		require.NoError(t, err)
	}

	// Mirrors the ticket: 36 months, last seen 2023-09-15.
	lastSeen := time.Date(2023, 9, 15, 10, 0, 0, 0, time.UTC)
	warnDay := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	deleteDay := time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC)

	t.Run("warns at month N-1, then schedules the deletion at month N", func(t *testing.T) {
		reset()
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, lastSeen)

		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		require.Len(t, sent, 1)
		assert.Equal(t, u, sent[0].userID)
		assert.False(t, sent[0].closesOn.Before(lastSeen.AddDate(0, 36, 0)))
		assert.False(t, sent[0].closesOn.Before(warnDay.AddDate(0, 1, 0)), "never sooner than the month of notice")
		got := getUser(t, store, u)
		assert.True(t, got.InactivityWarnedAt.Valid)
		assert.False(t, got.DeletionScheduledAt.Valid, "a warning is not a deletion")

		// The warning was stamped by NOW(); pretend it happened on warnDay.
		setWarnedAt(t, store, u, warnDay)
		require.NoError(t, svc.ProcessInactiveAccounts(ctx, deleteDay))
		got = getUser(t, store, u)
		require.True(t, got.DeletionScheduledAt.Valid)
		assert.Equal(t, InactivityDeletionReason, got.DeletionReason.String)
		assert.Len(t, sent, 1, "no second warning")
	})

	t.Run("a sign-in clears the warning and nothing is scheduled", func(t *testing.T) {
		reset()
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, lastSeen)
		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		require.True(t, getUser(t, store, u).InactivityWarnedAt.Valid)

		require.NoError(t, store.TouchSharedUserLastSeen(ctx, u))
		got := getUser(t, store, u)
		assert.False(t, got.InactivityWarnedAt.Valid)

		require.NoError(t, svc.ProcessInactiveAccounts(ctx, deleteDay))
		assert.False(t, getUser(t, store, u).DeletionScheduledAt.Valid)
	})

	t.Run("no deletion before a month of notice", func(t *testing.T) {
		reset()
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, lastSeen.AddDate(-1, 0, 0)) // already past N
		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		require.Len(t, sent, 1)
		assert.Equal(t, warnDay.AddDate(0, 1, 0), sent[0].closesOn, "the stated date leaves a month")

		setWarnedAt(t, store, u, warnDay)
		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay.AddDate(0, 0, 20)))
		assert.False(t, getUser(t, store, u).DeletionScheduledAt.Valid)
	})

	t.Run("0 disables the rule", func(t *testing.T) {
		reset()
		setConfig(t, store, "0")
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, lastSeen.AddDate(-5, 0, 0))
		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		assert.Empty(t, sent)
		assert.False(t, getUser(t, store, u).InactivityWarnedAt.Valid)
	})

	t.Run("the configured period replaces the default", func(t *testing.T) {
		reset()
		setConfig(t, store, "24")
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, warnDay.AddDate(0, -23, -1))
		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		assert.Len(t, sent, 1)
	})

	t.Run("an unreadable period does nothing and says so", func(t *testing.T) {
		reset()
		setConfig(t, store, "three years")
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, lastSeen.AddDate(-5, 0, 0))
		assert.Error(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		assert.Empty(t, sent)
	})

	t.Run("super-admins and accounts already closing are skipped", func(t *testing.T) {
		reset()
		admin := newUser(t, store, []string{"SUPER_ADMIN"})
		closing := newUser(t, store, nil)
		for _, u := range []string{admin, closing} {
			setLastSeen(t, store, u, lastSeen.AddDate(-5, 0, 0))
		}
		_, err := store.ConnPool.Exec(ctx,
			`UPDATE core_users SET deletion_scheduled_at = now() + interval '10 days' WHERE id = $1`, closing)
		require.NoError(t, err)

		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		assert.Empty(t, sent)
	})

	t.Run("an account never stamped counts from when stamping began", func(t *testing.T) {
		reset()
		u := newUser(t, store, nil)
		_, err := store.ConnPool.Exec(ctx,
			`UPDATE core_users SET created_at = '2019-01-01', last_seen_at = NULL WHERE id = $1`, u)
		require.NoError(t, err)

		require.NoError(t, svc.ProcessInactiveAccounts(ctx, warnDay))
		assert.Empty(t, sent, "silence before last_seen_at existed is not inactivity")

		require.NoError(t, svc.ProcessInactiveAccounts(ctx, lastSeenTrackedSince.AddDate(0, 35, 1)))
		assert.Len(t, sent, 1)
	})

	t.Run("a warning that cannot be sent is withdrawn", func(t *testing.T) {
		reset()
		failing := NewMembershipLifecycleService(store, nil)
		failing.warnInactive = func(repository.CoreUser, int, time.Time) error { return errors.New("smtp down") }
		u := newUser(t, store, nil)
		setLastSeen(t, store, u, lastSeen)

		require.NoError(t, failing.ProcessInactiveAccounts(ctx, warnDay))
		assert.False(t, getUser(t, store, u).InactivityWarnedAt.Valid,
			"an unsent warning must not start the month of notice")
	})
}

func newUser(t *testing.T, store *db.Store, roles []string) string {
	t.Helper()
	if roles == nil {
		roles = []string{}
	}
	id := uuid.NewString()
	_, err := store.CreateSharedUser(context.Background(), repository.CreateSharedUserParams{
		ID: id, Email: id + "@example.test", Roles: roles,
	})
	require.NoError(t, err)
	return id
}

func setLastSeen(t *testing.T, store *db.Store, id string, at time.Time) {
	t.Helper()
	_, err := store.ConnPool.Exec(context.Background(),
		`UPDATE core_users SET last_seen_at = $2, created_at = $2 WHERE id = $1`, id, at)
	require.NoError(t, err)
}

func setWarnedAt(t *testing.T, store *db.Store, id string, at time.Time) {
	t.Helper()
	_, err := store.ConnPool.Exec(context.Background(),
		`UPDATE core_users SET inactivity_warned_at = $2 WHERE id = $1`, id, at)
	require.NoError(t, err)
}

func setConfig(t *testing.T, store *db.Store, value string) {
	t.Helper()
	_, err := store.ConnPool.Exec(context.Background(),
		`INSERT INTO core_global_configs (name, value, user_id) VALUES ($1, $2, 'test')`,
		InactiveAccountMonthsConfig, value)
	require.NoError(t, err)
}

func getUser(t *testing.T, store *db.Store, id string) repository.CoreUser {
	t.Helper()
	u, err := store.GetSharedUserByID(context.Background(), id)
	require.NoError(t, err)
	return u
}
