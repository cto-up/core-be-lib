package access

import (
	"context"
	"sync/atomic"
)

// Principal is who is calling and what their credential allows (ADR 048 §B).
//
// One value, built once per door — session, API token, architect key or OAuth,
// in-process agent — so code below the door asks the same question the same
// way. It widens the one-bit tenant-admin flag this package started with; that
// flag's getter is kept and now reads the Principal too, so existing callers
// (care) compile unchanged.
type Principal struct {
	TenantID string
	UserID   string
	Email    string

	// Roles are the caller's roles in TenantID: CUSTOMER_ADMIN, ADMIN,
	// SUPER_ADMIN, or none.
	Roles []string
	// ActingReseller counts as admin — HasAdminPrivileges does, and a
	// Principal that forgot it would deny someone authorized today.
	ActingReseller bool
	// IsolateByUser: a sign-up tenant's non-admin, whose reads filter by user.
	IsolateByUser bool

	// Caps is what the credential was granted (read, propose, apply). Nil
	// means an unscoped credential — a session.
	Caps []string
	// AAL is the authenticator assurance level ("aal1", "aal2"); empty when
	// the door cannot tell. StepUpPossible is false for every credential that
	// has no browser behind it to step up with.
	AAL            string
	StepUpPossible bool

	// System marks an internal job. Set explicitly, never defaulted.
	System bool
}

const (
	RoleSuperAdmin    = "SUPER_ADMIN"
	RoleAdmin         = "ADMIN"
	RoleCustomerAdmin = "CUSTOMER_ADMIN"
)

// HasRole reports whether the caller holds any of roles.
func (p Principal) HasRole(roles ...string) bool {
	for _, have := range p.Roles {
		for _, want := range roles {
			if have == want {
				return true
			}
		}
	}
	return false
}

// IsTenantAdmin is the question HasAdminPrivileges answers on a gin context:
// a tenant-level admin role, or an acting reseller.
func (p Principal) IsTenantAdmin() bool {
	return p.ActingReseller || p.HasRole(RoleSuperAdmin, RoleAdmin, RoleCustomerAdmin)
}

// Can reports whether the credential carries a capability. An unscoped
// credential (nil Caps) carries every one.
func (p Principal) Can(capability string) bool {
	if p.Caps == nil {
		return true
	}
	for _, c := range p.Caps {
		if c == capability {
			return true
		}
	}
	return false
}

const ctxKeyPrincipal ctxKey = ctxKeyTenantAdmin + 1

// WithPrincipal carries p on ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKeyPrincipal, p)
}

// PrincipalFrom returns the Principal a door put on ctx.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKeyPrincipal).(Principal)
	return p, ok
}

// RoleResolver answers "which roles does this user hold in this tenant, and
// are their reads isolated?" for the credentials that carry no claims — an API
// token, an architect key, an OAuth token, an agent acting for its owner. One
// function in core serves all of them, where each library used to declare its
// own yes/no question (TenantAdminResolver, CanDefineTypes, CanPublish).
type RoleResolver func(ctx context.Context, tenantID, userID string) (roles []string, isolate bool, err error)

var roleResolver atomic.Pointer[RoleResolver]

// InstallRoleResolver sets the resolver every door uses. The host calls it
// once at boot; nil uninstalls it.
func InstallRoleResolver(r RoleResolver) {
	if r == nil {
		roleResolver.Store(nil)
		return
	}
	roleResolver.Store(&r)
}

// ResolveRoles asks the installed resolver. With none installed it returns no
// roles and no error: a door without a resolver grants nothing, which is the
// safe direction.
func ResolveRoles(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
	r := roleResolver.Load()
	if r == nil || tenantID == "" || userID == "" {
		return nil, false, nil
	}
	return (*r)(ctx, tenantID, userID)
}
