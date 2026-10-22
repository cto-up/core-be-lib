//go:build testutils

package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/shared/emailservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleDeletionByAdmin_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()

	type notice struct {
		userID    string
		closure   AdminClosure
		deletesOn time.Time
	}
	var sent []notice
	svc := NewMembershipLifecycleService(store, nil)
	svc.notifyAdminClosure = func(u repository.CoreUser, c AdminClosure, deletesOn time.Time) error {
		sent = append(sent, notice{u.ID, c, deletesOn})
		return nil
	}

	t.Run("schedules with the admin reason and emails the holder the date", func(t *testing.T) {
		u := newUser(t, store, nil)
		before := time.Now()
		status, err := svc.ScheduleDeletionByAdmin(ctx, u, AdminClosure{Reason: "asked by phone", TenantName: "Acme", KeepURL: "https://acme.test/user/me/organizations"})
		require.NoError(t, err)

		require.NotNil(t, status.ScheduledFor)
		assert.WithinDuration(t, before.Add(DeletionGrace), *status.ScheduledFor, time.Minute)
		got := getUser(t, store, u)
		assert.Equal(t, AdminClosureReason+": asked by phone", got.DeletionReason.String)

		require.Len(t, sent, 1)
		assert.Equal(t, u, sent[0].userID)
		assert.Equal(t, "Acme", sent[0].closure.TenantName)
		assert.True(t, sent[0].deletesOn.Equal(*status.ScheduledFor))

		_, err = svc.CancelDeletion(ctx, u)
		require.NoError(t, err, "the holder keeps the account the usual way")
		assert.False(t, getUser(t, store, u).DeletionScheduledAt.Valid)
	})

	t.Run("without a reason the reason is the plain admin one", func(t *testing.T) {
		u := newUser(t, store, nil)
		_, err := svc.ScheduleDeletionByAdmin(ctx, u, AdminClosure{})
		require.NoError(t, err)
		assert.Equal(t, AdminClosureReason, getUser(t, store, u).DeletionReason.String)
	})

	t.Run("an already scheduled account is refused and not re-mailed", func(t *testing.T) {
		u := newUser(t, store, nil)
		_, err := svc.ScheduleDeletionByAdmin(ctx, u, AdminClosure{})
		require.NoError(t, err)
		sent = nil
		_, err = svc.ScheduleDeletionByAdmin(ctx, u, AdminClosure{})
		assert.ErrorIs(t, err, ErrDeletionScheduled)
		assert.Empty(t, sent)
	})

	t.Run("a failed email does not undo the closure", func(t *testing.T) {
		failing := NewMembershipLifecycleService(store, nil)
		failing.notifyAdminClosure = func(repository.CoreUser, AdminClosure, time.Time) error { return errors.New("smtp down") }
		u := newUser(t, store, nil)
		_, err := failing.ScheduleDeletionByAdmin(ctx, u, AdminClosure{})
		require.NoError(t, err)
		assert.True(t, getUser(t, store, u).DeletionScheduledAt.Valid)
	})
}

func TestAdminClosureTemplateRenders(t *testing.T) {
	r := emailservice.NewEmailRequest("from@x.test", []string{"to@x.test"}, "s", "")
	require.NoError(t, r.ParseTemplate(filepath.Join("..", "..", "..", "templates", adminClosureTemplate), adminClosureData{
		Name: "Ada", Organization: "Acme", DeletesOn: "21 November 2026", KeepURL: "https://acme.test/user/me/organizations",
	}))
	assert.Contains(t, r.Body, "Acme")
	assert.Contains(t, r.Body, "21 November 2026")
	assert.Contains(t, r.Body, `href="https://acme.test/user/me/organizations"`)
}
