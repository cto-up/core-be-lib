package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	sharedservice "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// DormancyWindow is how long a departed member's tenant-scoped data is kept
	// so that returning restores their history. The membership row itself
	// outlives it — that row is the return path and is never purged.
	DormancyWindow = 90 * 24 * time.Hour

	// DeletionGrace is the window between asking to close an account and the
	// deletion actually running. Signing in during it cancels.
	DeletionGrace = 30 * 24 * time.Hour

	// RejoinCooldown caps leave/rejoin churn. Without it the pair is a way to
	// re-fire the per-tenant onboarding callbacks — initial credit grants
	// included — and to thrash seats.
	RejoinCooldown = 24 * time.Hour

	// sweepPage bounds one sweep pass. A backlog drains over successive runs
	// rather than in one transaction that could run for minutes.
	sweepPage = 200
)

var (
	ErrNoActiveMembership   = errors.New("no active membership of this tenant")
	ErrDecisionsOutstanding = errors.New("an impact requires a decision")
	ErrDeletionScheduled    = errors.New("a deletion is already scheduled")
	ErrConfirmEmailMismatch = errors.New("confirmEmail does not match the caller")
	ErrRejoinTooSoon        = errors.New("rejoining is rate limited")
)

// MembershipLifecycleService owns leaving a tenant and closing an account.
//
// It is deliberately ignorant of what leaving costs: the counts, the decisions
// and the erasures all come from modules that registered a UserDataContributor,
// because core has never heard of an enrollment (ADR 040 §10).
type MembershipLifecycleService struct {
	store              *db.Store
	authProvider       auth.AuthProvider
	warnInactive       func(u repository.CoreUser, monthsInactive int, closesOn time.Time) error
	notifyAdminClosure func(u repository.CoreUser, closure AdminClosure, deletesOn time.Time) error
}

func NewMembershipLifecycleService(store *db.Store, authProvider auth.AuthProvider) *MembershipLifecycleService {
	return &MembershipLifecycleService{
		store:              store,
		authProvider:       authProvider,
		warnInactive:       sendInactivityWarning,
		notifyAdminClosure: sendAdminClosureNotice,
	}
}

// LeavePreview reports what leaving would cost. Read at dialog-open time so the
// confirmation states real numbers; a generic warning teaches people to click
// through warnings.
func (s *MembershipLifecycleService) LeavePreview(ctx context.Context, userID, tenantID string) (core.LeaveTenantPreview, error) {
	if _, err := s.activeMembership(ctx, userID, tenantID); err != nil {
		return core.LeaveTenantPreview{}, err
	}

	impacts := s.collectImpacts(ctx, userID, tenantID)
	dormantUntil := time.Now().Add(DormancyWindow)

	return core.LeaveTenantPreview{
		CanLeaveNow:  !hasOutstandingDecision(impacts, nil),
		DormantUntil: &dormantUntil,
		Impacts:      impacts,
	}, nil
}

