package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/emailservice"
	sharedservice "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
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
func (s *UserTenantMembershipService) InviteUser(ctx context.Context, email, tenantID string, roles []core.Role, inviterID, acceptURL string) (repository.CoreUserTenantMembership, error) {
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

	m, err := s.store.AddSharedUserToTenant(ctx, repository.AddSharedUserToTenantParams{
		UserID:      user.ID,
		TenantID:    tenantID,
		TenantRoles: rolesToStrings(roles),
		Status:      StatusPending,
		InvitedBy:   pgtype.Text{String: inviterID, Valid: inviterID != ""},
		InvitedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return repository.CoreUserTenantMembership{}, err
	}

	// Best-effort: the invitation EXISTS whether or not the mail goes out, and
	// failing the request would leave the admin thinking nothing happened while
	// a pending row sits there. Logged loudly instead — an invitation nobody was
	// told about is the failure this whole change exists to prevent, so it must
	// be visible.
	s.sendInvitationEmail(ctx, email, tenantID, inviterID, acceptURL)
	return m, nil
}

// sendInvitationEmail tells the invitee. Without this the flow is an API that
// works and a person who never finds out.
func (s *UserTenantMembershipService) sendInvitationEmail(ctx context.Context, email, tenantID, inviterID, acceptURL string) {
	logger := util.GetLoggerFromCtx(ctx)
	if acceptURL == "" {
		logger.Warn().Str("tenant_id", tenantID).
			Msg("membership: invitation created but no accept URL was built; no email sent")
		return
	}

	tenantName := tenantID
	if t, err := s.store.GetTenantByTenantID(ctx, tenantID); err == nil && t.Name != "" {
		tenantName = t.Name
	}

	// "Alice invited you" beats "You have been invited" — a name tells the reader
	// whether they were expecting this. Falls back to the impersonal form rather
	// than printing an opaque user id.
	inviterLine := "You have been "
	if inviterID != "" {
		if u, err := s.store.GetSharedUserByID(ctx, inviterID); err == nil {
			if name := strings.TrimSpace(u.Email.String); u.Email.Valid && name != "" {
				inviterLine = name + " has "
			}
		}
	}

	data := struct {
		Link          string
		TenantName    string
		InviterLine   string
		ExpiresInDays int
	}{
		Link:          acceptURL,
		TenantName:    tenantName,
		InviterLine:   inviterLine,
		ExpiresInDays: int(InvitationTTL / (24 * time.Hour)),
	}

	r := emailservice.NewEmailRequest(systemEmailFrom(), []string{email},
		"You have been invited to "+tenantName, "")
	if err := r.ParseTemplate("email-tenant-invitation.html", data); err != nil {
		logger.Err(err).Str("tenant_id", tenantID).Msg("membership: could not render the invitation email")
		return
	}
	if err := r.SendEmail(); err != nil {
		logger.Err(err).Str("tenant_id", tenantID).
			Msg("membership: INVITATION CREATED BUT EMAIL NOT SENT — the invitee does not know")
		return
	}
	logger.Info().Str("tenant_id", tenantID).Msg("membership: invitation email sent")
}

// systemEmailFrom mirrors the from-address resolution the rest of core uses.
func systemEmailFrom() string {
	if from := os.Getenv("SYSTEM_EMAIL"); from != "" {
		return from
	}
	return "noreply@ctoup.com"
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
