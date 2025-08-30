package core

import (
	"fmt"
	"os"
	"strings"

	"ctoup.com/coreapp/pkg/shared/emailservice"
	"ctoup.com/coreapp/pkg/shared/service"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func getResetPasswordURL(c *gin.Context, subdomains ...string) (string, error) {
	var subdomain string
	if len(subdomains) > 0 {
		subdomain = subdomains[0]
	}

	host, err := service.GetHost(c)
	if err != nil {
		return "", err
	}
	// if no subdomain passed, return the full host which includes existing subdomain
	if subdomain == "" {
		url := fmt.Sprintf("%s://%s/signin?from=/", host.Scheme, host.Host)
		return url, nil
	}

	host.Host = host.Host[strings.Index(host.Host, ".")+1:]
	domain, err := service.GetBaseDomainWithPort(c)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s://%s.%s/signin?from=/", host.Scheme, subdomain, domain)

	return url, nil
}

func resetPasswordRequest(c *gin.Context, baseAuthClient service.BaseAuthClient, url, toEmail string) error {
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

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Reset Password Link", "")
	if err := r.ParseTemplate("templates/email-reset.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for reset link")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send reset link")
		return err
	}
	return nil
}

func sendWelcomeEmail(c *gin.Context, baseAuthClient service.BaseAuthClient, url, toEmail string) error {
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
	if err := r.ParseTemplate("templates/email-welcome.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for reset link")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send reset link")
		return err
	}
	return nil

}

func getConfirmationEmailURL(c *gin.Context) (string, error) {
	domainInfo, err := service.GetDomainInfo(c)
	if err != nil {
		return "", err
	}
	baseURL := domainInfo.BaseURL

	url := fmt.Sprintf("%s/verify-email", baseURL)

	return url, nil
}

func sendConfirmationEmail(c *gin.Context, baseAuthClient service.BaseAuthClient, url, toEmail string) error {
	fromEmail := os.Getenv("SYSTEM_EMAIL")
	if fromEmail == "" {
		fromEmail = "noreply@ctoup.com"
	}

	actionCodeSettings := &auth.ActionCodeSettings{
		URL: url,
	}

	link, err := baseAuthClient.EmailVerificationLinkWithSettings(c, toEmail, actionCodeSettings)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate email verification link")
		return err
	}

	// Send the link via email
	templateData := struct {
		Link  string
		Email string
	}{
		Link:  link,
		Email: toEmail,
	}

	r := emailservice.NewEmailRequest(fromEmail, []string{toEmail}, "Please verify your email address", "")
	if err := r.ParseTemplate("templates/email-verification.html", templateData); err != nil {
		log.Error().Err(err).Msg("Failed to parse template for email verification")
		return err
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send email verification")
		return err
	}
	return nil
}