// Leave ends the caller's own membership of one tenant.
//
// The claim goes first. VerifyTokenWithTenantID reads
// metadata_public.tenant_memberships and never the database, and VerifyIDToken
// re-reads it from Kratos on every request — so rewriting it revokes access on
// the very next call, while a row-only update would report success and revoke
// nothing. Sessions are left alone on purpose: a Kratos session belongs to the
// identity, not to a tenant, so revoking would sign the member out of the
// organizations they did not leave.
func (s *MembershipLifecycleService) Leave(ctx context.Context, userID, tenantID string, req core.LeaveTenantRequest) (core.LeaveTenantResult, error) {
	logger := util.GetLoggerFromCtx(ctx)

	if _, err := s.activeMembership(ctx, userID, tenantID); err != nil {
		return core.LeaveTenantResult{}, err
	}

	var decisions []core.LeaveDecision
	if req.Decisions != nil {
		decisions = *req.Decisions
	}

	impacts := s.collectImpacts(ctx, userID, tenantID)
	if hasOutstandingDecision(impacts, decisions) {
		return core.LeaveTenantResult{}, ErrDecisionsOutstanding
	}

	// Decisions are applied BEFORE the membership ends. A transfer that fails
	// after the member is gone leaves published courses orphaned with nobody
	// authorised to fix them.
	for _, c := range sharedservice.UserDataContributors() {
		if err := c.ApplyLeaveDecisions(ctx, userID, tenantID, decisions); err != nil {
			logger.Err(err).Str("contributor", c.Name()).Msg("leave: failed to apply decisions")
			return core.LeaveTenantResult{}, fmt.Errorf("%s: %w", c.Name(), err)
		}
	}

	authClient := s.authProvider.GetAuthClient()
	if err := authClient.RemoveTenantMembershipClaim(ctx, userID, tenantID); err != nil {
		logger.Err(err).Str("user_id", userID).Str("tenant_id", tenantID).
			Msg("leave: failed to remove the membership claim — access NOT revoked, aborting")
		return core.LeaveTenantResult{}, err
	}

	if _, err := s.store.DeleteSharedUserByTenant(ctx, repository.DeleteSharedUserByTenantParams{
		UserID: userID, TenantID: tenantID,
	}); err != nil {
		logger.Err(err).Msg("leave: claim removed but membership row not updated")
		return core.LeaveTenantResult{}, err
	}

	dormantUntil := time.Now().Add(DormancyWindow)
	seatReleased := true
	logger.Info().Str("user_id", userID).Str("tenant_id", tenantID).Msg("leave: membership ended")

	return core.LeaveTenantResult{
		MembershipStatus: core.LeaveTenantResultMembershipStatusInactive,
		DormantUntil:     &dormantUntil,
		SeatReleased:     &seatReleased,
	}, nil
}

// CheckRejoinCooldown refuses a rejoin that follows a departure too closely.
// Consulted from the membership add path so every way back in is covered.
func (s *MembershipLifecycleService) CheckRejoinCooldown(ctx context.Context, userID, tenantID string) error {
	m, err := s.store.GetUserTenantMembership(ctx, repository.GetUserTenantMembershipParams{
		UserID: userID, TenantID: tenantID,
	})
	if err != nil {
		return nil // no prior membership: nothing to rate limit
	}
	if m.Status != "inactive" {
		return nil
	}
	if time.Since(m.UpdatedAt) < RejoinCooldown {
		return ErrRejoinTooSoon
	}
	return nil
}

// DeletionStatus answers the SPA's sign-in interception.
func (s *MembershipLifecycleService) DeletionStatus(ctx context.Context, userID string) (core.AccountDeletion, error) {
	user, err := s.store.GetSharedUserByID(ctx, userID)
	if err != nil {
		return core.AccountDeletion{}, err
	}
	return deletionStatusOf(user, s.countMemberships(ctx, userID)), nil
}

// ClosurePreview asks every contributor what closing would cost, in every tenant
// the caller still belongs to.
//
// Leaving asks about one organization; closing ends them all, so the same
// questions are asked once per membership rather than discovered one at a time
// after the fact.
func (s *MembershipLifecycleService) ClosurePreview(ctx context.Context, userID string) (core.AccountClosurePreview, error) {
	memberships, err := s.store.ListUserTenantMemberships(ctx, repository.ListUserTenantMembershipsParams{
		UserID: userID, Status: "active",
	})
	if err != nil {
		return core.AccountClosurePreview{}, err
	}

	out := core.AccountClosurePreview{Tenants: []core.TenantClosureImpact{}}
	for _, m := range memberships {
		impacts := s.collectImpacts(ctx, userID, m.TenantID)
		if len(impacts) == 0 {
			continue
		}
		subdomain := m.Subdomain
		out.Tenants = append(out.Tenants, core.TenantClosureImpact{
			TenantId:   m.TenantID,
			TenantName: m.TenantName,
			Subdomain:  &subdomain,
			Impacts:    impacts,
		})
	}
	return out, nil
}

