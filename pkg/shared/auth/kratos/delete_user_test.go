package kratos

import (
	"context"
	"net/http"
	"testing"

	"ctoup.com/coreapp/pkg/shared/auth"
)

// A core_users row can outlive its identity — rows created under the earlier
// Firebase provider have no Kratos identity at all. Deleting one must still
// work, and the deletion paths allow that by testing IsUserNotFound. Kratos
// answers with a bare 404 carrying no error id, which used to fall through as
// a generic "kratos-error: Unable to locate the resource", so the row could
// never be removed from the UI.
func TestDeleteUserClassifies404AsNotFound(t *testing.T) {
	// Exactly what Kratos returns: a code, no id.
	const body = `{"error":{"code":404,"status":"Not Found","message":"Unable to locate the resource"}}`

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(body))
	})

	err := newTestClient(t, h).DeleteUser(context.Background(),
		"La5Ba4NAlSRy9tFx5AIMe6xPQMt1")
	if err == nil {
		t.Fatal("expected an error from a 404 delete")
	}
	if !auth.IsUserNotFound(err) {
		t.Fatalf("IsUserNotFound(%v) = false; deletion paths rely on this to "+
			"tolerate a row whose identity is already gone", err)
	}
}

// A 404 must not swallow genuine failures — a 500 stays an error the caller
// is expected to surface.
func TestDeleteUserKeeps500AsFailure(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":500,"message":"boom"}}`))
	})

	err := newTestClient(t, h).DeleteUser(context.Background(),
		"3c66043e-4d4e-4009-b73f-2d4d483746af")
	if err == nil {
		t.Fatal("expected an error from a 500 delete")
	}
	if auth.IsUserNotFound(err) {
		t.Errorf("a 500 was classified as user-not-found: %v", err)
	}
}
