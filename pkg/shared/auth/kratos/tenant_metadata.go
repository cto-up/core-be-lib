package kratos

import (
	"context"
	"fmt"

	"ctoup.com/coreapp/pkg/shared/auth"
	ory "github.com/ory/kratos-client-go"
)

// TenantMetadata represents tenant information stored in Kratos identity metadata
type TenantMetadata struct {
	TenantID   string   `json:"tenant_id"`
	Subdomain  string   `json:"subdomain,omitempty"`
	Roles      []string `json:"roles,omitempty"`
	TenantName string   `json:"tenant_name,omitempty"`
}

// SetTenantMetadata adds tenant information to a Kratos identity's metadata_public
func (k *KratosAuthClient) SetTenantMetadata(ctx context.Context, uid string, metadata TenantMetadata) error {
	existing, _, err := k.adminClient.IdentityAPI.GetIdentity(ctx, uid).Execute()
	if err != nil {
		return auth.ConvertKratosError(err)
	}

	// Get or create metadata_public with type assertion
	metadataPublic := make(map[string]interface{})
	if existing.MetadataPublic != nil {
		if mp, ok := existing.MetadataPublic.(map[string]interface{}); ok {
			metadataPublic = mp
		}
	}

	// Update tenant metadata
	metadataPublic["tenant_id"] = metadata.TenantID
	if metadata.Subdomain != "" {
		metadataPublic["subdomain"] = metadata.Subdomain
	}
	if metadata.TenantName != "" {
		metadataPublic["tenant_name"] = metadata.TenantName
	}
	if len(metadata.Roles) > 0 {
		metadataPublic["roles"] = metadata.Roles
	}

	// Update identity with new metadata
	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}

	// Type assert traits
	traits, ok := existing.Traits.(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
	}

	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, traits)
	updateBody.MetadataPublic = metadataPublic

	_, _, err = k.adminClient.IdentityAPI.UpdateIdentity(ctx, uid).UpdateIdentityBody(updateBody).Execute()
	return auth.ConvertKratosError(err)
}

// GetTenantMetadata retrieves tenant information from a Kratos identity
func (k *KratosAuthClient) GetTenantMetadata(ctx context.Context, uid string) (*TenantMetadata, error) {
	ident, _, err := k.adminClient.IdentityAPI.GetIdentity(ctx, uid).Execute()
	if err != nil {
		return nil, auth.ConvertKratosError(err)
	}

	if ident.MetadataPublic == nil {
		return nil, fmt.Errorf("no tenant metadata found for user")
	}

	// Type assert metadata_public
	metadataPublic, ok := ident.MetadataPublic.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid metadata_public format")
	}

	metadata := &TenantMetadata{}

	if tenantID, ok := metadataPublic["tenant_id"].(string); ok {
		metadata.TenantID = tenantID
	}
	if subdomain, ok := metadataPublic["subdomain"].(string); ok {
		metadata.Subdomain = subdomain
	}
	if tenantName, ok := metadataPublic["tenant_name"].(string); ok {
		metadata.TenantName = tenantName
	}
	if roles, ok := metadataPublic["roles"].([]interface{}); ok {
		for _, role := range roles {
			if roleStr, ok := role.(string); ok {
				metadata.Roles = append(metadata.Roles, roleStr)
			}
		}
	}

	return metadata, nil
}

// CreateUserWithTenant creates a user and associates them with a tenant
func (k *KratosAuthClient) CreateUserWithTenant(ctx context.Context, user *auth.UserToCreate, tenantID string, subdomain string) (*auth.UserRecord, error) {
	// Create the user first
	record, err := k.CreateUser(ctx, user)
	if err != nil {
		return nil, err
	}

	// Add tenant metadata
	metadata := TenantMetadata{
		TenantID:  tenantID,
		Subdomain: subdomain,
	}

	err = k.SetTenantMetadata(ctx, record.UID, metadata)
	if err != nil {
		// Rollback: delete the created user
		_ = k.DeleteUser(ctx, record.UID)
		return nil, fmt.Errorf("failed to set tenant metadata: %w", err)
	}

	return record, nil
}

// ListUsersByTenant lists all users belonging to a specific tenant
func (k *KratosAuthClient) ListUsersByTenant(ctx context.Context, tenantID string) ([]*auth.UserRecord, error) {
	// Note: Kratos doesn't have native filtering by metadata
	// This is a workaround that lists all identities and filters
	// For production, consider using Kratos search capabilities or maintaining a separate index

	identities, _, err := k.adminClient.IdentityAPI.ListIdentities(ctx).Execute()
	if err != nil {
		return nil, auth.ConvertKratosError(err)
	}

	var users []*auth.UserRecord
	for _, ident := range identities {
		if ident.MetadataPublic != nil {
			// Type assert metadata_public
			if metadataPublic, ok := ident.MetadataPublic.(map[string]interface{}); ok {
				if tid, ok := metadataPublic["tenant_id"].(string); ok && tid == tenantID {
					users = append(users, convertKratosIdentityToUserRecord(&ident))
				}
			}
		}
	}

	return users, nil
}
