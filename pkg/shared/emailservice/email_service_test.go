package emailservice

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

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
