package core

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

type RecoveryHandler struct {
	kratosPublicURL string
	publicClient    *ory.APIClient
}

// extractBaseDomain extracts the base domain from a host string
// Examples:
//   - "corpb.ctoup.localhost:5173" -> ".localhost"
//   - "moncto.localhost:5173" -> ".localhost"
//   - "localhost:5173" -> ".localhost"
//   - "example.com:8080" -> ".example.com"
func extractBaseDomain(host string) string {
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// For localhost (with or without subdomains), always return ".localhost"
	// This ensures cookies work across all subdomains
	if strings.HasSuffix(host, "localhost") {
		return ".localhost"
	}

	// Split by dots
	parts := strings.Split(host, ".")

	// If it's just one part, return as-is
	if len(parts) <= 1 {
		return host
	}

	// For other domains, return last two parts with leading dot
	// e.g., "subdomain.example.com" -> ".example.com"
	return "." + parts[len(parts)-2] + "." + parts[len(parts)-1]
}

// NewRecoveryHandler creates a RecoveryHandler only if the provider requires recovery proxy
// Returns nil if provider doesn't require proxy
func NewRecoveryHandler(authProvider auth.AuthProvider) *RecoveryHandler {
	// Check if provider requires recovery proxy
	authClient := authProvider.GetAuthClient()
	if !authClient.RequiresRecoveryProxy() {
		log.Error().
			Str("provider", authProvider.GetProviderName()).
			Msg("Provider does not require recovery proxy, skipping RecoveryHandler creation")
		return nil
	}

	kratosPublicURL := os.Getenv("KRATOS_PUBLIC_URL")
	if kratosPublicURL == "" {
		kratosPublicURL = "http://localhost:4433"
	}

	// Get Kratos public client from the auth provider
	// This assumes the provider is KratosAuthProvider
	var publicClient *ory.APIClient
	if kratosClient, ok := authClient.(interface{ GetPublicClient() *ory.APIClient }); ok {
		publicClient = kratosClient.GetPublicClient()
	}
	return &RecoveryHandler{
		kratosPublicURL: kratosPublicURL,
		publicClient:    publicClient,
	}
}

// HandleRecovery proxies the recovery request to Kratos
// This activates the recovery link and creates a session
// Then returns the settings flow URL for password setup
// GET /public-api/v1/auth/recovery?flow=xxx&token=yyy
func (h *RecoveryHandler) HandleRecovery(c *gin.Context, params core.HandleRecoveryParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	flowID := params.Flow
	token := params.Token

	// Construct the Kratos recovery URL, with /admin/ prefix when applicable
	kratosPath := "/self-service/recovery"
	if isAdminParticuleRequest(c) {
		kratosPath = "/admin/self-service/recovery"
	}
	kratosURL := fmt.Sprintf("%s%s?flow=%s&token=%s", h.kratosPublicURL, kratosPath, flowID, token)

	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Make request to Kratos to activate the recovery link
	req, err := http.NewRequest("GET", kratosURL, nil)
	if err != nil {
		logger.Err(err).Msg("Failed to create Kratos request")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Err(err).Msg("Failed to call Kratos recovery endpoint")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	defer resp.Body.Close()

	// Copy session cookies from Kratos response to client
	// This logs the user in
	// IMPORTANT: We need to set the cookie domain to the base domain (e.g., .localhost)
	// so it works across all subdomains (corpb.ctoup.localhost, moncto.localhost, etc.)

	// Extract base domain from the Origin header (frontend domain)
	// The Host header shows the backend (localhost:7001) due to Vite proxy
	host := ""

	// Try Referer header first (most reliable with Vite proxy)
	referer := c.Request.Header.Get("Referer")
	if referer != "" {
		if parsedReferer, err := url.Parse(referer); err == nil {
			host = parsedReferer.Host
		}
	}

	// Try Origin header
	if host == "" {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			if parsedOrigin, err := url.Parse(origin); err == nil {
				host = parsedOrigin.Host
			}
		}
	}

	// Fallback to Host header if Origin is not available
	if host == "" {
		host = c.Request.Host
	}

	baseDomain := extractBaseDomain(host)

	for _, cookie := range resp.Cookies() {
		c.SetCookie(
			cookie.Name,
			cookie.Value,
			cookie.MaxAge,
			cookie.Path,
			baseDomain, // Set explicit domain for cross-subdomain support
			cookie.Secure,
			cookie.HttpOnly,
		)
	}

	// Handle different response codes
	switch resp.StatusCode {
	case http.StatusSeeOther, http.StatusFound, http.StatusMovedPermanently:
		// Kratos is redirecting to settings page
		// This means recovery was successful and user is now logged in
		redirectURL := resp.Header.Get("Location")

		// Extract the settings flow ID from the redirect URL
		// The redirect URL is like: http://localhost:4455/settings?flow=xxx
		parsedURL, err := url.Parse(redirectURL)
		if err != nil {
			logger.Err(err).Msg("Failed to parse redirect URL")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

		settingsFlowID := parsedURL.Query().Get("flow")
		if settingsFlowID == "" {
			logger.Error().Str("redirect_url", redirectURL).Msg("No flow ID in redirect URL")
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to extract settings flow ID",
			})
			return
		}

		// Return the settings flow ID to the frontend
		// The frontend will call Kratos directly to get the flow details
		// This avoids session cookie issues with backend proxying
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"message":          "Recovery successful, please set your password",
			"settings_flow_id": settingsFlowID,
		})

	case http.StatusBadRequest:
		// Invalid or expired flow/token
		body, _ := io.ReadAll(resp.Body)
		logger.Warn().
			Str("flow", flowID).
			Str("response", string(body)).
			Msg("Recovery failed - invalid or expired")

		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Recovery link is invalid or expired",
		})

	default:
		// Other error
		body, _ := io.ReadAll(resp.Body)
		logger.Error().
			Str("flow", flowID).
			Int("status", resp.StatusCode).
			Str("response", string(body)).
			Msg("Recovery failed with unexpected status")

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to process recovery link",
		})
	}
}
