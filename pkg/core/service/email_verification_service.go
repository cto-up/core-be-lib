package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/emailservice"
	access "ctoup.com/coreapp/pkg/shared/service"
	utils "ctoup.com/coreapp/pkg/shared/util"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const (
	// Token expiration time (24 hours)
	EmailVerificationTokenExpiry = 24 * time.Hour
	// Token length in bytes (will be base64 encoded)
	TokenLength = 32
)

type EmailVerificationService struct {
	store          *db.Store
	authClientPool *access.FirebaseTenantClientConnectionPool
}

func NewEmailVerificationService(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *EmailVerificationService {
	return &EmailVerificationService{
		store:          store,
		authClientPool: authClientPool,
	}
}

// GenerateVerificationToken creates a secure random token
func (s *EmailVerificationService) GenerateVerificationToken() (string, []byte, error) {
	// Generate random bytes
	tokenBytes := make([]byte, TokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random token: %w", err)
	}

	// Create base64 encoded token for URL safety
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Create hash for storage
	hash := sha256.Sum256([]byte(token))

	return token, hash[:], nil
}

// CreateEmailVerificationToken creates and stores a new verification token
func (s *EmailVerificationService) CreateEmailVerificationToken(ctx *gin.Context, userID, tenantID string) (string, error) {
	// Delete any existing tokens for this user
	if err := s.store.DeleteEmailVerificationTokensByUserID(ctx, repository.DeleteEmailVerificationTokensByUserIDParams{
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		log.Error().Err(err).Msg("Failed to delete existing verification tokens")
		// Continue anyway, as this is not critical
	}

	// Generate new token
	token, tokenHash, err := s.GenerateVerificationToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Store token in database
	expiresAt := time.Now().Add(EmailVerificationTokenExpiry)
	_, err = s.store.CreateEmailVerificationToken(ctx, repository.CreateEmailVerificationTokenParams{
		UserID:    userID,
		TenantID:  tenantID,
		Token:     token,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("failed to store verification token: %w", err)
	}

	return token, nil
}

// VerifyEmailToken verifies a token and marks the user's email as verified in Firebase
func (s *EmailVerificationService) VerifyEmailToken(ctx *gin.Context, token string, tenantID string) error {
	// Get token from database
	tokenRecord, err := s.store.GetEmailVerificationToken(ctx, repository.GetEmailVerificationTokenParams{
		Token:    token,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("invalid or expired verification token")
	}

	subdomain, err := utils.GetSubdomain(ctx)
	if err != nil {
		return err
	}

	// Get Firebase auth client for the tenant
	authClient, err := s.authClientPool.GetBaseAuthClient(ctx, subdomain)
	if err != nil {
		return fmt.Errorf("failed to get Firebase auth client: %w", err)
	}

	// Update user's email verification status in Firebase
	userUpdate := (&auth.UserToUpdate{}).EmailVerified(true)
	if _, err := authClient.UpdateUser(ctx, tokenRecord.UserID, userUpdate); err != nil {
		return fmt.Errorf("failed to update user email verification status in Firebase: %w", err)
	}

	// Mark token as used
	if err := s.store.MarkEmailVerificationTokenAsUsed(ctx, repository.MarkEmailVerificationTokenAsUsedParams{
		Token:    token,
		TenantID: tenantID,
	}); err != nil {
		log.Error().Err(err).Msg("Failed to mark token as used")
		// Continue anyway, as the main operation (Firebase update) succeeded
	}

	return nil
}

// SendVerificationEmail sends a verification email to the user
func (s *EmailVerificationService) SendVerificationEmail(ctx *gin.Context, email, token, baseFrontendURL string) error {
	fromEmail := getSystemEmail()

	// Create verification URL
	verificationURL := fmt.Sprintf("%s/verify-email?token=%s", baseFrontendURL, token)

	// Prepare template data
	templateData := struct {
		Link  string
		Email string
	}{
		Link:  verificationURL,
		Email: email,
	}

	// Create email request
	r := emailservice.NewEmailRequest(fromEmail, []string{email}, "Please verify your email address", "")

	if err := r.ParseTemplateWithDomain(ctx, "email-verification.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse email verification template")
		return fmt.Errorf("failed to prepare verification email: %w", err)
	}

	// Send email
	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send verification email")
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}

// ResendVerificationEmail creates a new token and resends verification email with rate limiting
func (s *EmailVerificationService) ResendVerificationEmail(ctx *gin.Context, userID, tenantID, email, baseFrontendURL string) error {
	// Check rate limit
	if err := CheckEmailVerificationRateLimit(ctx, userID); err != nil {
		return err
	}

	// Create new verification token
	token, err := s.CreateEmailVerificationToken(ctx, userID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	// Send verification email
	if err := s.SendVerificationEmail(ctx, email, token, baseFrontendURL); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}

// GetUserVerificationStatus returns the email verification status of a user from Firebase
func (s *EmailVerificationService) GetUserVerificationStatus(ctx *gin.Context, userID, tenantID string) (bool, error) {
	// Get Firebase auth client for the tenant
	authClient, err := s.authClientPool.GetBaseAuthClientForTenant(tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get Firebase auth client: %w", err)
	}

	// Get user record from Firebase
	userRecord, err := authClient.GetUser(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user from Firebase: %w", err)
	}

	return userRecord.EmailVerified, nil
}

// CleanupExpiredTokens removes expired verification tokens
func (s *EmailVerificationService) CleanupExpiredTokens(ctx *gin.Context) error {
	if err := s.store.DeleteExpiredEmailVerificationTokens(ctx); err != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}
	return nil
}

// Helper function to get system email
func getSystemEmail() string {
	// This should match the pattern used in other email functions
	return "noreply@ctoup.com" // You might want to make this configurable
}
