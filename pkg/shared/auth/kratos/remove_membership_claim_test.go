package kratos

import (
	"context"
	"net/http"
	"testing"
)

// A user row can outlive its identity — records left behind by the earlier
// provider have none at all, and the users list marks them "No sign-in
// account". Removing such a member from a tenant used to fail with
// "user_not_found" because the claim lookup demanded an identity, so the
// leftover membership could never be cleared. With no identity there is no
// claim holding the tenant: the caller's goal is already met.
func TestRemoveTenantMembershipClaimTolerates404(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"status":"Not Found","message":"Unable to locate the resource"}}`))
	})

	err := newTestClient(t, h).RemoveTenantMembershipClaim(context.Background(),
		"3c66043e-4d4e-4009-b73f-2d4d483746af", "tenant-1")
	if err != nil {
		t.Fatalf("RemoveTenantMembershipClaim on a missing identity = %v; want nil", err)
	}
}

// Tolerating a missing identity must not swallow a provider that is failing:
// claim removal is what actually revokes access, so a 500 has to stay an error
// its callers abort on.
func TestRemoveTenantMembershipClaimKeeps500AsFailure(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":500,"message":"boom"}}`))
	})

	err := newTestClient(t, h).RemoveTenantMembershipClaim(context.Background(),
		"3c66043e-4d4e-4009-b73f-2d4d483746af", "tenant-1")
	if err == nil {
		t.Fatal("expected an error when the provider is down — access is not revoked")
	}
}
