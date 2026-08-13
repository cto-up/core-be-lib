package emailservice

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// Bounds on the SMTP exchange. net/smtp sets none of its own: it dials
// without a timeout and never puts a deadline on the connection, so a
// server that accepts the TCP connection and then says nothing — an
// implicit-TLS port reached by a plaintext client, for instance — blocks
// the caller until the kernel gives up on the socket, hours later. Callers
// that send from a fire-and-forget goroutine (approval notifications, the
// digest sweeper) see that as a silent goroutine leak rather than a failure.
const (
	smtpDialTimeout         = 10 * time.Second
	smtpConversationTimeout = 30 * time.Second
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
	cfg := InitializeSMTPConfig()
	return r.send(cfg, usesImplicitTLS(cfg.Port), smtpDialTimeout, smtpConversationTimeout)
}

// usesImplicitTLS reports whether the port speaks TLS from the first byte
// (SMTPS) rather than negotiating it with STARTTLS. 465 is the registered
// SMTPS port and the convention every other client keys off; a plaintext
// client pointed at it waits for a greeting that will never come.
func usesImplicitTLS(port string) bool {
	return strings.TrimSpace(port) == "465"
}

// send is smtp.SendMail with deadlines: dialTimeout caps the connect, and
// conversationTimeout caps everything after it — TLS handshake, greeting,
// STARTTLS, auth and the message body — as a single deadline on the
// connection.
func (r *EmailRequest) send(cfg *SMTPConfig, implicitTLS bool, dialTimeout, conversationTimeout time.Duration) error {
	if len(r.To) == 0 {
		return errors.New("failed to send email: no recipient")
	}
	for _, addr := range append([]string{r.From}, r.To...) {
		if strings.ContainsAny(addr, "\r\n") {
			return fmt.Errorf("failed to send email: address contains a line break: %q", addr)
		}
	}

	msg := []byte("To: " + r.To[0] + "\r\n" +
		"From: " + r.From + "\r\n" +
		"Subject: " + r.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" + r.Body)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	rawConn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("failed to send email: dial %s: %w", addr, err)
	}
	defer rawConn.Close()

	// Set before the handshake and before NewClient reads the greeting —
	// those reads are where a mute server parks us. It covers the TLS
	// connection too, which writes through to this one.
	if err := rawConn.SetDeadline(time.Now().Add(conversationTimeout)); err != nil {
		return fmt.Errorf("failed to send email: set deadline: %w", err)
	}

	conn := rawConn
	if implicitTLS {
		tlsConn := tls.Client(rawConn, &tls.Config{ServerName: cfg.Host})
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("failed to send email: tls handshake with %s: %w", addr, err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("failed to send email: greeting from %s: %w", addr, err)
	}
	defer client.Close()

	// Already encrypted on 465; a STARTTLS advertised inside that session
	// would mean handshaking a second time inside the first.
	if ok, _ := client.Extension("STARTTLS"); ok && !implicitTLS {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return fmt.Errorf("failed to send email: starttls: %w", err)
		}
	}

	// Same rule as smtp.SendMail: credentials configured but unusable is a
	// misconfiguration, not something to paper over.
	if ok, _ := client.Extension("AUTH"); !ok {
		return fmt.Errorf("failed to send email: %s does not support AUTH", addr)
	}
	if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
		return fmt.Errorf("failed to send email: auth: %w", err)
	}

	if err := client.Mail(r.From); err != nil {
		return fmt.Errorf("failed to send email: MAIL FROM %s: %w", r.From, err)
	}
	for _, rcpt := range r.To {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("failed to send email: RCPT TO %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to send email: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to send email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to send email: close body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to send email: quit: %w", err)
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
