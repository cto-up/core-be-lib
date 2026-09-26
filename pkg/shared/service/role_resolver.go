package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/access"
)

// membershipReader is the one query the resolver needs, so it can be tested
// without a database.
type membershipReader interface {
	GetSharedUserTenantMembership(ctx context.Context, arg repository.GetSharedUserTenantMembershipParams) (repository.CoreUserTenantMembership, error)
}

// MembershipRoleResolver answers access.RoleResolver from the tenant membership
// table: an ACTIVE membership's roles. An invitation carries its intended roles
// from the moment it is sent, so a membership that was never joined grants
// nothing. No row is "not a member" — an answer, not an error.
//
// Reads are never isolated here: the sign-up-tenant isolation needs the
// tenant's allow-sign-up flag, which only the session door has.
func MembershipRoleResolver(store membershipReader) access.RoleResolver {
	return func(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
		m, err := store.GetSharedUserTenantMembership(ctx, repository.GetSharedUserTenantMembershipParams{
			UserID: userID, TenantID: tenantID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if !strings.EqualFold(m.Status, "active") {
			return nil, false, nil
		}
		return m.Roles, false, nil
	}
}
