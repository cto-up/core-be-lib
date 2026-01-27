package auth

import "github.com/gin-gonic/gin"

const (
	AUTH_CURRENT_AAL_KEY   = "auth_current_aal"   // Current session AAL
	AUTH_AVAILABLE_AAL_KEY = "auth_available_aal" // Highest AAL user can achieve
	AUTH_AAL_INFO_KEY      = "auth_aal_info"      // Complete AAL info
)

// AALInfo contains both current and available AAL levels
type AALInfo struct {
	Current    string // Current session AAL (aal1 or aal2)
	Available  string // Highest AAL user can achieve (aal1 or aal2)
	CanUpgrade bool   // True if user has MFA configured but not verified in session
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
