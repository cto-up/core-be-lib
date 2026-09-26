package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/access"
	"ctoup.com/coreapp/pkg/shared/auth"
)

// hub#237. An X-Api-Key request carried no claims, so every auth.Is* gate
// refused it — even for a token an admin created.

func testCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/things", nil)
	return c
}

func TestAnAdminsAPITokenPassesARoleGate(t *testing.T) {
	access.InstallRoleResolver(func(_ context.Context, tenantID, userID string) ([]string, bool, error) {
		if tenantID == "t1" && userID == "admin-1" {
			return []string{access.RoleAdmin}, false, nil
		}
		return nil, false, nil
	})
	t.Cleanup(func() { access.InstallRoleResolver(nil) })

	c := testCtx()
	(&AuthMiddleware{}).setAPITokenPrincipal(c, "t1", "admin-1", []string{"read"})

	if !auth.IsAdmin(c) {
		t.Fatal("an admin's token was refused by auth.IsAdmin")
	}
	p, ok := access.PrincipalFrom(c.Request.Context())
	if !ok {
		t.Fatal("no Principal on the request")
	}
	if p.UserID != "admin-1" || p.TenantID != "t1" || p.AAL != "aal1" || p.StepUpPossible {
		t.Errorf("principal = %+v", p)
	}
	if !p.Can("read") || p.Can("propose") {
		t.Error("the token's own scopes are its caps")
	}
}

func TestAMembersAPITokenStaysUnprivileged(t *testing.T) {
	access.InstallRoleResolver(func(context.Context, string, string) ([]string, bool, error) {
		return []string{"LEARNER"}, false, nil
	})
	t.Cleanup(func() { access.InstallRoleResolver(nil) })

	c := testCtx()
	(&AuthMiddleware{}).setAPITokenPrincipal(c, "t1", "member-1", nil)
	if auth.IsAdmin(c) || auth.IsCustomerAdmin(c) || auth.IsSuperAdmin(c) {
		t.Error("a member's token gained an admin role")
	}
}

func TestTheSessionDoorBuildsAPrincipal(t *testing.T) {
	c := testCtx()
	(&AuthMiddleware{}).setAuthenticatedUser(c, &auth.AuthenticatedUser{
		UserID: "u1", TenantID: "t1", Email: "a@b.c",
		Claims:           map[string]interface{}{access.RoleCustomerAdmin: true},
		IsActingReseller: false,
	})
	p, ok := access.PrincipalFrom(c.Request.Context())
	if !ok || !p.HasRole(access.RoleCustomerAdmin) || p.Caps != nil || !p.StepUpPossible {
		t.Fatalf("principal = %+v, ok=%v", p, ok)
	}
	if !access.IsTenantAdminFromContext(c.Request.Context()) {
		t.Error("a CUSTOMER_ADMIN session is a tenant admin below the door")
	}
}

type fakeMembership struct {
	row repository.CoreUserTenantMembership
	err error
}

func (f fakeMembership) GetSharedUserTenantMembership(context.Context, repository.GetSharedUserTenantMembershipParams) (repository.CoreUserTenantMembership, error) {
	return f.row, f.err
}

func TestMembershipRoleResolver(t *testing.T) {
	ctx := context.Background()
	active := MembershipRoleResolver(fakeMembership{row: repository.CoreUserTenantMembership{Status: "active", Roles: []string{access.RoleAdmin}}})
	if roles, _, err := active(ctx, "t1", "u1"); err != nil || len(roles) != 1 {
		t.Errorf("active member: %v %v", roles, err)
	}
	invited := MembershipRoleResolver(fakeMembership{row: repository.CoreUserTenantMembership{Status: "invited", Roles: []string{access.RoleAdmin}}})
	if roles, _, _ := invited(ctx, "t1", "u1"); roles != nil {
		t.Error("an invitation never joined grants nothing")
	}
	none := MembershipRoleResolver(fakeMembership{err: pgx.ErrNoRows})
	if roles, _, err := none(ctx, "t1", "u1"); roles != nil || err != nil {
		t.Error("not a member is an answer, not an error")
	}
	broken := MembershipRoleResolver(fakeMembership{err: errors.New("db down")})
	if _, _, err := broken(ctx, "t1", "u1"); err == nil {
		t.Error("a failed read is an error")
	}
}
