package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"firebase.google.com/go/auth"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFirebaseConfig(t *testing.T) {
	// Set up test environment variables
	godotenv.Load("../../../.env")
	godotenv.Overload("../../../.env", "../../../.env.local")

	// Create a context
	ctx := context.Background()

	// Initialize Firebase Auth client

	service, projectID, err := createIdentityToolkitService(ctx)
	require.NoError(t, err)

	// Get the config
	configName, currentConfig, err := GetFirebaseConfig(ctx, projectID, service)
	// write out the config into a file called config.json
	file, err := os.Create("config.json")
	require.NoError(t, err)
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(currentConfig)
	require.NoError(t, err)

	require.NoError(t, err)
	require.NotNil(t, currentConfig)
	assert.Equal(t, configName, fmt.Sprintf("projects/%s/config", projectID))
}

func TestAddAuthorizeDomains(t *testing.T) {
	// Set up test environment variables
	godotenv.Load("../../../.env")
	godotenv.Overload("../../../.env", "../../../.env.local")

	tests := []struct {
		name           string
		domains        []string
		expectedError  string
		expectedStatus int
	}{
		{
			name:           "successful authorization",
			domains:        []string{"hey.alineo.com", "ho.alineo.com"},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock auth client
			authClient := &auth.Client{}

			// Call the function
			// initialize Firebase Auth client
			ctx := context.Background()
			authClient, err := newFirebaseClient(ctx)
			require.NoError(t, err)

			err = SDKAddAuthorizedDomains(ctx, authClient, tt.domains)

			// Check results
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteAuthorizeDomains(t *testing.T) {
	// Set up test environment variables
	godotenv.Load("../../../.env")
	godotenv.Overload("../../../.env", "../../../.env.local")

	tests := []struct {
		name           string
		domains        []string
		expectedError  string
		expectedStatus int
	}{
		{
			name:           "successful authorization",
			domains:        []string{"hey.alineo.com", "bohey.alineo.com"},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock auth client
			authClient := &auth.Client{}

			// Call the function
			// initialize Firebase Auth client
			ctx := context.Background()
			authClient, err := newFirebaseClient(ctx)
			require.NoError(t, err)

			err = SDKRemoveAuthorizedDomains(ctx, authClient, tt.domains)

			// Check results
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
