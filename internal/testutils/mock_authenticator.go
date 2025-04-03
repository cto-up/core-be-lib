package testutils

import (
	"context"
)

// MockAuthenticator is a mock implementation of authentication functionality for testing
type MockAuthenticator struct {
	// Add fields to track calls and store predefined responses
	CreateTokenCalled bool
	VerifyTokenCalled bool
	MockToken         string
	MockError         error
}

// CreateToken is a mock implementation of token creation
func (m *MockAuthenticator) CreateToken(ctx context.Context, claims map[string]interface{}) (string, error) {
	m.CreateTokenCalled = true
	if m.MockError != nil {
		return "", m.MockError
	}
	return m.MockToken, nil
}

// VerifyToken is a mock implementation of token verification
func (m *MockAuthenticator) VerifyToken(ctx context.Context, token string) (map[string]interface{}, error) {
	m.VerifyTokenCalled = true
	if m.MockError != nil {
		return nil, m.MockError
	}
	return map[string]interface{}{}, nil
}
