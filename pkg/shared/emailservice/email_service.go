package emailservice

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"

	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// SMTPConfig holds the SMTP configuration details
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

var (
	smtpConfig *SMTPConfig
	once       sync.Once
)

// InitializeSMTPConfig initializes SMTP configuration and ensures it's only done once
func InitializeSMTPConfig() *SMTPConfig {
	once.Do(func() {
		smtpConfig = &SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     os.Getenv("SMTP_PORT"),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
		}
	})
	return smtpConfig
}

// EmailRequest struct handles email request data
type EmailRequest struct {
	From    string
	To      []string
	Subject string
	Body    string
}

func NewEmailRequest(from string, to []string, subject, body string) *EmailRequest {
	return &EmailRequest{
		From:    from,
		To:      to,
		Subject: subject,
		Body:    body,
	}
}

// SendEmail sends an email using the SMTP configuration
func (r *EmailRequest) SendEmail() error {
	smtpCfg := InitializeSMTPConfig()
	auth := smtp.PlainAuth("", smtpCfg.Username, smtpCfg.Password, smtpCfg.Host)

	//msg := []byte(subject + mime + "\n" + r.Body)

	msg := []byte("To: " + r.To[0] + "\r\n" +
		"From: " + r.From + "\r\n" +
		"Subject: " + r.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" + r.Body)

	addr := smtpCfg.Host + ":" + smtpCfg.Port
	if err := smtp.SendMail(addr, auth, r.From, r.To, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

// ParseTemplate parses an HTML template and replaces placeholders with actual data
func (r *EmailRequest) ParseTemplate(templateFileName string, data interface{}) error {
	t, err := template.ParseFiles(templateFileName)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	buf := new(bytes.Buffer)
	if err = t.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	r.Body = buf.String()
	return nil
}

// ParseTemplateWithDomain parses an HTML template using domain-aware hierarchical lookup
// It searches for templates in the following order:
// 1. templates/domain/subdomain/templateName
// 2. templates/domain/templateName
// 3. templates/templateName
func (r *EmailRequest) ParseTemplateWithDomain(ctx *gin.Context, templateName string, data interface{}) error {
	templatePath, err := GetTemplate(ctx, templateName)
	if err != nil {
		return fmt.Errorf("failed to find template: %w", err)
	}

	return r.ParseTemplate(templatePath, data)
}

// GetTemplate finds a template file using hierarchical lookup based on domain and subdomain
// It searches in the following order:
// 1. templates/domain/subdomain/templateName
// 2. templates/domain/templateName
// 3. templates/templateName
//
// For example, if origin is "human.alineo.com" and templateName is "email-verification.html":
// 1. templates/alineo.com/human/email-verification.html
// 2. templates/alineo.com/email-verification.html
// 3. templates/email-verification.html
func GetTemplate(ctx *gin.Context, templateName string) (string, error) {
	domainInfo, err := utils.GetDomainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get domain info: %w", err)
	}

	// Build the search paths in order of priority
	searchPaths := make([]string, 0, 3)

	// If we have both domain and subdomain, try domain/subdomain/template first
	if domainInfo.Domain != "" && domainInfo.Subdomain != "" {
		searchPaths = append(searchPaths, filepath.Join("templates", domainInfo.Domain, domainInfo.Subdomain, templateName))
	}

	// If we have domain, try domain/template
	if domainInfo.Domain != "" {
		searchPaths = append(searchPaths, filepath.Join("templates", domainInfo.Domain, templateName))
	}

	// Always try the base template as fallback
	searchPaths = append(searchPaths, filepath.Join("templates", templateName))

	// Search for the first existing template file
	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// If no template found, return error with all attempted paths
	return "", fmt.Errorf("template '%s' not found in any of the following locations: %s", templateName, strings.Join(searchPaths, ", "))
}