// ScheduleDeletion closes the account across every tenant — as a date, not as a
// deletion. Nothing is destroyed here: the grace period is what makes an account
// takeover non-destructive and a change of mind cheap.
func (s *MembershipLifecycleService) ScheduleDeletion(ctx context.Context, userID string, req core.AccountDeletionRequest) (core.AccountDeletion, error) {
	user, err := s.store.GetSharedUserByID(ctx, userID)
	if err != nil {
		return core.AccountDeletion{}, err
	}
	if user.DeletionScheduledAt.Valid {
		return deletionStatusOf(user, s.countMemberships(ctx, userID)), ErrDeletionScheduled
	}

	// Checked server-side. The typed confirmation is a real check, not a
	// client-side speed bump: this is the one irreversible door in the product.
	if !strings.EqualFold(strings.TrimSpace(string(req.ConfirmEmail)), strings.TrimSpace(user.Email.String)) {
		return core.AccountDeletion{}, ErrConfirmEmailMismatch
	}

	reason := pgtype.Text{}
	if req.Reason != nil && *req.Reason != "" {
		reason = pgtype.Text{String: *req.Reason, Valid: true}
	}
	decisions := []core.TenantLeaveDecision{}
	if req.Decisions != nil {
		decisions = *req.Decisions
	}
	return s.schedule(ctx, user, reason, decisions)
}

// AdminClosureReason is the deletion_reason of an account an administrator
// closed for its holder.
const AdminClosureReason = "closed by an administrator"

// AdminClosure is what the holder is told when an administrator closes their
// account: which organization acted, and where to keep the account.
type AdminClosure struct {
	Reason     string
	TenantName string
	KeepURL    string
}

// ScheduleDeletionByAdmin is ScheduleDeletion performed for the holder: no
// typed confirmation (it is the administrator's, not the holder's), and the
// holder is emailed the date so they can keep the account the usual way.
func (s *MembershipLifecycleService) ScheduleDeletionByAdmin(ctx context.Context, userID string, closure AdminClosure) (core.AccountDeletion, error) {
	logger := util.GetLoggerFromCtx(ctx)

	user, err := s.store.GetSharedUserByID(ctx, userID)
	if err != nil {
		return core.AccountDeletion{}, err
	}
	if user.DeletionScheduledAt.Valid {
		return deletionStatusOf(user, s.countMemberships(ctx, userID)), ErrDeletionScheduled
	}

	reason := AdminClosureReason
	if r := strings.TrimSpace(closure.Reason); r != "" {
		reason += ": " + r
	}
	status, err := s.schedule(ctx, user, pgtype.Text{String: reason, Valid: true}, []core.TenantLeaveDecision{})
	if err != nil {
		return status, err
	}
	// The closure stands without the email: the pending-deletions list shows it
	// and signing in shows the holder the way back.
	if err := s.notifyAdminClosure(user, closure, *status.ScheduledFor); err != nil {
		logger.Err(err).Str("user_id", userID).Msg("close account by admin: notice not sent")
	}
	return status, nil
}

func (s *MembershipLifecycleService) schedule(ctx context.Context, user repository.CoreUser, reason pgtype.Text, decisions []core.TenantLeaveDecision) (core.AccountDeletion, error) {
	logger := util.GetLoggerFromCtx(ctx)
	userID := user.ID
	scheduledFor := time.Now().Add(DeletionGrace)

	// Memberships are deliberately NOT ended here, and no session is revoked.
	//
	// The grace period only exists if the person can still reach the cancel
	// button, and DELETE /users/me/deletion sits behind the auth middleware,
	// which rejects any session whose identity holds no membership claim for the
	// host's tenant. Ending the memberships now would make the account
	// unrecoverable the moment it became recoverable-in-principle — a 30-day
	// window nobody can open.
	//
	// So closing an account is a DATE until the sweeper runs. Somebody who wants
	// access gone today has the other door: leave each organization, which does
	// revoke immediately.
	updated, err := s.store.ScheduleUserDeletion(ctx, repository.ScheduleUserDeletionParams{
		ID:                  userID,
		DeletionScheduledAt: pgtype.Timestamptz{Time: scheduledFor, Valid: true},
		DeletionReason:      reason,
	})
	if err != nil {
		return core.AccountDeletion{}, err
	}

	// Recorded, not applied. Applying now would transfer somebody's courses away
	// during a window whose whole point is that they can change their mind.
	encoded, encodeErr := json.Marshal(decisions)
	if encodeErr != nil {
		return core.AccountDeletion{}, encodeErr
	}
	if err := s.store.SetUserDeletionDecisions(ctx, repository.SetUserDeletionDecisionsParams{
		ID: userID, DeletionDecisions: encoded,
	}); err != nil {
		return core.AccountDeletion{}, err
	}

	logger.Info().Str("user_id", userID).Time("scheduled_for", scheduledFor).Msg("close account: deletion scheduled")
	return deletionStatusOf(updated, s.countMemberships(ctx, userID)), nil
}

