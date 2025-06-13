package service

import (
	"context"
	"net/http"
	"testing"

	"firebase.google.com/go/auth"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockHTTPClient is a mock HTTP client for testing
type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

// MockAuthClient is a mock Firebase Auth client for testing
type MockAuthClient struct {
	mock.Mock
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
