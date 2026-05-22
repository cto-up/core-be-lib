package service

import (
	"context"
	"fmt"
	"strings"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
)

// ErrUserActiveInOtherTenants signals that the target user cannot be
// permanently deleted because they still have active memberships in tenants
// other than the one the caller is operating from. Callers should surface
// this as 409 Conflict with the error message and instruct the operator to
// deactivate those memberships first.
type ErrUserActiveInOtherTenants struct {
	OtherTenants []string
}

func (e *ErrUserActiveInOtherTenants) Error() string {
	return fmt.Sprintf(
		"user has active memberships in %d other tenant(s): %s — deactivate those first",
		len(e.OtherTenants),
		strings.Join(e.OtherTenants, ", "),
	)
}

// CheckUserNotActiveInOtherTenants is the pre-flight guard for any operation
// that permanently removes a user (hard-delete, GDPR erasure). Returns
// *ErrUserActiveInOtherTenants if the user still has status='active'
// membership outside currentTenantID; nil otherwise.
//
// Inactive memberships are fine — they cascade-delete with core_users.
func CheckUserNotActiveInOtherTenants(ctx context.Context, store *db.Store, userID, currentTenantID string) error {
	memberships, err := store.Queries.ListUserTenantMemberships(ctx, repository.ListUserTenantMembershipsParams{
		UserID: userID,
		Status: "active",
	})
	if err != nil {
		return fmt.Errorf("list active tenant memberships: %w", err)
	}
	var others []string
	for _, m := range memberships {
		if m.TenantID != currentTenantID {
			others = append(others, m.TenantName)
		}
	}
	if len(others) > 0 {
		return &ErrUserActiveInOtherTenants{OtherTenants: others}
	}
	return nil
}