// CancelDeletion keeps the account, restoring exactly the memberships the
// scheduling ended. Idempotent: with nothing scheduled it reports status none.
func (s *MembershipLifecycleService) CancelDeletion(ctx context.Context, userID string) (core.AccountDeletion, error) {
	logger := util.GetLoggerFromCtx(ctx)

	// Nothing to put back: scheduling ended no membership, precisely so this
	// call is reachable. Clearing the stamp is the whole cancellation.
	user, err := s.store.CancelUserDeletion(ctx, userID)
	if err != nil {
		return core.AccountDeletion{}, err
	}
	logger.Info().Str("user_id", userID).Msg("close account: deletion cancelled")
	return deletionStatusOf(user, s.countMemberships(ctx, userID)), nil
}

// Export assembles what core owns plus one section per registered contributor.
func (s *MembershipLifecycleService) Export(ctx context.Context, userID string) (core.UserDataExport, error) {

	user, err := s.store.GetSharedUserByID(ctx, userID)
	if err != nil {
		return core.UserDataExport{}, err
	}
	memberships, err := s.store.ListAllUserTenantMemberships(ctx, userID)
	if err != nil {
		return core.UserDataExport{}, err
	}

	modules, failed := exportSections(ctx, userID, sharedservice.UserDataContributors())

	out := core.UserDataExport{
		ExportedAt: time.Now(),
		User: core.User{
			Id:        user.ID,
			Email:     user.Email.String,
			Name:      user.Profile.Name,
			Roles:     []core.Role{},
			CreatedAt: &user.CreatedAt,
		},
		Memberships: toTenantMemberships(memberships),
		Modules:     modules,
	}
	if len(failed) > 0 {
		out.FailedSections = &failed
	}
	return out, nil
}

// exportSections asks every contributor for its section. A failed section is
// logged and named in the returned failures with its cause: an export that
// quietly drops a module is a false statement about what we hold.
func exportSections(ctx context.Context, userID string, contributors []sharedservice.UserDataContributor) (map[string]interface{}, []core.UserDataExportFailure) {
	logger := util.GetLoggerFromCtx(ctx)
	modules := map[string]interface{}{}
	var failed []core.UserDataExportFailure
	for _, c := range contributors {
		section, err := c.Export(ctx, userID)
		if err != nil {
			logger.Err(err).Str("contributor", c.Name()).Msg("export: contributor failed, section reported as failed")
			failed = append(failed, core.UserDataExportFailure{Name: c.Name(), Error: err.Error()})
			continue
		}
		modules[c.Name()] = section
	}
	return modules, failed
}

// toTenantMemberships converts the sqlc rows to the wire type. The wire is
// snake_case here because this payload predates its spec and a live SPA reads
// it; renaming is its own breaking decision (ADR 040 §8).
func toTenantMemberships(rows []repository.ListAllUserTenantMembershipsRow) []core.TenantMembership {
	out := make([]core.TenantMembership, 0, len(rows))
	for _, r := range rows {
		roles := make([]core.Role, 0, len(r.Roles))
		for _, role := range r.Roles {
			roles = append(roles, core.Role(role))
		}
		m := core.TenantMembership{
			Id:         r.ID,
			UserId:     r.UserID,
			TenantId:   r.TenantID,
			Status:     core.TenantMembershipStatus(r.Status),
			Roles:      &roles,
			TenantName: r.TenantName,
			Subdomain:  r.Subdomain,
			CreatedAt:  &r.CreatedAt,
			UpdatedAt:  &r.UpdatedAt,
		}
		if r.InvitedBy.Valid {
			invitedBy := r.InvitedBy.String
			m.InvitedBy = &invitedBy
		}
		if r.InvitedAt.Valid {
			invitedAt := r.InvitedAt.Time
			m.InvitedAt = &invitedAt
		}
		if r.JoinedAt.Valid {
			joinedAt := r.JoinedAt.Time
			m.JoinedAt = &joinedAt
		}
		out = append(out, m)
	}
	return out
}

