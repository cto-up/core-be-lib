//go:build testutils

package core

import (
	"context"
	"testing"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimSignupEmailSend(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()

	t.Run("a second send inside the cooldown is refused", func(t *testing.T) {
		ok, err := claimSignupEmailSend(ctx, store, "cooldown@example.com")
		require.NoError(t, err)
		assert.True(t, ok)

		ok, err = claimSignupEmailSend(ctx, store, "cooldown@example.com")
		require.NoError(t, err)
		assert.False(t, ok, "resubmitting the form at once must not send a second mail")

		ok, err = claimSignupEmailSend(ctx, store, "someone-else@example.com")
		require.NoError(t, err)
		assert.True(t, ok, "the cap is per address")
	})

	t.Run("the cooldown lapses", func(t *testing.T) {
		ok, err := claimSignupEmailSend(ctx, store, "lapse@example.com")
		require.NoError(t, err)
		require.True(t, ok)

		_, err = store.ConnPool.Exec(ctx,
			`UPDATE core_signup_email_throttle SET last_sent_at = last_sent_at - interval '61 seconds'`)
		require.NoError(t, err)

		ok, err = claimSignupEmailSend(ctx, store, "lapse@example.com")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("the window caps sends until it rolls over", func(t *testing.T) {
		claim := func() bool {
			_, err := store.ClaimSignupEmailSend(ctx, repository.ClaimSignupEmailSendParams{
				EmailHash:       "window",
				WindowSeconds:   signupEmailWindowSeconds,
				CooldownSeconds: 0,
				MaxPerWindow:    signupEmailMaxPerWindow,
			})
			return err == nil
		}
		for i := 0; i < signupEmailMaxPerWindow; i++ {
			require.True(t, claim(), "send %d is inside the window's allowance", i+1)
		}
		assert.False(t, claim(), "one past the allowance is refused")

		_, err := store.ConnPool.Exec(ctx,
			`UPDATE core_signup_email_throttle SET window_started_at = window_started_at - interval '2 hours' WHERE email_hash = 'window'`)
		require.NoError(t, err)
		assert.True(t, claim(), "a new window starts a new allowance")
		assert.True(t, claim())
	})
}
