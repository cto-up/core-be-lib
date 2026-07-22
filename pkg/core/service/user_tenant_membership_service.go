package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	sharedservice "ctoup.com/coreapp/pkg/shared/service"
)

// UserTenantMembershipService is the self-service side of membership: invite a
// person to a workspace, and let them accept or decline it themselves.
//
// This is deliberately distinct from the ADMIN path (UserAdminHandler /
// AddUserToTenant), which adds somebody outright. The difference is consent —
// an invitation is an offer the recipient answers, which is what makes it usable
// for onboarding people who do not yet work for you.
const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusRejected = "rejected"

	// InvitationTTL bounds how long an unanswered invitation stays open. An
	// invitation that lives forever is a standing grant nobody remembers issuing.
	InvitationTTL = 14 * 24 * time.Hour
)

var (
	ErrUserNotFound      = errors.New("no platform user with that email")
	ErrAlreadyMember     = errors.New("already an active member of this workspace")
	ErrNoInvitation      = errors.New("no pending invitation for this workspace")
	ErrInvitationExpired = errors.New("this invitation has expired")
)

type UserTenantMembershipService struct {
	store *db.Store
}

func NewUserTenantMembershipService(store *db.Store) *UserTenantMembershipService {
	return &UserTenantMembershipService{store: store}
}

// GetUserTenants lists the workspaces a person is an active member of.
func (s *UserTenantMembershipService) GetUserTenants(ctx context.Context, userID string) ([]repository.ListUserTenantMembershipsRow, error) {
	return s.store.ListUserTenantMemberships(ctx, repository.ListUserTenantMembershipsParams{
		UserID: userID, Status: StatusActive,
	})
}

// GetPendingInvitations lists invitations awaiting this person's answer.
// Expired ones are filtered out rather than offered — accepting one would fail,
// and showing an offer that cannot be taken is worse than showing nothing.
func (s *UserTenantMembershipService) GetPendingInvitations(ctx context.Context, userID string) ([]repository.ListPendingInvitationsRow, error) {
	rows, err := s.store.ListPendingInvitations(ctx, userID)
	if err != nil {
		return nil, err
	}
	live := rows[:0]
	for _, r := range rows {
		if !invitationExpired(r.InvitedAt) {
			live = append(live, r)
		}
	}
	return live, nil
}