// ExecuteDueDeletions is the cron job's entry point. Returns how many accounts
// were deleted.
func (s *MembershipLifecycleService) ExecuteDueDeletions(ctx context.Context, limit int32) (int, error) {
	logger := util.GetLoggerFromCtx(ctx)

	due, err := s.store.ListUsersDueForDeletion(ctx, limit)
	if err != nil {
		return 0, err
	}

	done := 0
	for _, user := range due {
		if err := s.executeDeletion(ctx, user.ID); err != nil {
			// One account failing must not stop the queue: the next run picks
			// it up, and a stuck row would silently hold everyone behind it.
			logger.Err(err).Str("user_id", user.ID).Msg("deletion: failed, will retry on the next run")
			continue
		}
		done++
	}
	return done, nil
}

func (s *MembershipLifecycleService) executeDeletion(ctx context.Context, userID string) error {
	logger := util.GetLoggerFromCtx(ctx)

	// What the person asked for, replayed per tenant before anything is erased.
	// A closure answered with silence — or one made before this existed — falls
	// through to each module's own policy; an answer that fails holds the
	// deletion instead.
	if err := s.applyStoredDecisions(ctx, userID); err != nil {
		return fmt.Errorf("apply stored decisions: %w", err)
	}

	// Modules first: they anonymize or delete rows that reference the user id,
	// and core_users must still exist while they do it.
	for _, c := range sharedservice.UserDataContributors() {
		if err := c.Erase(ctx, userID, "", sharedservice.EraseAccountDeletion); err != nil {
			return fmt.Errorf("contributor %s: %w", c.Name(), err)
		}
	}

	// Now — and only now — the person is actually leaving. Sessions go with the
	// account; there is no organization left to stay signed in to.
	if err := s.authProvider.GetAuthClient().RevokeSessions(ctx, userID); err != nil {
		logger.Warn().Err(err).Str("user_id", userID).
			Msg("deletion: session revocation failed; the identity is about to go anyway")
	}

	if err := s.store.DeleteAllUserTenantMemberships(ctx, userID); err != nil {
		return err
	}
	if err := s.store.DeleteSharedUserRow(ctx, userID); err != nil {
		return err
	}
	if err := s.authProvider.GetAuthClient().DeleteUser(ctx, userID); err != nil && !auth.IsUserNotFound(err) {
		return err
	}

	logger.Info().Str("user_id", userID).Msg("deletion: account deleted")
	return nil
}

// applyStoredDecisions replays the answers given at closing time, grouped by the
// tenant each one was about.
func (s *MembershipLifecycleService) applyStoredDecisions(ctx context.Context, userID string) error {
	user, err := s.store.GetSharedUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load stored deletion decisions: %w", err)
	}
	return replayDeletionDecisions(ctx, userID, user.DeletionDecisions, sharedservice.UserDataContributors())
}

