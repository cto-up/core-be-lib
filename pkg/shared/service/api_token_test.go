package service

import (
	"net/http"
	"testing"
	"time"

	commontestutils "ctoup.com/coreapp/internal/testutils"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func setupTestAPITokenService(t *testing.T) (*ClientApplicationService, *commontestutils.MockAuthenticator, *gin.Context) {
	store := testutils.NewTestStore(t)
	mockAuth := &commontestutils.MockAuthenticator{}
	service := NewClientApplicationService(store)
	ctx := &gin.Context{}
	// Set any required context values
	ctx.Request = &http.Request{
		Header: make(http.Header),
	}
	return service, mockAuth, ctx
}

func TestCreateAPIToken(t *testing.T) {
	service, _, ctx := setupTestAPITokenService(t)

	t.Run("successful creation", func(t *testing.T) {
		app := createTestClientApplication(t, service)
		name := commontestutils.RandomString(10)
		description := commontestutils.RandomString(20)
		createdBy := commontestutils.RandomString(10)
		expiryDays := 30
		scopes := []string{"read", "write"}

		token, apiToken, err := service.CreateAPIToken(
			ctx,
			app.ID,
			name,
			description,
			expiryDays,
			createdBy,
			scopes,
		)

		require.NoError(t, err)
		require.NotEmpty(t, token)
		require.NotEmpty(t, apiToken)
		require.Equal(t, name, apiToken.Name)
		require.Equal(t, description, apiToken.Description.String)
		require.Equal(t, createdBy, apiToken.CreatedBy)
		require.Equal(t, scopes, apiToken.Scopes)
		require.False(t, apiToken.Revoked)
		require.NotEmpty(t, apiToken.TokenPrefix)
		require.NotEmpty(t, apiToken.TokenHash)

		// Verify expiry date is set correctly
		expectedExpiry := time.Now().AddDate(0, 0, expiryDays)
		require.WithinDuration(t, expectedExpiry, apiToken.ExpiresAt, time.Minute)
	})

	t.Run("invalid client application ID", func(t *testing.T) {
		name := commontestutils.RandomString(10)
		invalidID := uuid.New()

		_, _, err := service.CreateAPIToken(
			ctx,
			invalidID,
			name,
			"description",
			30,
			"creator",
			nil,
		)

		require.Error(t, err)
	})
}

func TestGetAPIToken(t *testing.T) {
	service, _, ctx := setupTestAPITokenService(t)

	t.Run("get existing token", func(t *testing.T) {
		// Create test application and token
		app := createTestClientApplication(t, service)
		token, apiToken, err := service.CreateAPIToken(
			ctx,
			app.ID,
			"test token",
			"description",
			30,
			"creator",
			[]string{"read"},
		)
		require.NoError(t, err)

		// Get the token
		found, err := service.GetAPITokenByID(ctx, apiToken.ID, app.TenantID.String)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.NotNil(t, token)
		require.Equal(t, apiToken.ID, found.ID)
		require.Equal(t, apiToken.Name, found.Name)
	})

	t.Run("token not found", func(t *testing.T) {
		invalidID := uuid.New()
		_, err := service.GetAPITokenByID(ctx, invalidID, "tenant123")
		require.Error(t, err)
	})
}

func TestRevokeAPIToken(t *testing.T) {
	service, _, ctx := setupTestAPITokenService(t)

	t.Run("successful revocation", func(t *testing.T) {
		// Create test application and token
		app := createTestClientApplication(t, service)
		_, apiToken, err := service.CreateAPIToken(
			ctx,
			app.ID,
			"test token",
			"description",
			30,
			"creator",
			[]string{"read"},
		)
		require.NoError(t, err)

		// Revoke the token
		reason := "Security concern"
		revokedBy := "admin"
		revoked, err := service.RevokeAPIToken(ctx, apiToken.ID, reason, revokedBy)
		require.NoError(t, err)
		require.True(t, revoked.Revoked)
		require.Equal(t, reason, revoked.RevokedReason.String)
		require.Equal(t, revokedBy, revoked.RevokedBy.String)
		require.NotNil(t, revoked.RevokedAt)
	})

	t.Run("revoke non-existent token", func(t *testing.T) {
		invalidID := uuid.New()
		_, err := service.RevokeAPIToken(ctx, invalidID, "reason", "admin")
		require.Error(t, err)
	})
}

func TestListAPITokens(t *testing.T) {
	service, _, ctx := setupTestAPITokenService(t)

	t.Run("list tokens with pagination", func(t *testing.T) {
		// Create test application
		app := createTestClientApplication(t, service)

		// Create multiple tokens
		for i := 0; i < 3; i++ {
			_, _, err := service.CreateAPIToken(
				ctx,
				app.ID,
				commontestutils.RandomString(10),
				"test token",
				30,
				"creator",
				[]string{"read"},
			)
			require.NoError(t, err)
		}

		// List tokens with pagination
		tokens, err := service.ListAPITokens(ctx, &app.ID, "", 2, 0, "created_at", "desc", false, false)
		require.NoError(t, err)
		require.Len(t, tokens, 2)

		// Verify sorting
		require.True(t, tokens[0].CreatedAt.After(tokens[1].CreatedAt))
	})

	t.Run("list tokens with filters", func(t *testing.T) {
		app := createTestClientApplication(t, service)

		// Create active and revoked tokens
		_, _, err := service.CreateAPIToken(
			ctx,
			app.ID,
			"active token",
			"description",
			30,
			"creator",
			[]string{"read"},
		)
		require.NoError(t, err)

		_, revokedToken, err := service.CreateAPIToken(
			ctx,
			app.ID,
			"revoked token",
			"description",
			30,
			"creator",
			[]string{"read"},
		)
		require.NoError(t, err)

		// Revoke one token
		_, err = service.RevokeAPIToken(ctx, revokedToken.ID, "test", "admin")
		require.NoError(t, err)

		// List only active tokens
		tokens, err := service.ListAPITokens(ctx, &app.ID, "", 10, 0, "created_at", "desc", false, false)
		require.NoError(t, err)
		for _, token := range tokens {
			require.False(t, token.Revoked)
		}

		// List including revoked tokens
		tokens, err = service.ListAPITokens(ctx, &app.ID, "", 10, 0, "created_at", "desc", true, false)
		require.NoError(t, err)
		found := false
		for _, token := range tokens {
			if token.Revoked {
				found = true
				break
			}
		}
		require.True(t, found)
	})
}

func TestDeleteAPIToken(t *testing.T) {
	service, _, ctx := setupTestAPITokenService(t)

	t.Run("successful deletion", func(t *testing.T) {
		// Create test application and token
		app := createTestClientApplication(t, service)
		_, apiToken, err := service.CreateAPIToken(
			ctx,
			app.ID,
			"test token",
			"description",
			30,
			"creator",
			[]string{"read"},
		)
		require.NoError(t, err)

		// Delete the token
		err = service.DeleteAPIToken(ctx, apiToken.ID)
		require.NoError(t, err)

		// Verify token is deleted
		_, err = service.GetAPITokenByID(ctx, apiToken.ID, app.TenantID.String)
		require.Error(t, err)
	})

	t.Run("delete non-existent token", func(t *testing.T) {
		invalidID := uuid.New()
		err := service.DeleteAPIToken(ctx, invalidID)
		require.Error(t, err)
	})
}
