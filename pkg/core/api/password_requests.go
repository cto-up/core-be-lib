package core

import (
	"net/http"

	"ctoup.com/coreapp/pkg/shared/emailservice"
	access "ctoup.com/coreapp/pkg/shared/service"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func resetPasswordRequest(c *gin.Context, baseAuthClient access.BaseAuthClient, url, toEmail string) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate reset link"})
		return
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse  template for reset link"})
		return
	}

	if err := r.SendEmail(); err != nil {
		log.Error().Err(err).Msg("Failed to send reset link")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send reset link"})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
	}

}
