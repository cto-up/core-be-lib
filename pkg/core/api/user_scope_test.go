package core

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ctoup.com/coreapp/pkg/shared/auth"
)

// TenantScope is the seam thirteen duplicated operations collapse onto, so what
// it does with a missing, wrong-typed or unmanageable tenant is the contract
// every one of them now inherits. These are the tests the two forked copies
// could not share.

func scopeContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin-api/v1/users", nil)
	return c, rec
}

func TestSessionTenantIsTheSessionsOwn(t *testing.T) {
	c, _ := scopeContext(t)
	c.Set(auth.AUTH_TENANT_ID_KEY, "acme")

	got, err := (&userOps{}).SessionTenant(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.id != "acme" {
		t.Fatalf("got %q, want %q", got.id, "acme")
	}
}

// An admin signed in on the root domain has no tenant, and a handful of
// operations have a super-admin branch for exactly that. An empty id is a fact
// about the session, not a failure — reporting it as one would 500 every
// root-domain user lookup.
func TestAnEmptySessionTenantIsNotAnError(t *testing.T) {
	c, _ := scopeContext(t)
	c.Set(auth.AUTH_TENANT_ID_KEY, "")

	got, err := (&userOps{}).SessionTenant(c)
	if err != nil {
		t.Fatalf("the root domain must resolve, got %v", err)
	}
	if got.id != "" {
		t.Fatalf("got %q, want empty", got.id)
	}
}

func TestAMissingSessionTenantIsAnError(t *testing.T) {
	c, _ := scopeContext(t)

	if _, err := (&userOps{}).SessionTenant(c); err == nil {
		t.Fatal("a session with no tenant key at all must not resolve")
	}
}

// The middleware puts a string there. If something else ever does, the old code
// panicked on the type assertion — a 500 with a stack trace rather than a 500
// with a reason.
func TestAWrongTypedSessionTenantIsAnErrorNotAPanic(t *testing.T) {
	c, _ := scopeContext(t)
	c.Set(auth.AUTH_TENANT_ID_KEY, 42)

	if _, err := (&userOps{}).SessionTenant(c); err == nil {
		t.Fatal("a non-string tenant id must not resolve")
	}
}

// A refusal renders the status the scope chose; anything else is a 500. The
// distinction matters because "you may not manage this tenant" and "the tenant
// lookup failed" were both 500s in one of the two forked copies.
func TestARefusalRendersItsOwnStatus(t *testing.T) {
	c, rec := scopeContext(t)

	target, ok := resolve(c, func(*gin.Context) (scoped, error) {
		return scoped{}, refuse(http.StatusForbidden, "not allowed to manage this tenant")
	})

	if ok {
		t.Fatal("a refused scope must stop the operation")
	}
	if target.id != "" {
		t.Fatalf("a refused scope must yield no tenant, got %q", target.id)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal must render as JSON: %v", err)
	}
}

func TestAFailureToResolveIsAFiveHundred(t *testing.T) {
	c, rec := scopeContext(t)

	if _, ok := resolve(c, func(*gin.Context) (scoped, error) {
		return scoped{}, errors.New("the database is down")
	}); ok {
		t.Fatal("a failed scope must stop the operation")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
}

func TestAResolvedScopePassesTheTenantThrough(t *testing.T) {
	c, rec := scopeContext(t)

	target, ok := resolve(c, func(*gin.Context) (scoped, error) { return scoped{id: "acme"}, nil })
	if !ok {
		t.Fatal("a resolved scope must continue")
	}
	if target.id != "acme" {
		t.Fatalf("got %q, want acme", target.id)
	}
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatal("a resolved scope must write nothing itself")
	}
}
