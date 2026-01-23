package kratos

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

// MFAStatus represents the MFA configuration status for a user
type MFAStatus struct {
	TOTPEnabled      bool     `json:"totp_enabled"`
	WebAuthnEnabled  bool     `json:"webauthn_enabled"`
	RecoveryCodesSet bool     `json:"recovery_codes_set"`
	AvailableMethods []string `json:"available_methods"`
	AAL              string   `json:"aal"` // Current Authenticator Assurance Level
}

// GetMFAStatus returns the MFA configuration status for the current user
func (k *KratosAuthProvider) GetMFAStatus(c *gin.Context) (*MFAStatus, error) {
	sessionCookie, err := c.Cookie("ory_kratos_session")
	if err != nil {
		return nil, &AuthError{Code: "unauthorized", Message: "Not authenticated"}
	}

	// Get session from Kratos
	cookieString := "ory_kratos_session=" + sessionCookie
	session, resp, err := k.publicClient.FrontendAPI.ToSession(context.Background()).
		Cookie(cookieString).
		Execute()

	if err != nil || resp.StatusCode != 200 {
		return nil, &AuthError{Code: "unauthorized", Message: "Invalid session"}
	}

	identity := session.Identity
	status := &MFAStatus{
		TOTPEnabled:      false,
		WebAuthnEnabled:  false,
		RecoveryCodesSet: false,
		AvailableMethods: []string{"totp", "webauthn", "lookup_secret"},
		AAL:              "aal1", // Default
	}

	// Get current AAL from session
	if session.AuthenticatorAssuranceLevel != nil {
		status.AAL = string(*session.AuthenticatorAssuranceLevel)
	}

	// Check TOTP credentials
	if identity.Credentials != nil {
		if credentials, ok := (*identity.Credentials)["totp"]; ok {
			if credentials.Config != nil {
				status.TOTPEnabled = true
			}
		}

		// Check WebAuthn credentials
		if credentials, ok := (*identity.Credentials)["webauthn"]; ok {
			if credentials.Config != nil {
				if creds, ok := credentials.Config["credentials"].([]interface{}); ok {
					status.WebAuthnEnabled = len(creds) > 0
				}
			}
		}

		// Check recovery codes
		if credentials, ok := (*identity.Credentials)["lookup_secret"]; ok {
			if credentials.Config != nil {
				status.RecoveryCodesSet = true
			}
		}
	}

	return status, nil
}

// InitializeSettingsFlow creates a new settings flow for MFA configuration
func (k *KratosAuthProvider) InitializeSettingsFlow(c *gin.Context) (*ory.SettingsFlow, error) {
	cookieHeader := c.GetHeader("Cookie")
	if cookieHeader == "" {
		return nil, &AuthError{Code: "unauthorized", Message: "Not authenticated"}
	}

	// Create context with Cookie header
	ctx := context.WithValue(c.Request.Context(), ory.ContextAPIKeys, map[string]ory.APIKey{
		"Cookie": {
			Key: cookieHeader,
		},
	})

	// Create settings flow - SDK automatically adds Cookie header from context
	flow, resp, err := k.publicClient.FrontendAPI.CreateBrowserSettingsFlow(ctx).Cookie(cookieHeader).Execute()

	if err != nil || resp.StatusCode != 200 {
		log.Error().Err(err).Msg("Failed to create settings flow")
		return nil, &AuthError{Code: "internal", Message: "Failed to create settings flow"}
	}
	return flow, nil
}

// AuthError represents an authentication error
type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// AAL2Middleware is a middleware that ensures the user has completed MFA (AAL2)
type AAL2Middleware struct {
	provider *KratosAuthProvider
}

// NewAAL2Middleware creates a new AAL2 middleware
func NewAAL2Middleware(provider *KratosAuthProvider) *AAL2Middleware {
	return &AAL2Middleware{provider: provider}
}

// RequireAAL2 returns a Gin middleware that enforces AAL2 (MFA required)
func (m *AAL2Middleware) RequireAAL2() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionCookie, err := c.Cookie("ory_kratos_session")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication_required",
			})
			c.Abort()
			return
		}

		cookieString := "ory_kratos_session=" + sessionCookie
		session, _, err := m.provider.publicClient.FrontendAPI.ToSession(context.Background()).
			Cookie(cookieString).
			Execute()

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_session",
			})
			c.Abort()
			return
		}

		// Check AAL level
		aal := "aal1" // Default
		if session.AuthenticatorAssuranceLevel != nil {
			aal = string(*session.AuthenticatorAssuranceLevel)
		}

		if aal != "aal2" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":        "mfa_required",
				"message":      "This action requires MFA verification",
				"require_aal2": true,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
