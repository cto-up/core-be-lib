package emailservice

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"sync"
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
