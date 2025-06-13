package service

import (
	"context"
	"net/http"
	"testing"

	"firebase.google.com/go/auth"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			domains:        []string{"hey.alineo.com", "bo.hey.alineo.com"},
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
			domains:        []string{"hey.alineo.com", "bo.hey.alineo.com"},
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
