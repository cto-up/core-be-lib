package kratos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ctoup.com/coreapp/pkg/shared/auth"
	ory "github.com/ory/kratos-client-go"
)

// newTestClient points a KratosAuthClient at a stub admin API.
func newTestClient(t *testing.T, h http.Handler) *KratosAuthClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	return NewKratosAuthClient(ory.NewAPIClient(cfg), ory.NewAPIClient(cfg))
}

// A page of users can mix Kratos UUIDs with pre-Kratos Firebase uids, which are
// not UUIDs. Kratos answers 400 for the whole batch if one slips through, which
// used to blank status and last sign-in for every user on the page. The legacy
// ids must be dropped before the call, and the real ones must still resolve.
func TestGetUserActivitySkipsNonUUIDIDs(t *testing.T) {
	const realID = "3c66043e-4d4e-4009-b73f-2d4d483746af"
	const legacyID = "La5Ba4NAlSRy9tFx5AIMe6xPQMt1"
	authenticatedAt := time.Date(2026, 8, 22, 13, 45, 0, 0, time.UTC)

	var gotIDs []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/identities":
			gotIDs = r.URL.Query()["ids"]
			for _, id := range gotIDs {
				// Mirror the real server: any non-UUID poisons the batch.
				if id == legacyID {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":{"code":400}}`))
					return
				}
			}
			state := "active"
			_ = json.NewEncoder(w).Encode([]ory.Identity{{
				Id:     realID,
				State:  &state,
				Traits: map[string]interface{}{"email": "jc@example.com"},
				VerifiableAddresses: []ory.VerifiableIdentityAddress{
					{Value: "jc@example.com", Verified: true},
				},
			}})
		case "/admin/identities/" + realID + "/sessions":
			_ = json.NewEncoder(w).Encode([]ory.Session{
				{Id: "s1", AuthenticatedAt: &authenticatedAt},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := newTestClient(t, h).GetUserActivity(context.Background(),
		[]string{legacyID, realID})
	if err != nil {
		t.Fatalf("GetUserActivity: %v", err)
	}

	if len(gotIDs) != 1 || gotIDs[0] != realID {
		t.Errorf("sent ids = %v, want only the UUID %q", gotIDs, realID)
	}
	legacy, ok := got[legacyID]
	if !ok {
		t.Fatalf("legacy id %q should be reported, not silently dropped", legacyID)
	}
	if legacy.Found || legacy.State != auth.UserActivityStateMissing {
		t.Errorf("legacy id = %+v, want not-found and state %q",
			legacy, auth.UserActivityStateMissing)
	}

	real, ok := got[realID]
	if !ok {
		t.Fatalf("real identity missing from result; the legacy id blanked the page")
	}
	if real.State != "active" || !real.EmailVerified {
		t.Errorf("got %+v, want active and verified", real)
	}
	if real.LastAuthenticatedAt == nil || !real.LastAuthenticatedAt.Equal(authenticatedAt) {
		t.Errorf("LastAuthenticatedAt = %v, want %v", real.LastAuthenticatedAt, authenticatedAt)
	}
}

// Every id on the page can be legacy — the global admin list is exactly that
// today. Kratos must not be called at all, and the result is simply empty.
func TestGetUserActivityAllLegacyIDs(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusBadRequest)
	})

	got, err := newTestClient(t, h).GetUserActivity(context.Background(),
		[]string{"La5Ba4NAlSRy9tFx5AIMe6xPQMt1", "RhWd8mOwV3b4x1xqqVzptfe15j53"})
	if err != nil {
		t.Fatalf("GetUserActivity: %v", err)
	}
	if called {
		t.Error("Kratos was called with no valid UUID to ask about")
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want both ids reported as missing", got)
	}
	for id, a := range got {
		if a.Found || a.State != auth.UserActivityStateMissing {
			t.Errorf("%s = %+v, want not-found and state %q",
				id, a, auth.UserActivityStateMissing)
		}
	}
}