// InviteUser offers an existing platform user a place in a tenant.
//
// The invitee must already have an account: creating an identity for a stranger
// is the admin user-creation path's job (it needs the auth provider, a password
// flow and email verification), and conflating the two would make a simple
// invite carry all of that weight.
func (s *UserTenantMembershipService) InviteUser(ctx context.Context, email, tenantID string, roles []core.Role, inviterID string) (repository.CoreUserTenantMembership, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return repository.CoreUserTenantMembership{}, errors.New("an email is required")
	}
	if len(roles) == 0 {
		roles = []core.Role{core.USER}
	}

	user, err := s.store.GetUserByEmailGlobal(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.CoreUserTenantMembership{}, ErrUserNotFound
		}
		return repository.CoreUserTenantMembership{}, err
	}

	// Re-inviting an active member is a no-op the caller should hear about,
	// rather than something that silently downgrades them to pending.
	if existing, err := s.store.GetSharedUserTenantMembership(ctx, repository.GetSharedUserTenantMembershipParams{
		UserID: user.ID, TenantID: tenantID,
	}); err == nil && existing.Status == StatusActive {
		return repository.CoreUserTenantMembership{}, ErrAlreadyMember
	}

	// A pending invitation holds a seat. Without this, invitations are a way
	// around max_learners: issue a thousand, watch them all accept.
	if err := sharedservice.CheckSeatGuard(ctx, tenantID, roles); err != nil {
		return repository.CoreUserTenantMembership{}, err
	}

	return s.store.AddSharedUserToTenant(ctx, repository.AddSharedUserToTenantParams{
		UserID:      user.ID,
		TenantID:    tenantID,
		TenantRoles: rolesToStrings(roles),
		Status:      StatusPending,
		InvitedBy:   pgtype.Text{String: inviterID, Valid: inviterID != ""},
		InvitedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
}

// AcceptInvitation turns a pending invitation into membership.
func (s *UserTenantMembershipService) AcceptInvitation(ctx context.Context, userID, tenantID string) error {
	m, err := s.store.GetSharedUserTenantMembership(ctx, repository.GetSharedUserTenantMembershipParams{
		UserID: userID, TenantID: tenantID,
	})
	if err != nil || m.Status != StatusPending {
		return ErrNoInvitation
	}
	if invitationExpired(m.InvitedAt) {
		return ErrInvitationExpired
	}

	// Re-checked at accept, not only at invite. Seats can be consumed between
	// the two by anyone else accepting, and the invite-time check would then be
	// stale — this is the point where the seat is actually taken.
	if err := sharedservice.CheckSeatGuard(ctx, tenantID, stringsToRoles(m.Roles)); err != nil {
		return err
	}

	_, err = s.store.UpdateUserTenantMembershipJoinedAt(ctx, repository.UpdateUserTenantMembershipJoinedAtParams{
		UserID: userID, TenantID: tenantID,
		JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	return err
}

// RejectInvitation declines it. The row is kept as 'rejected' rather than
// deleted so re-inviting somebody who already said no is a visible act.
func (s *UserTenantMembershipService) RejectInvitation(ctx context.Context, userID, tenantID string) error {
	m, err := s.store.GetSharedUserTenantMembership(ctx, repository.GetSharedUserTenantMembershipParams{
		UserID: userID, TenantID: tenantID,
	})
	if err != nil || m.Status != StatusPending {
		return ErrNoInvitation
	}
	_, err = s.store.UpdateUserTenantMembershipStatus(ctx, repository.UpdateUserTenantMembershipStatusParams{
		UserID: userID, TenantID: tenantID, Status: StatusRejected,
	})
	return err
}

// ListMembers returns a workspace's active members.
func (s *UserTenantMembershipService) ListMembers(ctx context.Context, tenantID string) ([]repository.CoreUserTenantMembership, error) {
	return s.store.ListTenantMembers(ctx, repository.ListTenantMembersParams{
		TenantID: tenantID, Status: StatusActive,
	})
}

// UpdateMemberRoles changes what a member may do in this workspace.
func (s *UserTenantMembershipService) UpdateMemberRoles(ctx context.Context, tenantID, userID string, roles []core.Role) error {
	if len(roles) == 0 {
		return errors.New("at least one role is required")
	}
	_, err := s.store.UpdateUserTenantMembershipRoles(ctx, repository.UpdateUserTenantMembershipRolesParams{
		UserID: userID, TenantID: tenantID, Roles: rolesToStrings(roles),
	})
	return err
}

// RemoveMember revokes membership outright.
func (s *UserTenantMembershipService) RemoveMember(ctx context.Context, tenantID, userID string) error {
	return s.store.RemoveSharedUserFromTenant(ctx, repository.RemoveSharedUserFromTenantParams{
		UserID: userID, TenantID: tenantID,
	})
}

// invitationExpired reports whether an unanswered invitation has aged out. A
// missing invited_at is treated as live: it predates invitations, and refusing
// it would strand a legitimate membership.
func invitationExpired(invitedAt pgtype.Timestamptz) bool {
	if !invitedAt.Valid {
		return false
	}
	return time.Since(invitedAt.Time) > InvitationTTL
}

func rolesToStrings(roles []core.Role) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, string(r))
	}
	return out
}

func stringsToRoles(in []string) []core.Role {
	out := make([]core.Role, 0, len(in))
	for _, r := range in {
		out = append(out, core.Role(r))
	}
	return out
}

var _ = fmt.Sprintf
