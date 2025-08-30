package service

import (
	"context"
	"fmt"

	"ctoup.com/coreapp/api/openapi/core"
	"firebase.google.com/go/auth"
)

func CreateTenant(ctx context.Context, authClient *auth.Client, tenantJSON core.AddTenantJSONRequestBody) (*auth.Tenant, error) {
	// Define tenant properties
	params := (&auth.TenantToCreate{}).
		DisplayName(tenantJSON.Subdomain).
		EnableEmailLinkSignIn(tenantJSON.EnableEmailLinkSignIn).
		AllowPasswordSignUp(tenantJSON.AllowPasswordSignUp)
	// Create the tenant
	tenant, err := authClient.TenantManager.CreateTenant(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error creating tenant: %v", err)
	}

	return tenant, nil
}

func UpdateTenant(ctx context.Context, authClient *auth.Client, tenantID string, tenantJSON core.UpdateTenantJSONRequestBody) (*auth.Tenant, error) {
	// Define tenant properties
	params := (&auth.TenantToUpdate{}).
		DisplayName(tenantJSON.Subdomain).
		EnableEmailLinkSignIn(tenantJSON.EnableEmailLinkSignIn).
		AllowPasswordSignUp(tenantJSON.AllowPasswordSignUp)
	// Update the tenant
	tenant, err := authClient.TenantManager.UpdateTenant(ctx, tenantID, params)
	if err != nil {
		return nil, fmt.Errorf("error updating tenant: %v", err)
	}
	return tenant, nil
}

func DeleteTenant(ctx context.Context, authClient *auth.Client, tenantID string) error {
	// Delete the tenant
	err := authClient.TenantManager.DeleteTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("error deleting tenant: %v", err)
	}
	return nil
}

// List Tenants
func ListTenants(ctx context.Context, authClient *auth.Client, nextPageToken string) ([]auth.Tenant, error) {
	tenants := make([]auth.Tenant, 0)
	iterator := authClient.TenantManager.Tenants(ctx, nextPageToken)
	for {
		tenant, err := iterator.Next()

		if err != nil {
			if err.Error() == "no more items in iterator" {
				break
			}
			return nil, fmt.Errorf("error listing tenants: %v", err)
		}
		tenants = append(tenants, *tenant)
	}
	return tenants, nil
}
