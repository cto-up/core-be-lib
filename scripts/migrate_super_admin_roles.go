package main

import (
	"context"
	"os"

	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

// This script migrates existing SUPER_ADMIN users to use the new global_roles structure
// Run this once after deploying the refactored code

func main() {
	adminURL := os.Getenv("KRATOS_ADMIN_URL")
	if adminURL == "" {
		adminURL = "http://localhost:4434"
	}

	adminCfg := ory.NewConfiguration()
	adminCfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
	adminClient := ory.NewAPIClient(adminCfg)

	ctx := context.Background()

	// List all identities
	identities, _, err := adminClient.IdentityAPI.ListIdentities(ctx).Execute()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to list identities")
	}

	log.Info().Int("count", len(identities)).Msg("Found identities")

	for _, identity := range identities {
		// Check if metadata_public exists and has global_roles
		metadataPublic, ok := identity.MetadataPublic.(map[string]interface{})
		if !ok || metadataPublic == nil {
			metadataPublic = make(map[string]interface{})
		}

		// Check if global_roles already exists
		if _, exists := metadataPublic["global_roles"]; exists {
			log.Debug().Str("identity_id", identity.Id).Msg("Identity already has global_roles, skipping")
			continue
		}

		// Check if this identity should have SUPER_ADMIN
		// Look for legacy boolean claims or check database
		needsUpdate := false
		globalRoles := []string{}

		// Check if there's a legacy SUPER_ADMIN claim (this won't exist in Kratos, but checking anyway)
		// In reality, you'd need to check your database to see which users should be SUPER_ADMIN

		// For now, we'll just log identities that need manual review
		log.Info().
			Str("identity_id", identity.Id).
			Str("email", getEmail(identity)).
			Msg("Identity needs manual review for SUPER_ADMIN role")

		// Uncomment below to automatically set SUPER_ADMIN for specific users
		// if getEmail(identity) == "admin@example.com" {
		// 	globalRoles = append(globalRoles, "SUPER_ADMIN")
		// 	needsUpdate = true
		// }

		if needsUpdate {
			metadataPublic["global_roles"] = globalRoles

			traits, ok := identity.Traits.(map[string]interface{})
			if !ok {
				traits = make(map[string]interface{})
			}

			state := ""
			if identity.State != nil {
				state = string(*identity.State)
			}

			updateBody := *ory.NewUpdateIdentityBody(identity.SchemaId, state, traits)
			updateBody.MetadataPublic = metadataPublic

			_, _, err := adminClient.IdentityAPI.UpdateIdentity(ctx, identity.Id).
				UpdateIdentityBody(updateBody).
				Execute()

			if err != nil {
				log.Error().Err(err).Str("identity_id", identity.Id).Msg("Failed to update identity")
			} else {
				log.Info().Str("identity_id", identity.Id).Strs("global_roles", globalRoles).Msg("Updated identity with global_roles")
			}
		}
	}

	log.Info().Msg("Migration complete")
}

func getEmail(identity ory.Identity) string {
	if traits, ok := identity.Traits.(map[string]interface{}); ok {
		if email, ok := traits["email"].(string); ok {
			return email
		}
	}
	return ""
}
