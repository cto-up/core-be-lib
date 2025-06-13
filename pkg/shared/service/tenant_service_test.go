package service

import (
	"context"
	"math/rand"
	"testing"

	"ctoup.com/coreapp/api/openapi/core"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAndDeleteTenant(t *testing.T) {
	// Set up test environment variables
	godotenv.Load("../../../.env")
	godotenv.Overload("../../../.env", "../../../.env.local")

	// Test data
	tenantJSON := core.AddTenantJSONRequestBody{
		Name:                  "testtenant" + randomString(2),
		Subdomain:             "testsubdomain" + randomString(2),
		EnableEmailLinkSignIn: true,
		AllowPasswordSignUp:   true,
	}

	// Create a context
	ctx := context.Background()

	// Initialize Firebase Auth client
	authClient, err := newFirebaseClient(ctx)
	require.NoError(t, err)

	// Create the tenant
	tenant, err := CreateTenant(ctx, authClient, tenantJSON)
	require.NoError(t, err)
	require.NotNil(t, tenant)
	assert.Equal(t, tenantJSON.Subdomain, tenant.DisplayName)

	// Store the tenant ID for cleanup
	tenantID := tenant.ID

	// Verify tenant was created
	t.Logf("Created tenant with ID: %s", tenantID)

	// Test updating the tenant
	updateJSON := core.UpdateTenantJSONRequestBody{
		Name:                  tenantJSON.Name + "up",
		Subdomain:             tenantJSON.Subdomain + "up",
		EnableEmailLinkSignIn: false,
		AllowPasswordSignUp:   true,
	}

	updatedTenant, err := UpdateTenant(ctx, authClient, tenantID, updateJSON)
	require.NoError(t, err)
	require.NotNil(t, updatedTenant)
	assert.Equal(t, updateJSON.Subdomain, updatedTenant.DisplayName)

	// Delete the tenant
	err = DeleteTenant(ctx, authClient, tenantID)
	require.NoError(t, err)

	// Verify tenant was deleted by trying to get it
	tenants, err := ListTenants(ctx, authClient, "")
	require.NoError(t, err)

	// Check that the deleted tenant is not in the list
	var found bool
	for _, t := range tenants {
		if t.ID == tenantID {
			found = true
			break
		}
	}
	assert.False(t, found, "Tenant should have been deleted")
}

func TestListTenants(t *testing.T) {
	// Set up test environment variables
	godotenv.Load("../../../.env")
	godotenv.Overload("../../../.env", "../../../.env.local")

	// Create a context
	ctx := context.Background()

	// Initialize Firebase Auth client
	authClient, err := newFirebaseClient(ctx)
	require.NoError(t, err)

	// List tenants
	tenants, err := ListTenants(ctx, authClient, "")
	require.NoError(t, err)

	// Just verify we can list tenants without error
	t.Logf("Found %d tenants", len(tenants))
}

// Helper function to generate a random string
func randomString(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
