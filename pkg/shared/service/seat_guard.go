package service

import (
	"context"
	"sync"

	"ctoup.com/coreapp/api/openapi/core"
)

// SeatGuard is consulted before a user is added to a tenant. Returning an error
// refuses the addition.
//
// Registered by the consumer at boot rather than implemented here, exactly like
// credit-lib's PeriodGrantRule: core owns membership and must stay ignorant of
// plans, entitlements and billing. It knows only "somebody said no".
//
// roles is passed because not every membership consumes a seat — a plan that
// counts LEARNERS should not be charged for adding an admin. The consumer
// decides; core does not interpret roles for this purpose.
type SeatGuard func(ctx context.Context, tenantID string, roles []core.Role) error

var (
	seatGuardMu sync.RWMutex
	seatGuard   SeatGuard
)

// RegisterSeatGuard installs (or replaces) the guard. Idempotent; call at boot
// before serving. nil disables the check, which is the default — an unconfigured
// deployment must not start refusing memberships.
func RegisterSeatGuard(g SeatGuard) {
	seatGuardMu.Lock()
	defer seatGuardMu.Unlock()
	seatGuard = g
}

// checkSeatGuard runs the registered guard, if any.
func checkSeatGuard(ctx context.Context, tenantID string, roles []core.Role) error {
	seatGuardMu.RLock()
	g := seatGuard
	seatGuardMu.RUnlock()
	if g == nil {
		return nil
	}
	return g(ctx, tenantID, roles)
}
