package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ctoup.com/coreapp/pkg/shared/auth"
)

// The two guards the /admin-api user handlers apply and their /superadmin-api
// twins did not.
//
// The fork is the point: these handlers are the same thirteen operations
// written twice, differing only in where the tenant comes from, and the copies
// drifted on the destructive one. DeleteUser on /admin-api refuses to delete
// the caller and refuses a target who outranks them; DeleteUserFromSuperAdmin
// did neither, while HardDeleteUserFromSuperAdmin — one function away in the
// same file — already refused self-deletion. That asymmetry is what says drift
// rather than design.
//
// Neither guard is vacuous on the super-admin path. The auth middleware's
// reseller exemption tests HasPrefix(path, "/superadmin-api/v1/tenant") and
// these routes live under ".../v1/tenants/{id}/users/...", so a RESELLER
// reaches them — scoped by IsAllowedToManageTenant to tenants it owns, but not
// outranking everybody the way a SUPER_ADMIN does.
//
// The self guard runs BEFORE the tenant lookup, which is what lets these tests
// run with a nil store: a request that is already invalid never reaches the
// database.

func callerContext(t *testing.T, callerID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/superadmin-api/v1/tenants/x/users/y", nil)
	c.Set(auth.AUTH_USER_ID, callerID)
	return c, rec
}

const caller = "11111111-1111-4111-8111-111111111111"

func TestDeleteFromSuperAdminRefusesSelf(t *testing.T) {
	c, rec := callerContext(t, caller)

	// A nil store is deliberate: reaching it would mean the guard ran too late.
	(&UserSuperAdminHandler{}).DeleteUserFromSuperAdmin(c, uuid.New(), caller)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("removing yourself must be refused, got %d", rec.Code)
	}
}

func TestRemoveFromTenantFromSuperAdminRefusesSelf(t *testing.T) {
	c, rec := callerContext(t, caller)

	(&UserSuperAdminHandler{}).RemoveUserFromTenantFromSuperAdmin(c, uuid.New(), caller)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("removing yourself must be refused, got %d", rec.Code)
	}
}

// The guard must not fire on somebody else — otherwise it would refuse every
// call, and the test above would pass for the wrong reason. A different target
// falls through to the tenant lookup, which panics on the nil store; that panic
// IS the evidence the guard let the request past.
func TestTheSelfGuardOnlyStopsTheCaller(t *testing.T) {
	for name, fn := range map[string]func(*gin.Context, uuid.UUID, string){
		"DeleteUserFromSuperAdmin":           (&UserSuperAdminHandler{}).DeleteUserFromSuperAdmin,
		"RemoveUserFromTenantFromSuperAdmin": (&UserSuperAdminHandler{}).RemoveUserFromTenantFromSuperAdmin,
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := callerContext(t, caller)
			defer func() {
				if recover() == nil && rec.Code == http.StatusForbidden {
					t.Fatal("the self guard fired on a different user")
				}
			}()
			fn(c, uuid.New(), "22222222-2222-4222-8222-222222222222")
		})
	}
}

// An absent caller id must not be read as "the caller is the target". The empty
// string is what c.GetString returns when the middleware did not set it, and
// deleting a user whose id is also empty is not a thing — but matching them
// would turn a missing claim into a silent refusal that looks like a policy.
func TestAnAbsentCallerIDDoesNotMatchAnEmptyTarget(t *testing.T) {
	c, rec := callerContext(t, "")
	defer func() {
		if recover() == nil && rec.Code == http.StatusForbidden {
			t.Fatal("an absent caller id must not be treated as a self-delete")
		}
	}()
	(&UserSuperAdminHandler{}).DeleteUserFromSuperAdmin(c, uuid.New(), "")
}
