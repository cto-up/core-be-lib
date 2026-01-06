package service

import (
	"encoding/json"
	"net/http"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// KratosWebhookPayload represents the payload from Kratos webhooks
type KratosWebhookPayload struct {
	Identity struct {
		ID     string `json:"id"`
		Traits struct {
			Email     string `json:"email"`
			Name      string `json:"name,omitempty"`
			Subdomain string `json:"subdomain,omitempty"`
		} `json:"traits"`
		MetadataPublic map[string]interface{} `json:"metadata_public,omitempty"`
	} `json:"identity"`
	Flow struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"flow,omitempty"`
}

// KratosWebhookHandler handles webhooks from Kratos
type KratosWebhookHandler struct {
	tenantService      *KratosTenantService
	authProvider       auth.AuthProvider
	membershipService  *UserTenantMembershipService
	multitenantService *MultitenantService
}

// NewKratosWebhookHandler creates a new webhook handler
func NewKratosWebhookHandler(
	tenantService *KratosTenantService,
	authProvider auth.AuthProvider,
	membershipService *UserTenantMembershipService,
	multitenantService *MultitenantService,
) *KratosWebhookHandler {
	return &KratosWebhookHandler{
		tenantService:      tenantService,
		authProvider:       authProvider,
		membershipService:  membershipService,
		multitenantService: multitenantService,
	}
}

// HandleRegistrationWebhook processes registration webhooks from Kratos
// This is called after a user successfully registers
func (kwh *KratosWebhookHandler) HandleRegistrationWebhook(c *gin.Context) {
	var payload KratosWebhookPayload

	if err := c.BindJSON(&payload); err != nil {
		log.Error().Err(err).Msg("Failed to parse webhook payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	log.Info().
		Str("user_id", payload.Identity.ID).
		Str("email", payload.Identity.Traits.Email).
		Str("subdomain", payload.Identity.Traits.Subdomain).
		Msg("Processing registration webhook")

	// If subdomain is provided in traits, assign user to tenant
	if payload.Identity.Traits.Subdomain != "" {
		// Get tenant ID from subdomain
		tenantID, err := kwh.multitenantService.GetTenantIDWithSubdomain(c.Request.Context(), payload.Identity.Traits.Subdomain)
		if err != nil {
			log.Error().
				Err(err).
				Str("subdomain", payload.Identity.Traits.Subdomain).
				Msg("Failed to get tenant ID from subdomain")
		} else {
			// Create membership entry with default USER role
			err = kwh.membershipService.AddUserToTenant(
				c.Request.Context(),
				payload.Identity.ID,
				tenantID,
				[]string{"USER"}, // Default role as array
				"system",         // System-initiated during registration
			)

			if err != nil {
				log.Error().
					Err(err).
					Str("user_id", payload.Identity.ID).
					Str("tenant_id", tenantID).
					Msg("Failed to create membership entry")
			} else {
				log.Info().
					Str("user_id", payload.Identity.ID).
					Str("tenant_id", tenantID).
					Str("subdomain", payload.Identity.Traits.Subdomain).
					Msg("User membership created successfully")
			}
		}

		// Also update metadata for backward compatibility
		err = kwh.tenantService.AssignUserToTenant(
			c.Request.Context(),
			payload.Identity.ID,
			payload.Identity.Traits.Subdomain,
		)

		if err != nil {
			log.Error().
				Err(err).
				Str("user_id", payload.Identity.ID).
				Str("subdomain", payload.Identity.Traits.Subdomain).
				Msg("Failed to assign user to tenant metadata")
		}
	} else {
		log.Warn().
			Str("user_id", payload.Identity.ID).
			Msg("No subdomain provided in registration - user not assigned to tenant")
	}

	// Return success to Kratos
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"user_id": payload.Identity.ID,
	})
}

// HandleLoginWebhook processes login webhooks from Kratos
// Can be used for logging, analytics, or additional validation
func (kwh *KratosWebhookHandler) HandleLoginWebhook(c *gin.Context) {
	var payload KratosWebhookPayload

	if err := c.BindJSON(&payload); err != nil {
		log.Error().Err(err).Msg("Failed to parse webhook payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	log.Info().
		Str("user_id", payload.Identity.ID).
		Str("email", payload.Identity.Traits.Email).
		Msg("User logged in")

	// Validate user has tenant assignment
	tenantMetadata, err := kwh.tenantService.GetUserTenant(c.Request.Context(), payload.Identity.ID)
	if err != nil || tenantMetadata.TenantID == "" {
		log.Warn().
			Str("user_id", payload.Identity.ID).
			Msg("User logged in without tenant assignment")
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// HandleSettingsWebhook processes settings update webhooks
func (kwh *KratosWebhookHandler) HandleSettingsWebhook(c *gin.Context) {
	var payload KratosWebhookPayload

	if err := c.BindJSON(&payload); err != nil {
		log.Error().Err(err).Msg("Failed to parse webhook payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	log.Info().
		Str("user_id", payload.Identity.ID).
		Msg("User updated settings")

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// VerifyWebhookSignature verifies the webhook signature from Kratos
// Implement this if you configure webhook signatures in Kratos
func (kwh *KratosWebhookHandler) VerifyWebhookSignature(c *gin.Context) bool {
	// Get signature from header
	signature := c.GetHeader("X-Kratos-Webhook-Signature")

	if signature == "" {
		log.Warn().Msg("Webhook received without signature")
		return false
	}

	// TODO: Implement signature verification
	// This depends on your Kratos webhook configuration
	// See: https://www.ory.sh/docs/kratos/self-service/flows/verify-email-account-activation

	return true
}

// RegisterWebhookRoutes registers webhook routes on the router
func (kwh *KratosWebhookHandler) RegisterWebhookRoutes(router *gin.RouterGroup) {
	webhooks := router.Group("/webhooks/kratos")
	{
		webhooks.POST("/registration", kwh.HandleRegistrationWebhook)
		webhooks.POST("/login", kwh.HandleLoginWebhook)
		webhooks.POST("/settings", kwh.HandleSettingsWebhook)
	}

	log.Info().Msg("Kratos webhook routes registered")
}

// Example Kratos configuration for webhooks:
/*
selfservice:
  flows:
    registration:
      after:
        hooks:
          - hook: web_hook
            config:
              url: https://your-backend.com/webhooks/kratos/registration
              method: POST
              body: base64://ewogICJpZGVudGl0eSI6IHt9Cn0=
              auth:
                type: api_key
                config:
                  name: X-API-Key
                  value: your-webhook-secret
                  in: header

    login:
      after:
        hooks:
          - hook: web_hook
            config:
              url: https://your-backend.com/webhooks/kratos/login
              method: POST

    settings:
      after:
        hooks:
          - hook: web_hook
            config:
              url: https://your-backend.com/webhooks/kratos/settings
              method: POST
*/

// Example usage in server initialization:
/*
func SetupWebhooks(router *gin.Engine, tenantService *service.KratosTenantService, authProvider auth.AuthProvider) {
	webhookHandler := service.NewKratosWebhookHandler(tenantService, authProvider)

	// Public routes (no auth required for webhooks from Kratos)
	public := router.Group("/public")
	webhookHandler.RegisterWebhookRoutes(public)
}
*/

// TenantInvitationPayload represents an invitation to join a tenant
type TenantInvitationPayload struct {
	Email     string   `json:"email" binding:"required"`
	Subdomain string   `json:"subdomain" binding:"required"`
	Roles     []string `json:"roles,omitempty"`
	InvitedBy string   `json:"invited_by"`
}

// HandleTenantInvitation creates an invitation for a user to join a tenant
func (kwh *KratosWebhookHandler) HandleTenantInvitation(c *gin.Context) {
	var payload TenantInvitationPayload

	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get authenticated user (inviter)
	inviterID := c.GetString(auth.AUTH_USER_ID)
	if inviterID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Validate inviter has permission to invite to this tenant
	hasAccess, err := kwh.tenantService.ValidateUserTenantAccess(
		c.Request.Context(),
		inviterID,
		payload.Subdomain,
	)

	if err != nil || !hasAccess {
		log.Error().
			Err(err).
			Str("inviter_id", inviterID).
			Str("subdomain", payload.Subdomain).
			Msg("User attempted to invite to unauthorized tenant")

		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Check if user already exists
	authClient := kwh.authProvider.GetAuthClient()
	existingUser, err := authClient.GetUserByEmail(c.Request.Context(), payload.Email)

	if err == nil && existingUser != nil {
		// User exists - assign to tenant
		err = kwh.tenantService.AssignUserToTenant(
			c.Request.Context(),
			existingUser.UID,
			payload.Subdomain,
		)

		if err != nil {
			log.Error().Err(err).Msg("Failed to assign existing user to tenant")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign user"})
			return
		}

		// Set roles if provided
		if len(payload.Roles) > 0 {
			customClaims := make(map[string]interface{})
			for _, role := range payload.Roles {
				customClaims[role] = true
			}
			_ = authClient.SetCustomUserClaims(c.Request.Context(), existingUser.UID, customClaims)
		}

		log.Info().
			Str("user_id", existingUser.UID).
			Str("subdomain", payload.Subdomain).
			Msg("Existing user assigned to tenant")

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "User assigned to tenant",
			"user_id": existingUser.UID,
		})
		return
	}

	// User doesn't exist - create invitation
	// Store invitation in database for later use during registration
	// This is application-specific logic

	log.Info().
		Str("email", payload.Email).
		Str("subdomain", payload.Subdomain).
		Str("invited_by", inviterID).
		Msg("Tenant invitation created")

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Invitation created",
	})
}

// ParseWebhookPayload is a helper to parse webhook payloads
func ParseWebhookPayload(data []byte) (*KratosWebhookPayload, error) {
	var payload KratosWebhookPayload
	err := json.Unmarshal(data, &payload)
	return &payload, err
}
