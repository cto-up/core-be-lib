package service

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/identitytoolkit/v2"
	"google.golang.org/api/option"
)

// AuthorizedDomainsConfig represents the structure for authorized domains
type AuthorizedDomainsConfig struct {
	AuthorizedDomains []string `json:"authorizedDomains"`
}

// createIdentityToolkitService creates a new Identity Toolkit service client
func createIdentityToolkitService(ctx context.Context) (*identitytoolkit.Service, string, error) {
	fcfg := os.Getenv("FIREBASE_CONFIG")
	if fcfg == "" {
		return nil, "", fmt.Errorf("missing FIREBASE_CONFIG environment variable")
	}

	// Parse service account to get project ID
	creds, err := google.CredentialsFromJSON(ctx, []byte(fcfg),
		"https://www.googleapis.com/auth/identitytoolkit",
		"https://www.googleapis.com/auth/firebase",
		"https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create credentials: %w", err)
	}

	// Create the service
	service, err := identitytoolkit.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Identity Toolkit service: %w", err)
	}

	// Extract project ID from credentials
	projectID := creds.ProjectID
	if projectID == "" {
		return nil, "", fmt.Errorf("project ID not found in credentials")
	}

	return service, projectID, nil
}

// SDKAddAuthorizedDomains adds domains using the SDK with proper field masking
func SDKAddAuthorizedDomains(ctx context.Context, domainsToAdd []string) error {
	service, projectID, err := createIdentityToolkitService(ctx)
	if err != nil {
		return err
	}

	// Get current configuration
	configName, currentConfig, err := GetFirebaseConfig(ctx, projectID, service)
	if err != nil {
		return fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Create domain map for deduplication
	domainMap := make(map[string]bool)
	currentDomains := currentConfig.AuthorizedDomains
	if currentDomains == nil {
		currentDomains = []string{}
	}

	for _, domain := range currentDomains {
		domainMap[domain] = true
	}

	// Add new domains
	updatedDomains := make([]string, len(currentDomains))
	copy(updatedDomains, currentDomains)

	for _, domain := range domainsToAdd {
		if !domainMap[domain] {
			updatedDomains = append(updatedDomains, domain)
		}
	}

	// Update configuration with new domains
	config := &identitytoolkit.GoogleCloudIdentitytoolkitAdminV2Config{
		AuthorizedDomains: updatedDomains,
	}

	_, err = service.Projects.UpdateConfig(configName, config).
		UpdateMask("authorizedDomains").
		Context(ctx).
		Do()

	if err != nil {
		return fmt.Errorf("failed to add authorized domains: %w", err)
	}

	return nil
}

func GetFirebaseConfig(ctx context.Context, projectID string, service *identitytoolkit.Service) (string, *identitytoolkit.GoogleCloudIdentitytoolkitAdminV2Config, error) {
	configName := fmt.Sprintf("projects/%s/config", projectID)
	currentConfig, err := service.Projects.GetConfig(configName).Context(ctx).Do()
	return configName, currentConfig, err
}

// SDKRemoveAuthorizedDomains removes domains using the SDK with proper field masking
func SDKRemoveAuthorizedDomains(ctx context.Context, domainsToRemove []string) error {
	service, projectID, err := createIdentityToolkitService(ctx)
	if err != nil {
		return err
	}

	// Get current configuration
	// Get current configuration
	configName, currentConfig, err := GetFirebaseConfig(ctx, projectID, service)
	if err != nil {
		return fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Create remove map for efficient lookup
	removeMap := make(map[string]bool)
	for _, domain := range domainsToRemove {
		removeMap[domain] = true
	}

	// Filter out domains to remove
	var updatedDomains []string
	currentDomains := currentConfig.AuthorizedDomains

	for _, domain := range currentDomains {
		if !removeMap[domain] {
			updatedDomains = append(updatedDomains, domain)
		}
	}

	// Update configuration
	config := &identitytoolkit.GoogleCloudIdentitytoolkitAdminV2Config{
		AuthorizedDomains: updatedDomains,
	}

	_, err = service.Projects.UpdateConfig(configName, config).
		UpdateMask("authorizedDomains").
		Context(ctx).
		Do()

	if err != nil {
		return fmt.Errorf("failed to remove authorized domains: %w", err)
	}

	return nil
}
