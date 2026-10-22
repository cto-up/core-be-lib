//go:build testutils

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
)

// claimRemover is the one identity-provider call Leave makes.
type claimRemover struct {
	auth.AuthClient
	mu      sync.Mutex
	removed []string
}

func (f *claimRemover) RemoveTenantMembershipClaim(_ context.Context, uid, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, uid+"@"+tenantID)
	return nil
}

type fakeProvider struct {
	auth.AuthProvider
	client *claimRemover
}

func (p fakeProvider) GetAuthClient() auth.AuthClient { return p.client }

// decidingContributor asks one decision of one user, and records the answer.
type decidingContributor struct {
	userID  string
	applied []core.LeaveDecision
}

func (d *decidingContributor) Name() string { return "test-member-rights" }
func (d *decidingContributor) LeaveImpacts(_ context.Context, userID, _ string) ([]core.LeaveImpact, error) {
	if userID != d.userID {
		return nil, nil
	}
	return []core.LeaveImpact{{Key: "test.ownedThings", Severity: core.Decision, Actions: &[]core.LeaveImpactActions{"keep"}}}, nil
}
func (d *decidingContributor) ApplyLeaveDecisions(_ context.Context, userID, _ string, decisions []core.LeaveDecision) error {
	if userID == d.userID {
		d.applied = append(d.applied, decisions...)
	}
	return nil
}
func (d *decidingContributor) Export(_ context.Context, userID string) (any, error) {
	return map[string]string{"owner": userID}, nil
}
func (d *decidingContributor) Erase(context.Context, string, string, access.EraseScope) error {
	return nil
}

