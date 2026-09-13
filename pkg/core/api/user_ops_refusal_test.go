package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/shared/auth"
)

// Thirteen user operations used to exist twice, and the copies drifted on the
// destructive one. They exist once now, parameterised by TenantScope — so the
// property worth testing is not "does operation N check the tenant", thirteen
// times, but "does EVERY operation stop at the scope".
//
// That is what these assert, and the nil store is the assertion: reaching the
// database would panic. A refused request must not have touched it.

// opsWithNoStore is deliberately empty. Any operation that reads o.store or
// o.userService after a refused scope will panic, and the test will say so.
func opsWithNoStore() *userOps { return &userOps{} }

// refusingScope stands in for ManagedTenant when the caller may not manage the
// tenant it names.
func refusingScope(*gin.Context) (scoped, error) {
	return scoped{}, refuse(http.StatusForbidden, "not allowed to manage this tenant")
}

func privilegedContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	req := httptest.NewRequest(http.MethodPost, "/superadmin-api/v1/tenants/x/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	// A SUPER_ADMIN: passes every guard that runs BEFORE the scope, so what the
	// test observes is the scope refusing and nothing else.
	c.Set(auth.AUTH_USER_ID, "caller-id")
	c.Set(auth.AUTH_CLAIMS, map[string]interface{}{string(core.SUPERADMIN): true})
	return c, rec
}

func TestEveryUserOperationStopsAtARefusedTenant(t *testing.T) {
	const targetUser = "some-other-user"

	for name, run := range map[string]func(o *userOps, c *gin.Context){
		"listUsers":         func(o *userOps, c *gin.Context) { o.listUsers(c, refusingScope, listParams{}) },
		"getUserByID":       func(o *userOps, c *gin.Context) { o.getUserByID(c, refusingScope, targetUser) },
		"checkUserExists":   func(o *userOps, c *gin.Context) { o.checkUserExists(c, refusingScope, "a@b.test") },
		"assignRole":        func(o *userOps, c *gin.Context) { o.assignRole(c, refusingScope, targetUser, core.CUSTOMERADMIN) },
		"unassignRole":      func(o *userOps, c *gin.Context) { o.unassignRole(c, refusingScope, targetUser, core.CUSTOMERADMIN) },
		"updateUserStatus":  func(o *userOps, c *gin.Context) { o.updateUserStatus(c, refusingScope, targetUser) },
		"reactivateUser":    func(o *userOps, c *gin.Context) { o.reactivateUser(c, refusingScope, targetUser) },
		"addUser":           func(o *userOps, c *gin.Context) { o.addUser(c, refusingScope) },
		"updateUser":        func(o *userOps, c *gin.Context) { o.updateUser(c, refusingScope, targetUser) },
		"deleteUser":        func(o *userOps, c *gin.Context) { o.deleteUser(c, refusingScope, targetUser) },
		"removeFromTenant":  func(o *userOps, c *gin.Context) { o.removeUserFromTenant(c, refusingScope, targetUser) },
		"addUserMembership": func(o *userOps, c *gin.Context) { o.addUserMembership(c, refusingScope, targetUser, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := privilegedContext(t, `{}`)

			// A nil store: reaching it means the operation acted before the
			// scope answered.
			run(opsWithNoStore(), c)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403 — a refused tenant must stop the operation", rec.Code)
			}
		})
	}
}

// The other half of the guard decision: removing yourself from the tenant you
// are administering is the same footgun at either privilege level. The
// /admin-api handlers refused it and their /superadmin-api twins did not, while
// the hard-delete endpoint beside them already did.
//
// Both paths run the same code now, so one test covers both — and it runs
// before the scope, which is why a nil store and a refusing scope are enough.
func TestRemovingYourselfIsRefusedAtEitherLevel(t *testing.T) {
	const caller = "caller-id"

	for name, run := range map[string]func(o *userOps, c *gin.Context){
		"deleteUser":       func(o *userOps, c *gin.Context) { o.deleteUser(c, refusingScope, caller) },
		"removeFromTenant": func(o *userOps, c *gin.Context) { o.removeUserFromTenant(c, refusingScope, caller) },
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := privilegedContext(t, `{}`)

			run(opsWithNoStore(), c)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "self") {
				t.Fatalf("the refusal must say why, got %s", rec.Body.String())
			}
		})
	}
}

// Defence in depth, and the same on both paths. Redundant with the route gate,
// which is the trade this repo already makes for tenant filters in SQL.
func TestDestructiveOperationsRequireAdminPrivileges(t *testing.T) {
	for name, run := range map[string]func(o *userOps, c *gin.Context){
		"deleteUser":       func(o *userOps, c *gin.Context) { o.deleteUser(c, refusingScope, "other") },
		"removeFromTenant": func(o *userOps, c *gin.Context) { o.removeUserFromTenant(c, refusingScope, "other") },
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodDelete, "/admin-api/v1/users/other", nil)
			c.Set(auth.AUTH_USER_ID, "caller-id") // signed in, but no admin role

			run(opsWithNoStore(), c)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401", rec.Code)
			}
		})
	}
}
