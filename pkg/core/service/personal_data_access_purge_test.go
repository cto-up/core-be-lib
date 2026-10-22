//go:build testutils

package service

import (
	"context"
	"testing"
	"time"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPurgePersonalDataAccessLog_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()
	now := time.Now()
	for i, at := range []time.Time{now.AddDate(0, -12, -1), now.AddDate(0, -11, 0)} {
		_, err := store.ConnPool.Exec(ctx, `INSERT INTO core_personal_data_access_log
			(tenant_id, actor_user_id, subject_user_id, resource, accessed_at) VALUES ('t', 'a', $1, 'x', $2)`,
			[]string{"old", "recent"}[i], at)
		require.NoError(t, err)
	}

	n, err := NewMembershipLifecycleService(store, nil).PurgePersonalDataAccessLog(ctx, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	left, err := store.ListPersonalDataAccess(ctx, repository.ListPersonalDataAccessParams{RowLimit: 10})
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, "recent", left[0].SubjectUserID)
}
