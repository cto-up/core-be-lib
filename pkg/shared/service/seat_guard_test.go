package service

import (
	"context"
	"errors"
	"testing"

	"ctoup.com/coreapp/api/openapi/core"
)

// An unconfigured deployment must never start refusing memberships — the guard
// is opt-in, and its absence is the normal case.
func TestSeatGuardDefaultsToAllowing(t *testing.T) {
	RegisterSeatGuard(nil)
	if err := checkSeatGuard(context.Background(), "t1", []core.Role{core.USER}); err != nil {
		t.Fatalf("no guard registered must allow, got %v", err)
	}
}

func TestSeatGuardRefusalPropagates(t *testing.T) {
	want := errors.New("seat limit reached")
	RegisterSeatGuard(func(context.Context, string, []core.Role) error { return want })
	defer RegisterSeatGuard(nil)

	if err := checkSeatGuard(context.Background(), "t1", []core.Role{core.USER}); !errors.Is(err, want) {
		t.Fatalf("guard refusal must reach the caller, got %v", err)
	}
}

// Roles reach the guard because not every membership consumes a seat: a plan
// that counts LEARNERS should not be billed for adding an admin. Core does not
// interpret that itself — it just passes them along.
func TestSeatGuardReceivesTenantAndRoles(t *testing.T) {
	var gotTenant string
	var gotRoles []core.Role
	RegisterSeatGuard(func(_ context.Context, tenantID string, roles []core.Role) error {
		gotTenant, gotRoles = tenantID, roles
		return nil
	})
	defer RegisterSeatGuard(nil)

	_ = checkSeatGuard(context.Background(), "acme", []core.Role{core.ADMIN, core.USER})

	if gotTenant != "acme" {
		t.Fatalf("tenant = %q, want acme", gotTenant)
	}
	if len(gotRoles) != 2 {
		t.Fatalf("roles = %v, want both passed through", gotRoles)
	}
}

// Registering again replaces rather than stacks, so a re-registration at boot
// cannot silently double-check.
func TestRegisterSeatGuardReplaces(t *testing.T) {
	calls := 0
	RegisterSeatGuard(func(context.Context, string, []core.Role) error { calls++; return nil })
	RegisterSeatGuard(func(context.Context, string, []core.Role) error { return nil })
	defer RegisterSeatGuard(nil)

	_ = checkSeatGuard(context.Background(), "t1", nil)
	if calls != 0 {
		t.Fatalf("the replaced guard must not run, ran %d times", calls)
	}
}
