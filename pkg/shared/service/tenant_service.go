package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"ctoup.com/coreapp/api/openapi/core"
	"firebase.google.com/go/auth"
	"golang.org/x/oauth2/google"
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
	// Create the tenant
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

// AuthorizedDomainsConfig represents the structure for authorized domains
type AuthorizedDomainsConfig struct {
	AuthorizedDomains []string `json:"authorizedDomains"`
}

// AuthorizeDomains updates the authorized domains for Firebase Authentication
func AuthorizeDomains(ctx context.Context, authClient *auth.Client, domains []string) error {
	// Get Firebase service account credentials from environment
	fcfg := os.Getenv("FIREBASE_CONFIG")
	if fcfg == "" {
		return fmt.Errorf("missing FIREBASE_CONFIG environment variable")
	}

	// Parse the service account JSON to get project ID
	var serviceAccount map[string]interface{}
	if err := json.Unmarshal([]byte(fcfg), &serviceAccount); err != nil {
		return fmt.Errorf("failed to parse service account JSON: %w", err)
	}

	projectID, ok := serviceAccount["project_id"].(string)
	if !ok {
		return fmt.Errorf("project_id not found in service account JSON")
	}

	// Create Google credentials with required scopes
	scopes := []string{
		"https://www.googleapis.com/auth/identitytoolkit",
		"https://www.googleapis.com/auth/firebase",
		"https://www.googleapis.com/auth/cloud-platform",
	}

	creds, err := google.CredentialsFromJSON(ctx, []byte(fcfg), scopes...)
	if err != nil {
		return fmt.Errorf("failed to create credentials: %w", err)
	}

	// Get access token
	token, err := creds.TokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Prepare the request body
	config := AuthorizedDomainsConfig{
		AuthorizedDomains: domains,
	}

	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Construct the API URL
	url := fmt.Sprintf("https://identitytoolkit.googleapis.com/admin/v2/projects/%s/config", projectID)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read response body for error details
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, buf.String())
	}

	return nil
}