// replayDeletionDecisions applies every stored answer and fails if any of them
// could not be applied. The erase that follows is the module default — for
// authored courses that means deleting what the person asked to hand over — so
// a failed transfer must stop the deletion, not fall through to it. The sweep
// retries the account on its next run; contributors' decisions are idempotent.
func replayDeletionDecisions(ctx context.Context, userID string, raw []byte, contributors []sharedservice.UserDataContributor) error {
	if len(raw) == 0 {
		return nil
	}

	var stored []core.TenantLeaveDecision
	if err := json.Unmarshal(raw, &stored); err != nil {
		// Unreadable answers might have asked for a transfer; erasing anyway
		// could destroy what the person wanted kept.
		return fmt.Errorf("stored deletion decisions of user %s are unreadable, deletion held: %w", userID, err)
	}

	byTenant := map[string][]core.LeaveDecision{}
	for _, d := range stored {
		byTenant[d.TenantId] = append(byTenant[d.TenantId], core.LeaveDecision{
			Key: d.Key, Action: core.LeaveDecisionAction(d.Action), TargetUserId: d.TargetUserId,
		})
	}

	// Every tenant is still attempted so one run reports every failure at once.
	var failures []error
	for tenantID, decisions := range byTenant {
		for _, c := range contributors {
			if err := c.ApplyLeaveDecisions(ctx, userID, tenantID, decisions); err != nil {
				failures = append(failures, fmt.Errorf("contributor %s, tenant %s: %w", c.Name(), tenantID, err))
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("stored deletion decisions failed, deletion held so nothing is erased in their place: %w", errors.Join(failures...))
	}
	return nil
}

// PurgeDormantData drops the tenant-scoped data of members who left longer ago
// than the dormancy window. The membership row survives — it is the return path.
func (s *MembershipLifecycleService) PurgeDormantData(ctx context.Context, userID, tenantID string) error {
	for _, c := range sharedservice.UserDataContributors() {
		if err := c.Erase(ctx, userID, tenantID, sharedservice.EraseTenantDormancy); err != nil {
			return fmt.Errorf("contributor %s: %w", c.Name(), err)
		}
	}
	return s.store.MarkDormantDataPurged(ctx, repository.MarkDormantDataPurgedParams{
		UserID: userID, TenantID: tenantID,
	})
}

// PurgeDueDormantData runs the dormancy side of the sweep. Without it the
// 90-day promise in the leave dialog is decorative.
func (s *MembershipLifecycleService) PurgeDueDormantData(ctx context.Context, limit int32) (int, error) {
	logger := util.GetLoggerFromCtx(ctx)

	due, err := s.store.ListMembershipsDueForDormancyPurge(ctx, repository.ListMembershipsDueForDormancyPurgeParams{
		UpdatedAt: time.Now().Add(-DormancyWindow),
		Limit:     limit,
	})
	if err != nil {
		return 0, err
	}
	done := 0
	for _, m := range due {
		if err := s.PurgeDormantData(ctx, m.UserID, m.TenantID); err != nil {
			logger.Err(err).Str("user_id", m.UserID).Str("tenant_id", m.TenantID).
				Msg("dormancy purge failed, will retry on the next run")
			continue
		}
		done++
	}
	return done, nil
}

// PersonalDataAccessRetentionMonths is how long a personal-data access log row
// is kept: long enough to answer "who looked at my data this year".
const PersonalDataAccessRetentionMonths = 12

// PurgePersonalDataAccessLog deletes access-log rows past their retention.
func (s *MembershipLifecycleService) PurgePersonalDataAccessLog(ctx context.Context, now time.Time) (int64, error) {
	return s.store.PurgePersonalDataAccessBefore(ctx, now.AddDate(0, -PersonalDataAccessRetentionMonths, 0))
}

// StartLifecycleSweeper runs the scheduled halves of ADR 040: deletions whose
// grace period has expired, the dormant data of members who left long ago, and
// accounts nobody has used for the configured inactivity period, and
// personal-data access log rows past their retention.
//
// One immediate pass at boot, then daily. Both are idempotent and bounded, so a
// missed run costs a day of latency rather than correctness — which is why an
// in-process ticker is enough and no external scheduler is required.
func (s *MembershipLifecycleService) StartLifecycleSweeper(ctx context.Context) {
	go func() {
		// Logged from HERE, not from server construction: modules register their
		// contributors after core builds the server, so a log emitted there
		// always reported none and taught readers to ignore it. The sweeper is
		// the thing that depends on them, and it starts last.
		sharedservice.LogRegisteredContributors(ctx)

		s.sweepOnce(ctx)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweepOnce(ctx)
			}
		}
	}()
}