func TestAdminActsOnAMembersDataRights_Integration(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		_, err := store.ConnPool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	tenant := func(id string) {
		exec(`INSERT INTO core_tenants (tenant_id, name, subdomain, allow_password_sign_up, user_id)
			VALUES ($1, $2, $1, false, 'owner')`, id, id+" org")
	}
	user := func(id string, globalRoles ...string) {
		if globalRoles == nil {
			globalRoles = []string{}
		}
		exec(`INSERT INTO core_users (id, email, profile, roles) VALUES ($1, $2, jsonb_build_object('name', $3::text), $4)`,
			id, id+"@example.com", id, globalRoles)
	}
	member := func(id, tenantID string, roles ...string) {
		exec(`INSERT INTO core_user_tenant_memberships (user_id, tenant_id, status, joined_at, roles)
			VALUES ($1, $2, 'active', NOW(), $3)`, id, tenantID, roles)
	}

	tenant("t1")
	tenant("t2")
	user("admin")
	member("admin", "t1", "ADMIN")
	user("single")
	member("single", "t1", "USER")
	user("multi")
	member("multi", "t1", "USER")
	member("multi", "t2", "USER")
	user("leaver")
	member("leaver", "t1", "USER")
	user("tenantAdmin")
	member("tenantAdmin", "t1", "ADMIN")
	user("platform", string(core.SUPERADMIN))
	member("platform", "t1", "USER")

	contributor := &decidingContributor{userID: "leaver"}
	access.RegisterUserDataContributor(contributor)
	access.SetPersonalDataAccessStore(store)

	claims := &claimRemover{}
	gin.SetMode(gin.TestMode)
	adminH := &UserAdminHandler{userOps{store: store, authProvider: fakeProvider{client: claims}}}
	superH := &UserSuperAdminHandler{userOps{store: store, authProvider: fakeProvider{client: claims}}}

	type caller struct {
		id     string
		claims map[string]interface{}
	}
	tenantAdminOfT1 := caller{"admin", map[string]interface{}{string(core.ADMIN): true}}
	superAdmin := caller{"root", map[string]interface{}{string(core.SUPERADMIN): true}}
	customerAdmin := caller{"cust", map[string]interface{}{string(core.CUSTOMERADMIN): true}}

	call := func(who caller, method, body string, run func(c *gin.Context)) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, "/admin-api/v1/users/x", strings.NewReader(body))
		if body != "" {
			c.Request.Header.Set("Content-Type", "application/json")
		}
		c.Set(auth.AUTH_USER_ID, who.id)
		c.Set(auth.AUTH_TENANT_ID_KEY, "t1")
		c.Set(auth.AUTH_CLAIMS, who.claims)
		run(c)
		c.Writer.WriteHeaderNow()
		return w
	}
	refusal := func(t *testing.T, w *httptest.ResponseRecorder) core.MemberActionRefusal {
		t.Helper()
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		var r core.MemberActionRefusal
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		return r
	}

	t.Run("an admin exports a single-tenant member, and the export is logged", func(t *testing.T) {
		w := call(tenantAdminOfT1, http.MethodGet, "", func(c *gin.Context) { adminH.ExportMemberData(c, "single") })
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var export core.UserDataExport
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &export))
		assert.Equal(t, "single", export.User.Id)
		assert.Contains(t, export.Modules, "test-member-rights", "the same document as the member's own export")

		require.Eventually(t, func() bool {
			rows, err := store.ListPersonalDataAccess(ctx, repository.ListPersonalDataAccessParams{RowLimit: 10})
			if err != nil {
				return false
			}
			for _, r := range rows {
				if r.ActorUserID == "admin" && r.SubjectUserID == "single" && r.Resource == "core.user_export" && r.TenantID == "t1" {
					return true
				}
			}
			return false
		}, 5*time.Second, 50*time.Millisecond)
	})

	t.Run("an admin may not export or close a member of another tenant, and is told why", func(t *testing.T) {
		r := refusal(t, call(tenantAdminOfT1, http.MethodGet, "", func(c *gin.Context) { adminH.ExportMemberData(c, "multi") }))
		assert.Equal(t, core.MemberOfOtherTenants, r.Reason)
		assert.Contains(t, r.Message, "platform administrator")
		assert.NotContains(t, r.Message, "t2 org", "the other organization is not named")

		r = refusal(t, call(tenantAdminOfT1, http.MethodPost, "", func(c *gin.Context) { adminH.ScheduleMemberAccountDeletion(c, "multi") }))
		assert.Equal(t, core.MemberOfOtherTenants, r.Reason)
		got, err := store.GetSharedUserByID(ctx, "multi")
		require.NoError(t, err)
		assert.False(t, got.DeletionScheduledAt.Valid)
	})

	t.Run("an admin may still preview removing a multi-tenant member", func(t *testing.T) {
		w := call(tenantAdminOfT1, http.MethodGet, "", func(c *gin.Context) { adminH.GetMemberLeavePreview(c, "multi") })
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("a super admin exports a multi-tenant member, on either surface", func(t *testing.T) {
		w := call(superAdmin, http.MethodGet, "", func(c *gin.Context) { adminH.ExportMemberData(c, "multi") })
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

		t1, err := store.GetTenantByTenantID(ctx, "t1")
		require.NoError(t, err)
		w = call(superAdmin, http.MethodGet, "", func(c *gin.Context) { superH.ExportMemberDataFromSuperAdmin(c, t1.ID, "multi") })
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("acting on oneself is refused", func(t *testing.T) {
		for name, run := range map[string]func(c *gin.Context){
			"export":  func(c *gin.Context) { adminH.ExportMemberData(c, "admin") },
			"preview": func(c *gin.Context) { adminH.GetMemberLeavePreview(c, "admin") },
			"leave":   func(c *gin.Context) { adminH.LeaveTenantForMember(c, "admin") },
			"close":   func(c *gin.Context) { adminH.ScheduleMemberAccountDeletion(c, "admin") },
		} {
			t.Run(name, func(t *testing.T) {
				assert.Equal(t, http.StatusForbidden, call(tenantAdminOfT1, http.MethodPost, "", run).Code)
			})
		}
	})

	t.Run("acting on somebody who outranks the caller is refused", func(t *testing.T) {
		w := call(customerAdmin, http.MethodGet, "", func(c *gin.Context) { adminH.ExportMemberData(c, "tenantAdmin") })
		assert.Equal(t, http.StatusForbidden, w.Code, "a CUSTOMER_ADMIN does not act on an ADMIN")

		w = call(tenantAdminOfT1, http.MethodPost, "", func(c *gin.Context) { adminH.ScheduleMemberAccountDeletion(c, "platform") })
		assert.Equal(t, http.StatusForbidden, w.Code, "a tenant ADMIN does not close a platform administrator's account")
	})

	t.Run("a non-member is not found", func(t *testing.T) {
		w := call(tenantAdminOfT1, http.MethodGet, "", func(c *gin.Context) { adminH.ExportMemberData(c, "nobody") })
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("removing a member walks the leave lifecycle with their decisions", func(t *testing.T) {
		w := call(tenantAdminOfT1, http.MethodGet, "", func(c *gin.Context) { adminH.GetMemberLeavePreview(c, "leaver") })
		require.Equal(t, http.StatusOK, w.Code)
		var preview core.LeaveTenantPreview
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
		assert.False(t, preview.CanLeaveNow, "the member's decision is the admin's to answer")
		require.Len(t, preview.Impacts, 1)

		w = call(tenantAdminOfT1, http.MethodPost, "", func(c *gin.Context) { adminH.LeaveTenantForMember(c, "leaver") })
		require.Equal(t, http.StatusConflict, w.Code, "an unanswered decision blocks")

		w = call(tenantAdminOfT1, http.MethodPost, `{"decisions":[{"key":"test.ownedThings","action":"keep"}]}`,
			func(c *gin.Context) { adminH.LeaveTenantForMember(c, "leaver") })
		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

		require.Len(t, contributor.applied, 1)
		assert.Equal(t, "test.ownedThings", contributor.applied[0].Key)
		assert.Contains(t, claims.removed, "leaver@t1")
		m, err := store.GetUserTenantMembership(ctx, repository.GetUserTenantMembershipParams{UserID: "leaver", TenantID: "t1"})
		require.NoError(t, err)
		assert.Equal(t, "inactive", m.Status, "the row survives for the dormancy purge and the return path")
	})

	t.Run("closing a single-tenant member schedules it, and it shows in pending deletions with its reason", func(t *testing.T) {
		w := call(tenantAdminOfT1, http.MethodPost, `{"reason":"asked by email"}`,
			func(c *gin.Context) { adminH.ScheduleMemberAccountDeletion(c, "single") })
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var status core.AccountDeletion
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
		require.NotNil(t, status.ScheduledFor)
		assert.WithinDuration(t, time.Now().Add(service.DeletionGrace), *status.ScheduledFor, time.Minute)

		w = call(tenantAdminOfT1, http.MethodGet, "", func(c *gin.Context) {
			NewPendingDeletionsHandler(store).ListPendingDeletions(c, core.ListPendingDeletionsParams{})
		})
		require.Equal(t, http.StatusOK, w.Code)
		var pending []core.PendingDeletion
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pending))
		require.Len(t, pending, 1)
		assert.Equal(t, "single", pending[0].UserId)
		assert.Equal(t, core.ClosureRequested, pending[0].Stage)
		assert.Equal(t, service.AdminClosureReason+": asked by email", *pending[0].Reason)

		r := refusal(t, call(tenantAdminOfT1, http.MethodPost, "", func(c *gin.Context) { adminH.ScheduleMemberAccountDeletion(c, "single") }))
		assert.Equal(t, core.DeletionAlreadyScheduled, r.Reason)
	})

	t.Run("a super admin closes a multi-tenant member's account", func(t *testing.T) {
		w := call(superAdmin, http.MethodPost, "", func(c *gin.Context) { adminH.ScheduleMemberAccountDeletion(c, "multi") })
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("the super-admin surface refuses a tenant the caller may not manage", func(t *testing.T) {
		t1, err := store.GetTenantByTenantID(ctx, "t1")
		require.NoError(t, err)
		w := call(customerAdmin, http.MethodGet, "", func(c *gin.Context) { superH.ExportMemberDataFromSuperAdmin(c, t1.ID, "single") })
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
