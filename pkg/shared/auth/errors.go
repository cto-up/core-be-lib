package auth

import (
	"encoding/json"
	"errors"

	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

// Common error types
type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *AuthError) Error() string {
	return e.Message
}

// Error codes
const (
	ErrorCodeEmailAlreadyExists  = "email-already-exists"
	ErrorCodeInvalidPassword     = "invalid-password"
	ErrorCodeTenantNotFound      = "tenant-not-found"
	ErrorCodeSessionAAL2Required = "session_aal2_required"
	ErrorCodeMFANotConfigured    = "mfa_not_configured"
	ErrorCodeInvalidToken        = "invalid_token"
	ErrorCodeUserNotFound        = "user_not_found"
	ErrorCodeUnauthorized        = "unauthorized"
	ErrorCodeForbidden           = "forbidden"
)

// Helper functions for error checking
func IsUserNotFound(err error) bool {
	if authErr, ok := err.(*AuthError); ok {
		return authErr.Code == ErrorCodeUserNotFound
	}
	return false
}

func IsEmailAlreadyExists(err error) bool {
	if authErr, ok := err.(*AuthError); ok {
		return authErr.Code == ErrorCodeEmailAlreadyExists
	}
	return false
}

func ConvertKratosError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *ory.GenericOpenAPIError
	if !errors.As(err, &apiErr) {
		log.Err(err).Msg("Non-Kratos error encountered")
		return &AuthError{
			Code:    "unknown-error",
			Message: err.Error(),
		}
	}

	// Default values
	errorCode := "kratos-error"
	message := apiErr.Error()

	// 1. Try to get ID from the Model
	model := apiErr.Model()
	if model != nil {
		// Kratos often uses ErrorGeneric for API errors
		if eg, ok := model.(ory.ErrorGeneric); ok {
			if eg.Error.Id != nil {
				errorCode = *eg.Error.Id
			}
			message = eg.Error.Message
			if eg.Error.Reason != nil {
				message += " reason: " + *eg.Error.Reason
			}
		} else if fe, ok := model.(ory.FlowError); ok {
			// Some flows (like settings/login) return FlowError
			if fe.Id != "" {
				errorCode = fe.Id
			}
		}
	}

	// 2. Fallback: If Code is still generic, try parsing the raw JSON body
	// (Useful if the SDK model mapping fails)
	if errorCode == "kratos-error" {
		var raw struct {
			Error struct {
				ID string `json:"id"`
			} `json:"error"`
			ID string `json:"id"` // Some responses have ID at top level
		}
		if jsonErr := json.Unmarshal(apiErr.Body(), &raw); jsonErr == nil {
			if raw.Error.ID != "" {
				errorCode = raw.Error.ID
			} else if raw.ID != "" {
				errorCode = raw.ID
			}
		}
	}

	return &AuthError{
		Code:    errorCode,
		Message: message,
	}
}

// NewAuthError creates a new AuthError
func NewAuthError(code, message string) *AuthError {
	return &AuthError{
		Code:    code,
		Message: message,
	}
}
