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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/shared/auth"
)

func TestListPersonalDataAccess_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()
	for _, r := range []repository.InsertPersonalDataAccessParams{
		{TenantID: "t1", ActorUserID: "admin1", SubjectUserID: "learnerA", Resource: "aiemployee.run", ResourceID: pgtype.Text{String: "r1", Valid: true}},
		{TenantID: "t1", ActorUserID: "admin2", SubjectUserID: "learnerB", Resource: "aiemployee.conversation"},
		{TenantID: "t2", ActorUserID: "admin3", SubjectUserID: "learnerA", Resource: "aiemployee.run"},
	} {
		require.NoError(t, store.InsertPersonalDataAccess(ctx, r))
	}

	h := NewPersonalDataAccessHandler(store)
	gin.SetMode(gin.TestMode)
	list := func(tenantID string, superAdmin bool, params core.ListPersonalDataAccessParams) (int, []core.PersonalDataAccess) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin-api/v1/personal-data-access", nil)
		c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
		c.Set(auth.AUTH_CLAIMS, map[string]interface{}{string(core.SUPERADMIN): superAdmin, string(core.ADMIN): true})
		h.ListPersonalDataAccess(c, params)
		var out []core.PersonalDataAccess
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		}
		return w.Code, out
	}

	t.Run("an admin sees their tenant's rows only", func(t *testing.T) {
		code, rows := list("t1", false, core.ListPersonalDataAccessParams{})
		require.Equal(t, http.StatusOK, code)
		require.Len(t, rows, 2)
		for _, r := range rows {
			assert.Equal(t, "t1", *r.TenantId)
		}
	})

	t.Run("filters narrow the tenant's rows", func(t *testing.T) {
		subject := "learnerA"
		_, rows := list("t1", false, core.ListPersonalDataAccessParams{SubjectUserId: &subject})
		require.Len(t, rows, 1)
		assert.Equal(t, "admin1", rows[0].ActorUserId)
		assert.Equal(t, "r1", *rows[0].ResourceId)

		actor := "admin2"
		_, rows = list("t1", false, core.ListPersonalDataAccessParams{ActorUserId: &actor})
		require.Len(t, rows, 1)

		future := time.Now().Add(time.Hour)
		_, rows = list("t1", false, core.ListPersonalDataAccessParams{From: &future})
		assert.Empty(t, rows)
		_, rows = list("t1", false, core.ListPersonalDataAccessParams{To: &future})
		assert.Len(t, rows, 2)
	})

	t.Run("an admin without a tenant is refused", func(t *testing.T) {
		code, _ := list("", false, core.ListPersonalDataAccessParams{})
		assert.Equal(t, http.StatusForbidden, code)
	})

	t.Run("a super-admin without a tenant sees every tenant", func(t *testing.T) {
		code, rows := list("", true, core.ListPersonalDataAccessParams{})
		require.Equal(t, http.StatusOK, code)
		assert.Len(t, rows, 3)
	})

	t.Run("pages", func(t *testing.T) {
		one, two := int32(1), int32(2)
		_, first := list("t1", false, core.ListPersonalDataAccessParams{PageSize: &one})
		_, second := list("t1", false, core.ListPersonalDataAccessParams{PageSize: &one, Page: &two})
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		assert.NotEqual(t, first[0].ActorUserId, second[0].ActorUserId)
	})
}
