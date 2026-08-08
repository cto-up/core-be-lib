package service

import (
	"context"
	"sync"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/shared/util"
)

// EraseScope says which of the two erasures is being performed. They are not the
// same operation: one ends a relationship with an organization, the other ends
// the person's presence on the platform.
type EraseScope string

const (
	// EraseTenantDormancy purges what a departed member's tenant-scoped data
	// was kept for — the return path — once the dormancy window has passed.
	// The membership row itself is never purged: reactivating it is what makes
	// returning cheap.
	EraseTenantDormancy EraseScope = "tenant_dormancy"

	// EraseAccountDeletion runs after the account-deletion grace period. It
	// means "the person is no longer identifiable in it", not "the row is
	// gone" — anonymize in place wherever a row must survive for accounting,
	// certificate verification or course statistics.
	EraseAccountDeletion EraseScope = "account_deletion"
)

// UserDataContributor is implemented by any module that owns user-attributed
// rows, and registered by the consumer at boot. Core owns the endpoints of the
// membership lifecycle and knows nothing about what the modules store: an LMS
// answers with enrollments and certificates because it registered, not because
// core has heard of them.
//
// One interface rather than three registries, because the three methods are the
// same question asked at three moments. Something worth warning about before
// leaving is something worth exporting, and something worth exporting is
// something the erase path has to reach.
type UserDataContributor interface {
	// Name keys this contributor's section of the export, e.g. "lms".
	Name() string

	// LeaveImpacts reports what the caller would lose, or must decide about,
	// by leaving this tenant. Impacts of severity "decision" must be answered
	// before the leave proceeds.
	LeaveImpacts(ctx context.Context, userID, tenantID string) ([]core.LeaveImpact, error)

	// ApplyLeaveDecisions performs the decisions the caller answered with
	// (transfer authored content, cancel a subscription at period end). Called
	// before the membership is ended, so a failure aborts the leave rather than
	// stranding content.
	ApplyLeaveDecisions(ctx context.Context, userID, tenantID string, decisions []core.LeaveDecision) error

	// Export returns this module's slice of the caller's data. The shape is the
	// module's business; core only keys it by Name.
	Export(ctx context.Context, userID string) (any, error)

	// Erase deletes or anonymizes. tenantID is set for EraseTenantDormancy and
	// empty for EraseAccountDeletion, which spans every tenant.
	Erase(ctx context.Context, userID, tenantID string, scope EraseScope) error
}

var (
	contributorsMu sync.RWMutex
	contributors   []UserDataContributor
)

// RegisterUserDataContributor installs a contributor. Call at boot, before
// serving, from the same place the seat guard and the credit module's rules are
// registered. Idempotent per name: registering the same Name twice replaces the
// earlier one rather than double-counting its impacts.
func RegisterUserDataContributor(c UserDataContributor) {
	if c == nil {
		return
	}
	contributorsMu.Lock()
	defer contributorsMu.Unlock()
	for i, existing := range contributors {
		if existing.Name() == c.Name() {
			contributors[i] = c
			return
		}
	}
	contributors = append(contributors, c)
}

// UserDataContributors returns the registered contributors.
func UserDataContributors() []UserDataContributor {
	contributorsMu.RLock()
	defer contributorsMu.RUnlock()
	out := make([]UserDataContributor, len(contributors))
	copy(out, contributors)
	return out
}

// LogRegisteredContributors states at boot who answered. Unlike the seat guard,
// where "nobody registered" is a safe default, an unregistered contributor means
// a leave dialog that under-reports, an export missing personal data, and rows
// that outlive a deletion which promised otherwise — and it is invisible in the
// output, because the missing module is missing from that too. This line is
// where the omission becomes findable.
func LogRegisteredContributors(ctx context.Context) {
	logger := util.GetLoggerFromCtx(ctx)
	names := make([]string, 0, len(contributors))
	for _, c := range UserDataContributors() {
		names = append(names, c.Name())
	}
	if len(names) == 0 {
		logger.Warn().Msg("membership lifecycle: NO user-data contributors registered — export and erase will cover core data only")
		return
	}
	logger.Info().Strs("contributors", names).Msg("membership lifecycle: user-data contributors registered")
}
