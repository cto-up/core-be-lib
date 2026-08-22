package service

import (
	"testing"
	"time"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/shared/auth"
)

func tp(t time.Time) *time.Time { return &t }

// last_seen_at must never come out older than last_authenticated_at: signing in
// is activity, recorded instantly by the provider, while our own stamp is
// throttled and lags. The observed failure was "Last active 1 minute ago, Last
// sign-in 33 seconds ago" — 28s apart, straight out of the throttle window.
func TestLastSeenNeverPredatesSignIn(t *testing.T) {
	stamped := time.Date(2026, 8, 22, 15, 58, 46, 0, time.UTC)
	signedIn := time.Date(2026, 8, 22, 15, 59, 14, 0, time.UTC)

	cases := []struct {
		name       string
		lastSeen   *time.Time
		signIn     *time.Time
		wantActive *time.Time
	}{
		{"sign-in newer than a throttled stamp", tp(stamped), tp(signedIn), tp(signedIn)},
		{"stamp newer than an old sign-in", tp(signedIn), tp(stamped), tp(signedIn)},
		{"no stamp yet falls back to the sign-in", nil, tp(signedIn), tp(signedIn)},
		{"no sign-in keeps the stamp", tp(stamped), nil, tp(stamped)},
		{"neither stays empty", nil, nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := core.User{Id: "u1", LastSeenAt: tc.lastSeen}
			applyActivity(&u, auth.UserActivity{
				Found: true, State: "active", LastAuthenticatedAt: tc.signIn,
			})

			switch {
			case tc.wantActive == nil && u.LastSeenAt != nil:
				t.Fatalf("LastSeenAt = %v, want nil", u.LastSeenAt)
			case tc.wantActive != nil && u.LastSeenAt == nil:
				t.Fatalf("LastSeenAt = nil, want %v", tc.wantActive)
			case tc.wantActive != nil && !u.LastSeenAt.Equal(*tc.wantActive):
				t.Fatalf("LastSeenAt = %v, want %v", u.LastSeenAt, tc.wantActive)
			}

			if u.LastSeenAt != nil && u.LastAuthenticatedAt != nil &&
				u.LastSeenAt.Before(*u.LastAuthenticatedAt) {
				t.Errorf("last active %v predates sign-in %v",
					u.LastSeenAt, u.LastAuthenticatedAt)
			}
		})
	}
}

// A row with no identity must not be given verification or timing facts — they
// are the zero value, not observations about a person.
func TestApplyActivityLeavesMissingRowsBare(t *testing.T) {
	u := core.User{Id: "La5Ba4NAlSRy9tFx5AIMe6xPQMt1"}
	applyActivity(&u, auth.UserActivity{State: auth.UserActivityStateMissing})

	if u.AuthState == nil || *u.AuthState != auth.UserActivityStateMissing {
		t.Errorf("AuthState = %v, want %q", u.AuthState, auth.UserActivityStateMissing)
	}
	if u.EmailVerified != nil {
		t.Errorf("EmailVerified = %v, want nil", u.EmailVerified)
	}
	if u.LastSeenAt != nil || u.LastAuthenticatedAt != nil {
		t.Errorf("timings = %v/%v, want both nil", u.LastSeenAt, u.LastAuthenticatedAt)
	}
}
