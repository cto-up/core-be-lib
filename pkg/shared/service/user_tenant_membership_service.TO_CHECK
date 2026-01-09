package service

import (
	"context"
	"fmt"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/auth/kratos"
	"github.com/jackc/pgx/v5/pgtype"
	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

type UserTenantMembershipService struct {
	store        *db.Store
	authProvider auth.AuthProvider
}

func NewUserTenantMembershipService(store *db.Store, authProvider auth.AuthProvider) *UserTenantMembershipService {
	return &UserTenantMembershipService{
		store:        store,
		authProvider: authProvider,
	}
}

// AddUserToTenant adds a user to a tenant with specific roles
func (s *UserTenantMembershipService) AddUserToTenant(
	ctx context.Context,
	userID string,
	tenantID string,
	roles []string,
	invitedBy string,
) error {
	now := time.Now()

	// Create membership in database
	_, err := s.store.CreateUserTenantMembership(ctx, repository.CreateUserTenantMembershipParams{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    roles,
		Status:   "active",
		InvitedBy: pgtype.Text{
			String: invitedBy,
			Valid:  true,
		},
		InvitedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		JoinedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create membership: %w", err)
	}

	// Update Kratos metadata with tenant memberships
	err = s.updateKratosTenantMemberships(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update Kratos metadata")
		// Don't fail - membership is in database
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Strs("roles", roles).
		Msg("User added to tenant")

	return nil
}

// RemoveUserFromTenant removes a user from a tenant
func (s *UserTenantMembershipService) RemoveUserFromTenant(
	ctx context.Context,
	userID string,
	tenantID string,
) error {
	// Update status to removed
	_, err := s.store.UpdateUserTenantMembershipStatus(ctx, repository.UpdateUserTenantMembershipStatusParams{
		UserID:   userID,
		TenantID: tenantID,
		Status:   "removed",
	})

	if err != nil {
		return fmt.Errorf("failed to remove membership: %w", err)
	}

	// Update Kratos metadata
	err = s.updateKratosTenantMemberships(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update Kratos metadata")
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Msg("User removed from tenant")

	return nil
}

// GetUserTenants returns all tenants a user belongs to
func (s *UserTenantMembershipService) GetUserTenants(
	ctx context.Context,
	userID string,
) ([]repository.ListUserTenantMembershipsRow, error) {
	return s.store.ListUserTenantMemberships(ctx, repository.ListUserTenantMembershipsParams{
		UserID: userID,
		Status: "active",
	})
}

// GetPendingInvitations returns all pending invitations for a user
func (s *UserTenantMembershipService) GetPendingInvitations(
	ctx context.Context,
	userID string,
) ([]repository.ListPendingInvitationsRow, error) {
	return s.store.ListPendingInvitations(ctx, userID)
}

// GetTenantMembers returns all members of a tenant
func (s *UserTenantMembershipService) GetTenantMembers(
	ctx context.Context,
	tenantID string,
	status string,
) ([]repository.CoreUserTenantMembership, error) {
	return s.store.ListTenantMembers(ctx, repository.ListTenantMembersParams{
		TenantID: tenantID,
		Status:   status,
	})
}

// CheckUserTenantAccess checks if a user has access to a tenant
func (s *UserTenantMembershipService) CheckUserTenantAccess(
	ctx context.Context,
	userID string,
	tenantID string,
) (bool, error) {
	result, err := s.store.CheckUserTenantAccess(ctx, repository.CheckUserTenantAccessParams{
		UserID:   userID,
		TenantID: tenantID,
	})

	if err != nil {
		return false, err
	}

	return result, nil
}

// GetUserTenantRoles returns the user's roles in a specific tenant
func (s *UserTenantMembershipService) GetUserTenantRoles(
	ctx context.Context,
	userID string,
	tenantID string,
) ([]string, error) {
	roles, err := s.store.GetUserTenantRoles(ctx, repository.GetUserTenantRolesParams{
		UserID:   userID,
		TenantID: tenantID,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get user tenant roles: %w", err)
	}

	return roles, nil
}

// CheckUserHasRole checks if user has a specific role in a tenant
func (s *UserTenantMembershipService) CheckUserHasRole(
	ctx context.Context,
	userID string,
	tenantID string,
	role string,
) (bool, error) {
	hasRole, err := s.store.CheckUserHasTenantRole(ctx, repository.CheckUserHasTenantRoleParams{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
	})

	if err != nil {
		return false, fmt.Errorf("failed to check user role: %w", err)
	}

	return hasRole, nil
}

// UpdateMemberRoles updates a member's roles in a tenant
func (s *UserTenantMembershipService) UpdateMemberRoles(
	ctx context.Context,
	userID string,
	tenantID string,
	roles []string,
) error {
	_, err := s.store.UpdateUserTenantMembershipRoles(ctx, repository.UpdateUserTenantMembershipRolesParams{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    roles,
	})

	if err != nil {
		return fmt.Errorf("failed to update roles: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Strs("roles", roles).
		Msg("Member roles updated")

	return nil
}

// AddRoleToMember adds a single role to a member's existing roles
func (s *UserTenantMembershipService) AddRoleToMember(
	ctx context.Context,
	userID string,
	tenantID string,
	role string,
) error {
	_, err := s.store.AddRoleToUserTenantMembership(ctx, repository.AddRoleToUserTenantMembershipParams{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
	})

	if err != nil {
		return fmt.Errorf("failed to add role: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Str("role", role).
		Msg("Role added to member")

	return nil
}

// RemoveRoleFromMember removes a single role from a member's roles
func (s *UserTenantMembershipService) RemoveRoleFromMember(
	ctx context.Context,
	userID string,
	tenantID string,
	role string,
) error {
	_, err := s.store.RemoveRoleFromUserTenantMembership(ctx, repository.RemoveRoleFromUserTenantMembershipParams{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
	})

	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Str("role", role).
		Msg("Role removed from member")

	return nil
}

// InviteUserToTenant creates a pending membership invitation
func (s *UserTenantMembershipService) InviteUserToTenant(
	ctx context.Context,
	email string,
	tenantID string,
	roles []string,
	invitedBy string,
) error {
	// Check if user exists
	authClient := s.authProvider.GetAuthClient()
	user, err := authClient.GetUserByEmail(ctx, email)

	if err != nil {
		// User doesn't exist - create pending invitation
		// Store in separate invitations table or send email
		return fmt.Errorf("user not found, invitation email should be sent")
	}

	now := time.Now()

	// User exists - create pending membership
	_, err = s.store.CreateUserTenantMembership(ctx, repository.CreateUserTenantMembershipParams{
		UserID:   user.UID,
		TenantID: tenantID,
		Roles:    roles,
		Status:   "pending",
		InvitedBy: pgtype.Text{
			String: invitedBy,
			Valid:  true,
		},
		InvitedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		JoinedAt: pgtype.Timestamptz{
			Valid: false,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}

	log.Info().
		Str("email", email).
		Str("user_id", user.UID).
		Str("tenant_id", tenantID).
		Strs("roles", roles).
		Msg("User invited to tenant")

	return nil
}

// AcceptTenantInvitation accepts a pending invitation
func (s *UserTenantMembershipService) AcceptTenantInvitation(
	ctx context.Context,
	userID string,
	tenantID string,
) error {
	now := time.Now()

	// Update status to active and set joined_at
	_, err := s.store.UpdateUserTenantMembershipJoinedAt(ctx, repository.UpdateUserTenantMembershipJoinedAtParams{
		UserID:   userID,
		TenantID: tenantID,
		JoinedAt: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to accept invitation: %w", err)
	}

	// Update Kratos metadata
	err = s.updateKratosTenantMemberships(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update Kratos metadata")
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Msg("User accepted tenant invitation")

	return nil
}

// RejectTenantInvitation rejects a pending invitation
func (s *UserTenantMembershipService) RejectTenantInvitation(
	ctx context.Context,
	userID string,
	tenantID string,
) error {
	// Delete the invitation
	err := s.store.DeleteUserTenantMembership(ctx, repository.DeleteUserTenantMembershipParams{
		UserID:   userID,
		TenantID: tenantID,
	})

	if err != nil {
		return fmt.Errorf("failed to reject invitation: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Msg("User rejected tenant invitation")

	return nil
}

// UpdateMemberRole updates a member's role in a tenant (deprecated - use UpdateMemberRoles)
func (s *UserTenantMembershipService) UpdateMemberRole(
	ctx context.Context,
	userID string,
	tenantID string,
	role string,
) error {
	// Convert single role to array for backward compatibility
	return s.UpdateMemberRoles(ctx, userID, tenantID, []string{role})
}

// updateKratosTenantMemberships updates the user's Kratos metadata with current tenant memberships and roles
func (s *UserTenantMembershipService) updateKratosTenantMemberships(
	ctx context.Context,
	userID string,
) error {
	// Get all active memberships
	memberships, err := s.GetUserTenants(ctx, userID)
	if err != nil {
		return err
	}

	// Build tenant memberships with roles
	tenantMemberships := make([]map[string]interface{}, len(memberships))
	for i, membership := range memberships {
		tenantMemberships[i] = map[string]interface{}{
			"tenant_id": membership.TenantID,
			"roles":     membership.Roles,
		}
	}

	// Update Kratos metadata
	authClient := s.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		return fmt.Errorf("auth provider is not Kratos")
	}

	// Get existing identity
	existing, _, err := kratosClient.GetAdminClient().IdentityAPI.GetIdentity(ctx, userID).Execute()
	if err != nil {
		return err
	}

	// Update metadata_public
	metadataPublic, ok := existing.MetadataPublic.(map[string]interface{})
	if !ok || metadataPublic == nil {
		metadataPublic = make(map[string]interface{})
	}

	metadataPublic[auth.AUTH_TENANT_MEMBERSHIPS] = tenantMemberships

	// Update identity
	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}

	traits, ok := existing.Traits.(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
	}

	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, traits)
	updateBody.MetadataPublic = metadataPublic

	_, _, err = kratosClient.GetAdminClient().IdentityAPI.UpdateIdentity(ctx, userID).
		UpdateIdentityBody(updateBody).
		Execute()

	return err
}
