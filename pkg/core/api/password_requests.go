package core

import (
	"fmt"
	"strings"

	"ctoup.com/coreapp/pkg/shared/emailservice"
	access "ctoup.com/coreapp/pkg/shared/service"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func getResetPasswordURL(c *gin.Context, subdomains ...string) (string, error) {
	var subdomain string
	if len(subdomains) > 0 {
		subdomain = subdomains[0]
	}

	host, err := access.GetHost(c)
	if err != nil {
		return "", err
	}
	// if no subdomain passed, return the full host which includes existing subdomain
	if subdomain == "" {
		url := fmt.Sprintf("%s://%s/signin?from=/", host.Scheme, host.Host)
		return url, nil
	}

	host.Host = host.Host[strings.Index(host.Host, ".")+1:]
	domain, err := access.GetBaseDomainWithPort(c)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s://%s.%s/signin?from=/", host.Scheme, subdomain, domain)

	return url, nil
}

func resetPasswordRequest(c *gin.Context, baseAuthClient access.BaseAuthClient, url, toEmail string) error {
	fromEmail := c.GetString("SYSTEM_EMAIL")
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

func sendWelcomeEmail(c *gin.Context, baseAuthClient access.BaseAuthClient, url, toEmail string) error {
	fromEmail := c.GetString("SYSTEM_EMAIL")
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
