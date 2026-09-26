package access

import (
	"context"
	"errors"
	"testing"
)

// ADR 048 §B, hub#237.

func TestIsTenantAdminReadsThePrincipal(t *testing.T) {
	ctx := context.Background()
	if IsTenantAdminFromContext(ctx) {
		t.Fatal("a bare context is not an admin")
	}
	if !IsTenantAdminFromContext(WithPrincipal(ctx, Principal{Roles: []string{RoleCustomerAdmin}})) {
		t.Error("a CUSTOMER_ADMIN principal is a tenant admin")
	}
	if IsTenantAdminFromContext(WithPrincipal(ctx, Principal{Roles: []string{"LEARNER"}})) {
		t.Error("a plain member is not")
	}
	// The one-bit flag care sets still answers on its own.
	if !IsTenantAdminFromContext(ContextWithTenantAdmin(ctx, true)) {
		t.Error("the legacy flag must keep working")
	}
}

// HasAdminPrivileges counts an acting reseller as an admin. A Principal that
// did not would deny someone authorized today — a regression made by the fix.
func TestAnActingResellerIsATenantAdmin(t *testing.T) {
	if !(Principal{ActingReseller: true}).IsTenantAdmin() {
		t.Error("acting reseller must count as admin")
	}
}

func TestCapsNilIsUnscoped(t *testing.T) {
	if !(Principal{}).Can("apply") {
		t.Error("a session (nil Caps) is unscoped")
	}
	p := Principal{Caps: []string{"read"}}
	if !p.Can("read") || p.Can("propose") {
		t.Error("a scoped credential carries exactly its scopes")
	}
}

func TestResolveRolesUsesTheInstalledResolver(t *testing.T) {
	t.Cleanup(func() { InstallRoleResolver(nil) })

	roles, _, err := ResolveRoles(context.Background(), "t1", "u1")
	if err != nil || roles != nil {
		t.Fatalf("no resolver installed must grant nothing, got %v %v", roles, err)
	}

	InstallRoleResolver(func(_ context.Context, tenantID, userID string) ([]string, bool, error) {
		if tenantID == "t1" && userID == "u1" {
			return []string{RoleAdmin}, false, nil
		}
		return nil, false, errors.New("unexpected")
	})
	roles, _, err = ResolveRoles(context.Background(), "t1", "u1")
	if err != nil || len(roles) != 1 || roles[0] != RoleAdmin {
		t.Fatalf("got %v %v", roles, err)
	}
	if roles, _, _ := ResolveRoles(context.Background(), "", "u1"); roles != nil {
		t.Error("no tenant, no roles")
	}
}
