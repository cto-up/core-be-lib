package emailservice

import (
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A server that accepts the connection and then says nothing — what an
// implicit-TLS port looks like to a plaintext client. The send must fail on
// the deadline instead of blocking the goroutine until the socket dies.
func TestSendEmailTimesOutOnMuteServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn // held open, never written to
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)

	r := NewEmailRequest("from@example.com", []string{"to@example.com"}, "subject", "body")

	done := make(chan error, 1)
	go func() {
		done <- r.send(&SMTPConfig{Host: host, Port: port}, false, 200*time.Millisecond, 300*time.Millisecond)
	}()

	select {
	case err := <-done:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "greeting")
	case <-time.After(5 * time.Second):
		t.Fatal("send blocked past the conversation deadline")
	}

	select {
	case conn := <-accepted:
		conn.Close()
	default:
	}
}

// On the implicit-TLS path there is no greeting to wait for: the client
// speaks TLS first. A mute server must fail the handshake on the deadline,
// not block, and must say so.
func TestSendEmailImplicitTLSTimesOutOnMuteServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn // never completes the handshake
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)

	r := NewEmailRequest("from@example.com", []string{"to@example.com"}, "subject", "body")

	done := make(chan error, 1)
	go func() {
		done <- r.send(&SMTPConfig{Host: host, Port: port}, true, 200*time.Millisecond, 300*time.Millisecond)
	}()

	select {
	case err := <-done:
		assert.ErrorContains(t, err, "tls handshake")
	case <-time.After(5 * time.Second):
		t.Fatal("send blocked past the conversation deadline")
	}

	select {
	case conn := <-accepted:
		conn.Close()
	default:
	}
}

func TestUsesImplicitTLS(t *testing.T) {
	for port, want := range map[string]bool{
		"465":   true,
		" 465 ": true,
		"587":   false,
		"25":    false,
		"2525":  false,
		"":      false,
	} {
		assert.Equal(t, want, usesImplicitTLS(port), "port %q", port)
	}
}

func TestSendEmailRejectsHeaderInjection(t *testing.T) {
	r := NewEmailRequest("from@example.com\r\nBcc: attacker@example.com", []string{"to@example.com"}, "subject", "body")
	err := r.send(&SMTPConfig{Host: "127.0.0.1", Port: "1"}, false, time.Second, time.Second)
	assert.ErrorContains(t, err, "line break")
}

func TestGetTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Change to the root directory for the test
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd) // Restore original working directory after test

	// Go up three levels to reach the root directory
	os.Chdir("../../../")

	tests := []struct {
		name         string
		origin       string
		templateName string
		expectedPath string
		shouldError  bool
	}{
		{
			name:         "Human subdomain template exists",
			origin:       "https://human.alineo.com",
			templateName: "email-verification.html",
			expectedPath: "templates/alineo.com/human/email-verification.html",
			shouldError:  false,
		},
		{
			name:         "Domain template fallback",
			origin:       "https://api.alineo.com",
			templateName: "email-verification.html",
			expectedPath: "templates/alineo.com/email-verification.html",
			shouldError:  false,
		},
		{
			name:         "Base template fallback",
			origin:       "https://example.com",
			templateName: "email-verification.html",
			expectedPath: "templates/email-verification.html",
			shouldError:  false,
		},
		{
			name:         "Template not found",
			origin:       "https://example.com",
			templateName: "non-existent.html",
			expectedPath: "",
			shouldError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test request with the specified origin
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Origin", tt.origin)

			// Create a test context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Call GetTemplate
			result, err := GetTemplate(c, tt.templateName)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, result)
			}
		})
	}
}
