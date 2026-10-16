package kratos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ory "github.com/ory/kratos-client-go"

	"ctoup.com/coreapp/pkg/shared/auth"
)

// hub#184: both the user record and the AuthenticatedUser of every request
// said email_verified=true for any identity, so nothing could ever ask a user
// to verify their address.

var emailVerifiedCases = []struct {
	name      string
	addresses string
	want      bool
}{
	{"no verifiable address", `[]`, false},
	{"an unverified address", `[{"value":"who@example.com","verified":false,"via":"email","status":"sent"}]`, false},
	{"a verified address", `[{"value":"who@example.com","verified":true,"via":"email","status":"completed"}]`, true},
}

func identityJSON(addresses string) string {
	return `{"id":"9d4b7e10-0000-4000-8000-000000000002","schema_id":"default",` +
		`"schema_url":"http://kratos.test/schemas/default","state":"active",` +
		`"traits":{"email":"who@example.com"},` +
		`"metadata_public":{"global_roles":["ADMIN"]},` +
		`"verifiable_addresses":` + addresses + `}`
}

func TestUserRecordEmailVerifiedFollowsVerifiableAddresses(t *testing.T) {
	for _, tc := range emailVerifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			var ident ory.Identity
			if err := ident.UnmarshalJSON([]byte(identityJSON(tc.addresses))); err != nil {
				t.Fatalf("decode identity: %v", err)
			}
			if got := convertKratosIdentityToUserRecord(&ident).EmailVerified; got != tc.want {
				t.Errorf("EmailVerified = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVerifyTokenWithTenantIDEmailVerifiedFollowsVerifiableAddresses(t *testing.T) {
	for _, tc := range emailVerifiedCases {
		t.Run(tc.name, func(t *testing.T) {
			session := fmt.Sprintf(`{"id":"5f2a1c88-0000-4000-8000-000000000001","active":true,"identity":%s}`,
				identityJSON(tc.addresses))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(session))
			}))
			t.Cleanup(srv.Close)

			cfg := ory.NewConfiguration()
			cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
			client := ory.NewAPIClient(cfg)
			provider := &KratosAuthProvider{adminClient: client, publicClient: client}

			// A global ADMIN returns before any tenant lookup, so no tenancy
			// service is needed to reach the user.
			user, err := provider.VerifyTokenWithTenantID(context.Background(), "", "cookie")
			if err != nil {
				t.Fatalf("verify failed: %v", err)
			}
			if user.EmailVerified != tc.want {
				t.Errorf("EmailVerified = %v, want %v", user.EmailVerified, tc.want)
			}
		})
	}
}

// hub#555. Core's own verification link calls UpdateUser(EmailVerified(true)),
// which only ever wrote traits and password, so once EmailVerified was read
// from Kratos the user stayed unverified forever.
func TestUpdateUserEmailVerifiedReachesTheVerifiableAddress(t *testing.T) {
	verified := false
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			var patch []ory.JsonPatch
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Errorf("decode patch: %v", err)
			}
			for _, op := range patch {
				if op.Op == "replace" && op.Path == "/verifiable_addresses/0/verified" {
					verified, _ = op.Value.(bool)
				}
			}
		}
		_, _ = w.Write([]byte(identityJSON(fmt.Sprintf(
			`[{"value":"who@example.com","verified":%v,"via":"email","status":"sent"}]`, verified))))
	}))
	ctx := context.Background()
	const uid = "9d4b7e10-0000-4000-8000-000000000002"

	rec, err := client.UpdateUser(ctx, uid, (&auth.UserToUpdate{}).EmailVerified(true))
	if err != nil {
		t.Fatal(err)
	}
	if !rec.EmailVerified {
		t.Error("UpdateUser(EmailVerified(true)) returned an unverified user")
	}
	if got, _ := client.GetUser(ctx, uid); !got.EmailVerified {
		t.Error("the identity read back is still unverified")
	}
}
