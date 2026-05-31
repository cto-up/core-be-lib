package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	AUTH_CURRENT_AAL_KEY   = "auth_current_aal"   // Current session AAL
	AUTH_AVAILABLE_AAL_KEY = "auth_available_aal" // Highest AAL user can achieve
	AUTH_AAL_INFO_KEY      = "auth_aal_info"      // Complete AAL info
)

// AALInfo contains both current and available AAL levels
type AALInfo struct {
	Current      string // Current session AAL (aal1 or aal2)
	Available    string // Highest AAL user can achieve (aal1 or aal2)
	CanUpgrade   bool   // True if user has MFA configured but not verified in session
	IsAAL2Recent bool   // True if user recently verified AAL2 (used for grace periods)
}

// GetAALInfo retrieves the AAL information from context
func GetAALInfo(c *gin.Context) *AALInfo {
	if aalInfo, exists := c.Get(AUTH_AAL_INFO_KEY); exists {
		if info, ok := aalInfo.(*AALInfo); ok {
			return info
		}
	}
	return &AALInfo{
		Current:    "aal1",
		Available:  "aal1",
		CanUpgrade: false,
	}
}

// GetCurrentAAL retrieves just the current AAL level
func GetCurrentAAL(c *gin.Context) string {
	return GetAALInfo(c).Current
}

// GetAvailableAAL retrieves the highest AAL the user can achieve
func GetAvailableAAL(c *gin.Context) string {
	return GetAALInfo(c).Available
}

// CanUpgradeToAAL2 checks if user can upgrade to AAL2
func CanUpgradeToAAL2(c *gin.Context) bool {
	return GetAALInfo(c).CanUpgrade
}

// HasMFAConfigured checks if user has any MFA method configured
func HasMFAConfigured(c *gin.Context) bool {
	return GetAALInfo(c).Available == "aal2"
}

// IsAAL2Active checks if current session is at AAL2
func IsAAL2Active(c *gin.Context) bool {
	return GetAALInfo(c).Current == "aal2"
}

// RequireAAL2StepUp enforces a step-up to AAL2 (MFA) for a sensitive
// operation. It writes the HTTP response and returns false when the request
// must be blocked; it returns true (and writes nothing) when the caller may
// proceed.
//
// The action ALWAYS requires AAL2 — no graceful degradation. When the session
// is not already aal2:
//
//   - User HAS a second factor available → 403 with the Kratos-shaped body
//     {id: "session_aal2_required"}. Frontends wiring the core-fe-lib axios
//     interceptor recognise exactly that shape, open the AAL2 verification
//     dialog, and retry the original request after the user completes MFA.
//   - User has NO second factor enrolled (cannot reach aal2) → 403 with
//     {id: "mfa_not_configured"} telling them to enable two-factor auth first.
//     Returning session_aal2_required there would loop them into a flow they
//     cannot complete.
func RequireAAL2StepUp(c *gin.Context) bool {
	info := GetAALInfo(c)
	if info != nil && info.Current == "aal2" {
		return true
	}

	if info != nil && info.Available == "aal2" {
		c.JSON(http.StatusForbidden, gin.H{
			"id":      ErrorCodeSessionAAL2Required,
			"message": "This action requires two-factor verification.",
		})
		c.Abort()
		return false
	}

	c.JSON(http.StatusForbidden, gin.H{
		"id":      ErrorCodeMFANotConfigured,
		"message": "This sensitive action requires two-factor authentication. Enable it in your Security settings, then try again.",
	})
	c.Abort()
	return false
}
