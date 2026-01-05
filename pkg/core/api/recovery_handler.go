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
// Returns nil if provider doesn't require proxy (e.g., Firebase)
func NewRecoveryHandler(authProvider auth.AuthProvider) *RecoveryHandler {
	// Check if provider requires recovery proxy
	authClient := authProvider.GetAuthClient()
	if !authClient.RequiresRecoveryProxy() {
		log.Info().
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

	log.Info().
		Str("provider", authProvider.GetProviderName()).
		Str("kratos_url", kratosPublicURL).
		Bool("has_client", publicClient != nil).
		Msg("RecoveryHandler created for provider requiring proxy")

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
	flowID := params.Flow
	token := params.Token

	// Construct the Kratos recovery URL
	kratosURL := fmt.Sprintf("%s/self-service/recovery?flow=%s&token=%s", h.kratosPublicURL, flowID, token)

	log.Info().
		Str("flow", flowID).
		Str("kratos_url", kratosURL).
		Msg("Activating recovery link via Kratos")

	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Make request to Kratos to activate the recovery link
	req, err := http.NewRequest("GET", kratosURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Kratos request")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to call Kratos recovery endpoint")
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

	log.Info().
		Str("request_host", c.Request.Host).
		Str("referer", referer).
		Str("origin", c.Request.Header.Get("Origin")).
		Str("effective_host", host).
		Str("base_domain", baseDomain).
		Msg("Setting cookies with base domain")

	for _, cookie := range resp.Cookies() {
		log.Info().
			Str("cookie_name", cookie.Name).
			Str("cookie_path", cookie.Path).
			Int("cookie_max_age", cookie.MaxAge).
			Bool("http_only", cookie.HttpOnly).
			Bool("secure", cookie.Secure).
			Str("domain", baseDomain).
			Msg("Setting cookie from Kratos")

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

		log.Info().
			Str("flow", flowID).
			Str("redirect_url", redirectURL).
			Msg("Recovery link activated, user logged in")

		// Extract the settings flow ID from the redirect URL
		// The redirect URL is like: http://localhost:4455/settings?flow=xxx
		parsedURL, err := url.Parse(redirectURL)
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse redirect URL")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

		settingsFlowID := parsedURL.Query().Get("flow")
		if settingsFlowID == "" {
			log.Error().Str("redirect_url", redirectURL).Msg("No flow ID in redirect URL")
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to extract settings flow ID",
			})
			return
		}

		log.Info().
			Str("settings_flow_id", settingsFlowID).
			Msg("Extracted settings flow ID from redirect URL")

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
		log.Warn().
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
		log.Error().
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