func (s *MembershipLifecycleService) sweepOnce(ctx context.Context) {
	logger := util.GetLoggerFromCtx(ctx)

	if n, err := s.ExecuteDueDeletions(ctx, sweepPage); err != nil {
		logger.Err(err).Msg("lifecycle sweep: deletions failed")
	} else if n > 0 {
		logger.Info().Int("accounts", n).Msg("lifecycle sweep: accounts deleted")
	}

	if n, err := s.PurgeDueDormantData(ctx, sweepPage); err != nil {
		logger.Err(err).Msg("lifecycle sweep: dormancy purge failed")
	} else if n > 0 {
		logger.Info().Int("memberships", n).Msg("lifecycle sweep: dormant data purged")
	}

	if err := s.ProcessInactiveAccounts(ctx, time.Now()); err != nil {
		logger.Err(err).Msg("lifecycle sweep: inactive accounts failed")
	}

	if n, err := s.PurgePersonalDataAccessLog(ctx, time.Now()); err != nil {
		logger.Err(err).Msg("lifecycle sweep: personal-data access log purge failed")
	} else if n > 0 {
		logger.Info().Int64("rows", n).Msg("lifecycle sweep: personal-data access log purged")
	}
}

func (s *MembershipLifecycleService) activeMembership(ctx context.Context, userID, tenantID string) (repository.GetUserTenantMembershipRow, error) {
	m, err := s.store.GetUserTenantMembership(ctx, repository.GetUserTenantMembershipParams{
		UserID: userID, TenantID: tenantID,
	})
	if err != nil {
		return m, ErrNoActiveMembership
	}
	if m.Status != "active" {
		return m, ErrNoActiveMembership
	}
	return m, nil
}

func (s *MembershipLifecycleService) collectImpacts(ctx context.Context, userID, tenantID string) []core.LeaveImpact {
	logger := util.GetLoggerFromCtx(ctx)
	impacts := []core.LeaveImpact{}
	for _, c := range sharedservice.UserDataContributors() {
		got, err := c.LeaveImpacts(ctx, userID, tenantID)
		if err != nil {
			logger.Err(err).Str("contributor", c.Name()).Msg("leave preview: contributor failed")
			continue
		}
		impacts = append(impacts, got...)
	}
	return impacts
}

func (s *MembershipLifecycleService) countMemberships(ctx context.Context, userID string) int {
	memberships, err := s.store.ListAllUserTenantMemberships(ctx, userID)
	if err != nil {
		return 0
	}
	return len(memberships)
}

// hasOutstandingDecision reports whether any impact needing an answer has none.
// A transfer without a target is unanswered too — it names no destination.
func hasOutstandingDecision(impacts []core.LeaveImpact, decisions []core.LeaveDecision) bool {
	for _, impact := range impacts {
		if impact.Severity != core.LeaveImpactSeverity("decision") {
			continue
		}
		answered := false
		for _, d := range decisions {
			if d.Key != impact.Key {
				continue
			}
			if d.Action == core.LeaveDecisionAction("transfer") && (d.TargetUserId == nil || *d.TargetUserId == "") {
				continue
			}
			answered = true
			break
		}
		if !answered {
			return true
		}
	}
	return false
}

func deletionStatusOf(user repository.CoreUser, tenantsAffected int) core.AccountDeletion {
	exportAvailable := true
	if !user.DeletionScheduledAt.Valid {
		return core.AccountDeletion{
			Status:          core.AccountDeletionStatus("none"),
			ExportAvailable: &exportAvailable,
			TenantsAffected: &tenantsAffected,
		}
	}
	scheduledFor := user.DeletionScheduledAt.Time
	out := core.AccountDeletion{
		Status:          core.AccountDeletionStatus("scheduled"),
		ScheduledFor:    &scheduledFor,
		ExportAvailable: &exportAvailable,
		TenantsAffected: &tenantsAffected,
	}
	if user.DeletionRequestedAt.Valid {
		requestedAt := user.DeletionRequestedAt.Time
		out.ScheduledAt = &requestedAt
	}
	return out
}
