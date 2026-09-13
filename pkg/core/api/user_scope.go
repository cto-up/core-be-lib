package core

import (
	"errors"
	"fmt"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Thirteen user operations were written twice — once under /admin-api on the
// session's own tenant, once under /superadmin-api on a named one — and the
// pairs differed in their first twelve lines and nowhere else.
//
// Nothing was broken by having two. But the copies had already drifted on the
// destructive path: DeleteUser refused self-deletion and rank-jumping, and
// DeleteUserFromSuperAdmin did neither, while the hard-delete endpoint sitting
// beside it guarded against exactly the first. That reads as drift, not design,
// and with 26% test-file coverage in this library nothing was going to catch
// the day it happened.
//
// TenantScope is the seam: which tenant, and may you? It is the only question
// the two paths answer differently, so it is the only thing parameterised.

// TenantScope resolves the tenant an operation acts on, refusing when the
// caller may not act on it.
//
// The resolved id may legitimately be EMPTY: an admin signed in on the root
// domain has no tenant, and a handful of operations have a super-admin branch
// for that case. An empty id is a fact about the session, not a failure.
type TenantScope func(c *gin.Context) (scoped, error)

// scoped is what a TenantScope resolves to: which tenant, and how to reach its
// identity provider.
//
// The auth client belongs here and not in each operation, because the two paths
// genuinely resolve it differently and the difference is not cosmetic. On
// /admin-api the request arrives on the tenant's OWN subdomain, so the client
// comes from the host. On /superadmin-api it arrives on the admin domain while
// acting on somebody else's tenant, so the host is the wrong answer and the
// client comes from the tenant id. Leaving that to the caller is how a shared
// implementation would eventually be given the admin path's rule on the
// super-admin path and mint a password reset against the wrong identity
// provider.
type scoped struct {
	id string

	// subdomain is the tenant's own, and EMPTY means "whatever host this
	// request arrived on". That is not a missing value: on /admin-api the
	// request arrives on the tenant's subdomain already, so the host is the
	// right answer and buildTenantURL says so. On /superadmin-api it arrives on
	// the admin domain, so a welcome email built from the host would point the
	// new user at a sign-in page for somebody else's tenant.
	subdomain string

	// name is how this tenant is written in a user-facing string — the subject
	// of the "you've been added to X" mail.
	//
	// The two paths knew different amounts here and still do. The super-admin
	// path has the tenant row, so it uses the real name. The session path has
	// only the host, so it uses the subdomain — a slug where a name belongs,
	// which is what it has always sent. Carrying the difference explicitly is
	// better than a lookup this operation does not otherwise need, and better
	// than silently changing what one of the two paths mails out.
	name string

	// authClient is the identity-provider client for this tenant. Resolved
	// lazily: most operations never need one, and it is a network call.
	authClient func(c *gin.Context) (auth.AuthClient, error)
}

// scopeRefusal carries the status a refusal should be rendered with, so the
// scope decides the code and the operation just stops.
type scopeRefusal struct {
	status int
	err    error
}

func (r scopeRefusal) Error() string { return r.err.Error() }
func (r scopeRefusal) Unwrap() error { return r.err }

func refuse(status int, format string, args ...any) error {
	return scopeRefusal{status: status, err: fmt.Errorf(format, args...)}
}

// resolve runs the scope and, on refusal, writes the response. The caller stops
// when it returns false — the one line that used to be twelve.
func resolve(c *gin.Context, scope TenantScope) (scoped, bool) {
	target, err := scope(c)
	if err == nil {
		return target, true
	}
	logger := util.GetLoggerFromCtx(c.Request.Context())
	var refusal scopeRefusal
	if errors.As(err, &refusal) {
		logger.Err(refusal.err).Msg("tenant scope refused")
		c.JSON(refusal.status, helpers.ErrorResponse(refusal.err))
		return scoped{}, false
	}
	logger.Err(err).Msg("failed to resolve tenant scope")
	c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
	return scoped{}, false
}

// authClient resolves the identity-provider client, rendering the failure. The
// caller stops when it returns false.
func (t scoped) client(c *gin.Context) (auth.AuthClient, bool) {
	client, err := t.authClient(c)
	if err != nil {
		logger := util.GetLoggerFromCtx(c.Request.Context())
		logger.Err(err).Msg("Failed to get auth client")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return nil, false
	}
	return client, true
}

// userOps is what both user handlers are made of. Embedded rather than passed,
// so every shared implementation reads exactly as it did when it lived in one
// of the two files.
type userOps struct {
	store        *db.Store
	authProvider auth.AuthProvider
	userService  access.UserService
}

// SessionTenant is the caller's own tenant, from the authenticated session.
//
// There is no "may you?" here because there is no choice of tenant: the path
// gate on /admin-api already established that the caller is an ADMIN or a
// SUPER_ADMIN, and the session decides which tenant that is over.
func (o *userOps) SessionTenant(c *gin.Context) (scoped, error) {
	value, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		return scoped{}, errors.New("tenant id not found on the session")
	}
	tenantID, ok := value.(string)
	if !ok {
		return scoped{}, fmt.Errorf("tenant id on the session is %T, not a string", value)
	}
	subdomain, _ := util.GetSubdomain(c)

	return scoped{
		id:   tenantID,
		name: subdomain,
		// The request arrived on this tenant's own subdomain, so both the auth
		// client and any tenant URL come from the host.
		authClient: func(c *gin.Context) (auth.AuthClient, error) {
			subdomain, err := util.GetSubdomain(c)
			if err != nil {
				return nil, err
			}
			return o.authProvider.GetAuthClientForSubdomain(c, subdomain)
		},
	}, nil
}

// ManagedTenant is a named tenant, refused unless the caller may manage it.
//
// This guard is NOT redundant with the path gate, and that is the thing most
// worth knowing about /superadmin-api. The reseller exemption in
// auth_middleware.go tests HasPrefix(path, "/superadmin-api/v1/tenant"), and
// the user routes live under …/v1/tenants/…, so RESELLERS reach every one of
// these operations. IsAllowedToManageTenant is what scopes them to tenants
// where tenant.ResellerID is their own — and it is why the rank guards on this
// path are load-bearing rather than vacuous, which is the opposite of what
// "super-admin outranks everyone" suggests.
func (o *userOps) ManagedTenant(id uuid.UUID) TenantScope {
	return func(c *gin.Context) (scoped, error) {
		tenant, err := o.store.Queries.GetTenantByID(c, id)
		if err != nil {
			return scoped{}, fmt.Errorf("get tenant: %w", err)
		}
		if !auth.IsAllowedToManageTenant(c, tenant) {
			return scoped{}, refuse(http.StatusForbidden, "not allowed to manage this tenant")
		}
		return scoped{
			id:        tenant.TenantID,
			subdomain: tenant.Subdomain,
			name:      tenant.Name,
			// The request arrived on the ADMIN domain while acting on this
			// tenant — the host says nothing about which identity provider.
			authClient: func(c *gin.Context) (auth.AuthClient, error) {
				return o.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
			},
		}, nil
	}
}

var (
	errOnlySuperAdminMayListAll    = errors.New("only super admins may list all users")
	errOnlySuperAdminWithoutTenant = errors.New("only SUPER_ADMIN can get user without tenant")
	errResellerRoleCap             = errors.New("resellers are not allowed to assign roles higher than CUSTOMER_ADMIN")
)
