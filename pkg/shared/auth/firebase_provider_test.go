package auth

import (
	"context"
	"testing"

	"firebase.google.com/go/auth"
)

// MockMultitenantService for testing
type MockMultitenantService struct {
	tenantMap map[string]string
}

func (m *MockMultitenantService) GetFirebaseTenantID(ctx context.Context, subdomain string) (string, error) {
	if tenantID, ok := m.tenantMap[subdomain]; ok {
		return tenantID, nil
	}
	return "", &AuthError{
		Code:    ErrorCodeTenantNotFound,
		Message: "tenant not found",
	}
}

// MockFirebaseClient for testing
type MockFirebaseClient struct {
	users map[string]*auth.UserRecord
}

func (m *MockFirebaseClient) CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error) {
	// Mock implementation
	return &auth.UserRecord{
		UserInfo: &auth.UserInfo{
			UID:   "test-uid",
			Email: "test@example.com",
		},
		UserMetadata: &auth.UserMetadata{
			CreationTimestamp: 1234567890,
		},
	}, nil
}

func (m *MockFirebaseClient) UpdateUser(ctx context.Context, uid string, user *auth.UserToUpdate) (*auth.UserRecord, error) {
	return &auth.UserRecord{
		UserInfo: &auth.UserInfo{
			UID: uid,
		},
		UserMetadata: &auth.UserMetadata{
			CreationTimestamp: 1234567890,
		},
	}, nil
}

func (m *MockFirebaseClient) DeleteUser(ctx context.Context, uid string) error {
	return nil
}

func (m *MockFirebaseClient) GetUser(ctx context.Context, uid string) (*auth.UserRecord, error) {
	return &auth.UserRecord{
		UserInfo: &auth.UserInfo{
			UID: uid,
		},
		UserMetadata: &auth.UserMetadata{
			CreationTimestamp: 1234567890,
		},
	}, nil
}

func (m *MockFirebaseClient) GetUserByEmail(ctx context.Context, email string) (*auth.UserRecord, error) {
	return &auth.UserRecord{
		UserInfo: &auth.UserInfo{
			Email: email,
		},
		UserMetadata: &auth.UserMetadata{
			CreationTimestamp: 1234567890,
		},
	}, nil
}

func (m *MockFirebaseClient) SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error {
	return nil
}

func (m *MockFirebaseClient) EmailVerificationLink(ctx context.Context, email string) (string, error) {
	return "https://example.com/verify", nil
}

func (m *MockFirebaseClient) PasswordResetLink(ctx context.Context, email string) (string, error) {
	return "https://example.com/reset", nil
}

func (m *MockFirebaseClient) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return "https://example.com/verify", nil
}

func (m *MockFirebaseClient) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return "https://example.com/reset", nil
}

func (m *MockFirebaseClient) EmailSignInLink(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return "https://example.com/signin", nil
}

func (m *MockFirebaseClient) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	return &auth.Token{
		UID: "test-uid",
	}, nil
}

func TestFirebaseAuthClient_CreateUser(t *testing.T) {
	mockClient := &MockFirebaseClient{
		users: make(map[string]*auth.UserRecord),
	}

	fbClient := &FirebaseAuthClient{
		client: mockClient,
	}

	user := (&UserToCreate{}).
		Email("test@example.com").
		DisplayName("Test User").
		EmailVerified(false)

	record, err := fbClient.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if record.UID != "test-uid" {
		t.Errorf("Expected UID 'test-uid', got '%s'", record.UID)
	}
}

func TestFirebaseAuthClient_GetUser(t *testing.T) {
	mockClient := &MockFirebaseClient{
		users: make(map[string]*auth.UserRecord),
	}

	fbClient := &FirebaseAuthClient{
		client: mockClient,
	}

	record, err := fbClient.GetUser(context.Background(), "test-uid")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if record.UID != "test-uid" {
		t.Errorf("Expected UID 'test-uid', got '%s'", record.UID)
	}
}

func TestFirebaseAuthClient_SetCustomUserClaims(t *testing.T) {
	mockClient := &MockFirebaseClient{
		users: make(map[string]*auth.UserRecord),
	}

	fbClient := &FirebaseAuthClient{
		client: mockClient,
	}

	claims := map[string]interface{}{
		"ADMIN": true,
		"USER":  true,
	}

	err := fbClient.SetCustomUserClaims(context.Background(), "test-uid", claims)
	if err != nil {
		t.Fatalf("SetCustomUserClaims failed: %v", err)
	}
}

func TestErrorConversion(t *testing.T) {
	// Test user not found error
	err := &AuthError{
		Code:    ErrorCodeUserNotFound,
		Message: "user not found",
	}

	if !IsUserNotFound(err) {
		t.Error("Expected IsUserNotFound to return true")
	}

	// Test email already exists error
	err2 := &AuthError{
		Code:    ErrorCodeEmailAlreadyExists,
		Message: "email already exists",
	}

	if !IsEmailAlreadyExists(err2) {
		t.Error("Expected IsEmailAlreadyExists to return true")
	}
}
