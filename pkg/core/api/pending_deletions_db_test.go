//go:build testutils

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/shared/auth"
)

func TestListPendingDeletions_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		_, err := store.ConnPool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}

	for _, tenant := range []string{"t1", "t2"} {
		exec(`INSERT INTO core_tenants (tenant_id, name, subdomain, allow_password_sign_up, user_id)
			VALUES ($1, $2, $1, false, 'owner')`, tenant, tenant+" org")
	}
	member := func(id, tenant string) {
		exec(`INSERT INTO core_users (id, email, profile) VALUES ($1, $2, jsonb_build_object('name', $3::text))
			ON CONFLICT (id) DO NOTHING`, id, id+"@example.com", id)
		exec(`INSERT INTO core_user_tenant_memberships (user_id, tenant_id, status, joined_at) VALUES ($1, $2, 'active', NOW())`, id, tenant)
	}

	// Mirrors the ticket: A asked to close on 20 Oct, B was warned on 22 Oct,
	// C is pending in another tenant, D is a member with nothing pending.
	requested := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	warned := time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)
	lastSeen := time.Date(2023, 11, 21, 8, 0, 0, 0, time.UTC)
	member("userA", "t1")
	member("userB", "t1")
	member("userB", "t2")
	member("userC", "t2")
	member("userD", "t1")
	exec(`UPDATE core_users SET deletion_requested_at = $2, deletion_scheduled_at = $3, deletion_reason = 'moving on' WHERE id = $1`,
		"userA", requested, requested.Add(service.DeletionGrace))
	exec(`UPDATE core_users SET inactivity_warned_at = $2, last_seen_at = $3 WHERE id = $1`, "userB", warned, lastSeen)
	exec(`UPDATE core_users SET deletion_requested_at = $2, deletion_scheduled_at = $3 WHERE id = $1`,
		"userC", requested, requested.Add(service.DeletionGrace))

	h := NewPendingDeletionsHandler(store)
	gin.SetMode(gin.TestMode)
	list := func(tenantID string, superAdmin bool, params core.ListPendingDeletionsParams) (int, []core.PendingDeletion) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin-api/v1/pending-deletions", nil)
		c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
		c.Set(auth.AUTH_CLAIMS, map[string]interface{}{string(core.SUPERADMIN): superAdmin, string(core.ADMIN): true})
		h.ListPendingDeletions(c, params)
		var out []core.PendingDeletion
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		}
		return w.Code, out
	}

	t.Run("an admin sees their tenant's pending accounts, soonest first", func(t *testing.T) {
		code, rows := list("t1", false, core.ListPendingDeletionsParams{})
		require.Equal(t, http.StatusOK, code)
		require.Len(t, rows, 2)

		a, b := rows[0], rows[1]
		assert.Equal(t, "userA", a.UserId)
		assert.Equal(t, core.ClosureRequested, a.Stage)
		assert.Equal(t, "moving on", *a.Reason)
		assert.True(t, requested.Equal(*a.RequestedAt))
		assert.True(t, time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC).Equal(a.DeletesOn))
		assert.Nil(t, a.WarnedAt)
		assert.Equal(t, []string{"t1 org"}, a.Tenants)

		assert.Equal(t, "userB", b.UserId)
		assert.Equal(t, core.InactivityWarned, b.Stage)
		assert.Equal(t, "userB@example.com", b.Email)
		assert.Equal(t, "userB", b.Name)
		assert.True(t, warned.Equal(*b.WarnedAt))
		assert.True(t, lastSeen.Equal(*b.LastSeenAt))
		assert.Nil(t, b.RequestedAt)
		// Scheduled once the warning period is over, deleted after the grace.
		want := warned.AddDate(0, service.InactivityWarningMonths, 0).Add(service.DeletionGrace)
		assert.True(t, want.Equal(b.DeletesOn), "deletesOn %s, want %s", b.DeletesOn, want)
		assert.Equal(t, []string{"t1 org"}, b.Tenants, "an admin sees only their own tenant's name")
	})

	t.Run("an admin without a tenant is refused", func(t *testing.T) {
		code, _ := list("", false, core.ListPendingDeletionsParams{})
		assert.Equal(t, http.StatusForbidden, code)
	})

	t.Run("a super-admin without a tenant sees every account with its tenants", func(t *testing.T) {
		code, rows := list("", true, core.ListPendingDeletionsParams{})
		require.Equal(t, http.StatusOK, code)
		require.Len(t, rows, 3)
		ids := []string{rows[0].UserId, rows[1].UserId, rows[2].UserId}
		assert.ElementsMatch(t, []string{"userA", "userB", "userC"}, ids)
		for _, r := range rows {
			if r.UserId == "userB" {
				assert.Equal(t, []string{"t1 org", "t2 org"}, r.Tenants)
			}
		}
	})

	t.Run("pages", func(t *testing.T) {
		one, two := int32(1), int32(2)
		_, first := list("t1", false, core.ListPendingDeletionsParams{PageSize: &one})
		_, second := list("t1", false, core.ListPendingDeletionsParams{PageSize: &one, Page: &two})
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		assert.Equal(t, "userA", first[0].UserId)
		assert.Equal(t, "userB", second[0].UserId)
	})

	t.Run("keeping the account or signing in removes the row", func(t *testing.T) {
		_, err := store.CancelUserDeletion(ctx, "userA")
		require.NoError(t, err)
		require.NoError(t, store.TouchSharedUserLastSeen(ctx, "userB"))
		_, rows := list("t1", false, core.ListPendingDeletionsParams{})
		assert.Empty(t, rows)
	})
}
