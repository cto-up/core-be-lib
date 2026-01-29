package core

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/auth/kratos"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// MFAHandler handles MFA-related endpoints
type MFAHandler struct {
	authProvider auth.AuthProvider
}

// NewMFAHandler creates a new MFA handler
func NewMFAHandler(authProvider auth.AuthProvider) *MFAHandler {
	return &MFAHandler{
		authProvider: authProvider,
	}
}

// GetMFAStatus returns the MFA configuration status for the current user
// (GET /api/v1/mfa/status)
func (h *MFAHandler) GetMFAStatus(c *gin.Context) {
	// Check if provider is Kratos
	kratosProvider, ok := h.authProvider.(*kratos.KratosAuthProvider)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_supported",
			"message": "MFA is only supported with Kratos authentication provider",
		})
		return
	}

	status, err := kratosProvider.GetMFAStatus(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get MFA status")

		// Check error type
		if authErr, ok := err.(*auth.AuthError); ok {
			if authErr.Code == "unauthorized" {
				c.JSON(http.StatusUnauthorized, err)
				return
			}
		}

		c.JSON(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, status)
}

// InitializeSettingsFlow creates a new settings flow for MFA configuration
// (POST /api/v1/mfa/settings/init)
func (h *MFAHandler) InitializeSettingsFlow(c *gin.Context) {
	// Check if provider is Kratos
	kratosProvider, ok := h.authProvider.(*kratos.KratosAuthProvider)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_supported",
			"message": "MFA is only supported with Kratos authentication provider",
		})
		return
	}

	flow, err := kratosProvider.InitializeSettingsFlow(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize settings flow")

		// Check error type
		if authErr, ok := err.(*auth.AuthError); ok {
			if authErr.Code == "unauthorized" {
				c.JSON(http.StatusUnauthorized, err)
				return
			}
		}

		c.JSON(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, flow)
}

// DisableWebAuthn completely removes all WebAuthn credentials for the current user
// (DELETE /api/v1/mfa/webauthn)
func (h *MFAHandler) DisableWebAuthn(c *gin.Context) {
	// Check if provider is Kratos
	kratosProvider, ok := h.authProvider.(*kratos.KratosAuthProvider)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_supported",
			"message": "MFA is only supported with Kratos authentication provider",
		})
		return
	}

	err := kratosProvider.DisableWebAuthn(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to disable WebAuthn")

		// Check error type
		if authErr, ok := err.(*auth.AuthError); ok {
			if authErr.Code == "unauthorized" {
				c.JSON(http.StatusUnauthorized, err)
				return
			}
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to disable WebAuthn",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "WebAuthn disabled successfully",
	})
}
