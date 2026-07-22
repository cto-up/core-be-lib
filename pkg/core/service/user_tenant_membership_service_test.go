package service

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ctoup.com/coreapp/api/openapi/core"
)

// An invitation that lives forever is a standing grant nobody remembers issuing.
func TestInvitationExpiry(t *testing.T) {
	fresh := pgtype.Timestamptz{Time: time.Now().Add(-1 * time.Hour), Valid: true}
	if invitationExpired(fresh) {
		t.Fatal("an hour-old invitation must still be live")
	}

	stale := pgtype.Timestamptz{Time: time.Now().Add(-InvitationTTL - time.Hour), Valid: true}
	if !invitationExpired(stale) {
		t.Fatal("past the TTL an invitation must be expired")
	}

	// Rows predating invitations have no invited_at. Treating those as expired
	// would strand legitimate memberships created by the admin path.
	if invitationExpired(pgtype.Timestamptz{}) {
		t.Fatal("a missing invited_at must be treated as live, not expired")
	}

	// Exactly at the boundary is still live — expiry is "past the TTL".
	edge := pgtype.Timestamptz{Time: time.Now().Add(-InvitationTTL + time.Minute), Valid: true}
	if invitationExpired(edge) {
		t.Fatal("just inside the TTL must be live")
	}
}

func TestRoleConversionRoundTrips(t *testing.T) {
	in := []core.Role{core.USER, core.ADMIN}
	got := stringsToRoles(rolesToStrings(in))
	if len(got) != len(in) || got[0] != core.USER || got[1] != core.ADMIN {
		t.Fatalf("roles did not round-trip: %v -> %v", in, got)
	}
	if len(rolesToStrings(nil)) != 0 {
		t.Fatal("nil roles must convert to an empty slice, not nil-with-length")
	}
}

// The three states are distinct strings the queries filter on; a typo here
// would silently make invitations invisible.
func TestMembershipStatusesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []string{StatusPending, StatusActive, StatusRejected} {
		if s == "" || seen[s] {
			t.Fatalf("status %q is empty or duplicated", s)
		}
		seen[s] = true
	}
}
