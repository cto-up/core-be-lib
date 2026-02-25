package core

import (
	"fmt"
	"os"
	"strings"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/emailservice"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	utils "ctoup.com/coreapp/pkg/shared/util"
)

// buildTenantURL constructs a URL for a specific tenant subdomain with proper port handling
// If subdomain is empty, uses the current subdomain from the request
func buildTenantURL(c *gin.Context, path string, subdomain string) (string, error) {
	host, err := utils.GetHost(c)
	if err != nil {
		return "", err
	}

	// If no subdomain passed, return the full host which includes existing subdomain
	if subdomain == "" {
		url := fmt.Sprintf("%s://%s%s", host.Scheme, host.Host, path)
		return url, nil
	}

	// Get base domain with port
	domain, err := utils.GetBaseDomainWithPort(c)
	if err != nil {
		return "", err
	}

	// Build URL with subdomain and domain (which includes port if present)
	url := fmt.Sprintf("%s://%s.%s%s", host.Scheme, subdomain, domain, path)
	return url, nil
}

// isAdminParticuleRequest returns true when the request originates from the backoffice
// (served under /admin/) and the FRONTEND_USE_ADMIN_PARTICULE env var is enabled.
func isAdminParticuleRequest(c *gin.Context) bool {
	if os.Getenv("FRONTEND_USE_ADMIN_PARTICULE") != "true" {
		return false
	}
	referer := c.GetHeader("Referer")
	origin := c.GetHeader("Origin")
	return strings.Contains(referer, "/admin/") || strings.Contains(origin, "/admin/")
}

func getResetPasswordURL(c *gin.Context, subdomains ...string) (string, error) {
	var subdomain string
	if len(subdomains) > 0 {
		subdomain = subdomains[0]
	}

	path := "/signin?from=/"
	if isAdminParticuleRequest(c) {
		path = "/admin/signin?from=/"
	}

	return buildTenantURL(c, path, subdomain)
}

func resetPasswordRequest(c *gin.Context, baseAuthClient auth.AuthClient, url, toEmail string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	actionCodeSettings := &auth.ActionCodeSettings{
		URL: url,
	}

	link, err := baseAuthClient.PasswordResetLinkWithSettings(c, toEmail, actionCodeSettings)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate reset link")
		if strings.HasPrefix(err.Error(), auth.ErrorCodeUserNotFound) {
			log.Warn().Str("email", toEmail).Msg("Password reset requested for non-existent user")
			return nil // Don't return an error to avoid revealing user existence
		}
		return err
	}

	// Log the generated link to verify it's not empty
	if link == "" {
		log.Error().Str("email", toEmail).Str("url", url).Msg("Returned empty password reset link")
		return fmt.Errorf("Returned empty password reset link")
	}
	linkPrefix := link
	if len(link) > 10 {
		linkPrefix = link[:10]
	}
	log.Info().Str("link_prefix", linkPrefix).Int("link_length", len(link)).Str("email", toEmail).Msg("Successfully generated password reset link")

	// Send the link via email (implement your email sending logic here)
	templateData := struct {
		Link string
	}{
		Link: link,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Reset Password Link", "")
	if err := r.ParseTemplateWithDomain(c, "email-reset.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for reset link")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send reset link")
		return err
	}
	log.Info().Str("email", toEmail).Msg("Password reset email sent successfully")
	return nil
}

func sendWelcomeEmail(c *gin.Context, baseAuthClient auth.AuthClient, url, toEmail string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	actionCodeSettings := &auth.ActionCodeSettings{
		URL: url,
	}

	link, err := baseAuthClient.PasswordResetLinkWithSettings(c, toEmail, actionCodeSettings)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate reset link")
		return err
	}

	// Send the link via email (implement your email sending logic here)
	templateData := struct {
		Link string
	}{
		Link: link,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Welcome, Set Your Password", "")
	if err := r.ParseTemplateWithDomain(c, "email-welcome.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for reset link")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send reset link")
		return err
	}
	log.Info().Str("email", toEmail).Msg("Welcome email sent successfully")
	return nil

}

func getConfirmationEmailURL(c *gin.Context) (string, error) {
	domainInfo, err := utils.GetDomainInfo(c)
	if err != nil {
		return "", err
	}
	baseURL := domainInfo.BaseURL

	url := fmt.Sprintf("%s/verify-email", baseURL)

	return url, nil
}

func sendConfirmationEmail(c *gin.Context, url, toEmail string, confirmationToken string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	/** Option 1: Generate the email verification link via firebase
	actionCodeSettings := &auth.ActionCodeSettings{
		URL: url,
	}

	link, err := baseAuthClient.EmailVerificationLinkWithSettings(c, toEmail, actionCodeSettings)

	if err != nil {
		log.Error().Err(err).Msg("Failed to generate email verification link")
		return err
	}*/

	// Option 2: Generate the email verification link manually
	link := fmt.Sprintf("%s?token=%s", url, confirmationToken)

	// Send the link via email
	templateData := struct {
		Link  string
		Email string
	}{
		Link:  link,
		Email: toEmail,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Please verify your email address", "")
	if err := r.ParseTemplateWithDomain(c, "email-verification.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for email verification")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send email verification")
		return err
	}
	return nil
}

func sendTenantAddedEmail(c *gin.Context, baseAuthClient auth.AuthClient, url, toEmail, tenantName string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	// Send the notification email
	templateData := struct {
		Link       string
		TenantName string
	}{
		Link:       url,
		TenantName: tenantName,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "You've been added to "+tenantName, "")
	if err := r.ParseTemplateWithDomain(c, "email-tenant-added.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for tenant added notification")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send tenant added notification")
		return err
	}
	log.Info().Str("email", toEmail).Str("tenant", tenantName).Msg("Tenant added notification sent successfully")
	return nil
}

func sendMagicLink(c *gin.Context, baseAuthClient auth.AuthClient, origin, toEmail string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	actionCodeSettings := &auth.ActionCodeSettings{
		URL: origin + "/signin", // Redirect to signin after using the link
	}

	link, err := baseAuthClient.PasswordResetLinkWithSettings(c, toEmail, actionCodeSettings)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate magic link (recovery link)")
		return err
	}

	templateData := struct {
		Link string
	}{
		Link: link,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Welcome! Access Your Account", "")
	if err := r.ParseTemplateWithDomain(c, "email-magic-link.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for magic link")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send magic link email")
		return err
	}
	log.Info().Str("email", toEmail).Msg("Magic link email sent successfully")
	return nil
}

func sendSigninEmail(c *gin.Context, origin, toEmail string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	signinURL := origin + "/signin"

	templateData := struct {
		Link string
	}{
		Link: signinURL,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Sign in to your account", "")
	if err := r.ParseTemplateWithDomain(c, "email-signin.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for signin email")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send signin email")
		return err
	}
	log.Info().Str("email", toEmail).Msg("Signin email sent successfully")
	return nil
}
