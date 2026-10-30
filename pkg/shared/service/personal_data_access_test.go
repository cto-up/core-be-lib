//go:build testutils

package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordPersonalDataAccess_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	SetPersonalDataAccessStore(store)
	t.Cleanup(func() { SetPersonalDataAccessStore(nil) })
	ctx := context.Background()

	rows := func() []repository.CorePersonalDataAccessLog {
		got, err := store.ListPersonalDataAccess(ctx, repository.ListPersonalDataAccessParams{RowLimit: 100})
		require.NoError(t, err)
		return got
	}
	reset := func() {
		_, err := store.ConnPool.Exec(ctx, `DELETE FROM core_personal_data_access_log`)
		require.NoError(t, err)
	}

	t.Run("a staff read of another person is written", func(t *testing.T) {
		reset()
		RecordPersonalDataAccess(ctx, PersonalDataAccess{
			TenantID: "t1", ActorUserID: "admin", SubjectUserID: "learner",
			Resource: "agent.run", ResourceID: "run-1",
		})
		require.Eventually(t, func() bool { return len(rows()) == 1 }, 5*time.Second, 20*time.Millisecond)
		r := rows()[0]
		assert.Equal(t, "t1", r.TenantID)
		assert.Equal(t, "admin", r.ActorUserID)
		assert.Equal(t, "learner", r.SubjectUserID)
		assert.Equal(t, "agent.run", r.Resource)
		assert.Equal(t, "run-1", r.ResourceID.String)
		assert.WithinDuration(t, time.Now(), r.AccessedAt, time.Minute)
	})

	t.Run("reading one's own record writes nothing", func(t *testing.T) {
		reset()
		RecordPersonalDataAccess(ctx, PersonalDataAccess{
			TenantID: "t1", ActorUserID: "learner", SubjectUserID: "learner", Resource: "agent.run",
		})
		// A marker written after it proves the writer had time to run.
		RecordPersonalDataAccess(ctx, PersonalDataAccess{
			TenantID: "t1", ActorUserID: "admin", SubjectUserID: "marker", Resource: "x",
		})
		require.Eventually(t, func() bool { return len(rows()) == 1 }, 5*time.Second, 20*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		got := rows()
		require.Len(t, got, 1)
		assert.Equal(t, "marker", got[0].SubjectUserID)
	})

	t.Run("the tenant defaults to the request's", func(t *testing.T) {
		reset()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(auth.AUTH_TENANT_ID_KEY, "from-host")
		RecordPersonalDataAccess(c, PersonalDataAccess{ActorUserID: "admin", SubjectUserID: "learner", Resource: "lms.roster"})
		require.Eventually(t, func() bool { return len(rows()) == 1 }, 5*time.Second, 20*time.Millisecond)
		assert.Equal(t, "from-host", rows()[0].TenantID)
		assert.False(t, rows()[0].ResourceID.Valid)
	})

	t.Run("a failing write neither blocks nor panics the caller", func(t *testing.T) {
		broken, cleanup, err := testutils.NewMigratedStore()
		require.NoError(t, err)
		cleanup() // pool closed, container gone
		SetPersonalDataAccessStore(broken)
		t.Cleanup(func() { SetPersonalDataAccessStore(store) })

		// The write runs in a goroutine; the bound only tells "returned" from
		// "waited", so it leaves a loaded suite room (hub#641).
		start := time.Now()
		RecordPersonalDataAccess(ctx, PersonalDataAccess{TenantID: "t1", ActorUserID: "a", SubjectUserID: "b", Resource: "x"})
		assert.Less(t, time.Since(start), time.Second)
	})

	t.Run("with no store configured it is a no-op", func(t *testing.T) {
		SetPersonalDataAccessStore((*db.Store)(nil))
		t.Cleanup(func() { SetPersonalDataAccessStore(store) })
		RecordPersonalDataAccess(ctx, PersonalDataAccess{TenantID: "t1", ActorUserID: "a", SubjectUserID: "b", Resource: "x"})
	})
}
