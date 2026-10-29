package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// A JSON body built from a bare error value marshals to {}: the client was
// told "500" and nothing else. These responses must carry a message.

func errorBody(t *testing.T, handler gin.HandlerFunc) (int, map[string]string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/admin-api/v1/tenant/profile", nil)
	handler(c)
	body := map[string]string{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", w.Body.String(), err)
	}
	return w.Code, body
}

func TestATenantlessContextSaysWhyItFailed(t *testing.T) {
	code, body := errorBody(t, (&TenantHandler{}).UpdateTenantProfile)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d", code)
	}
	if body["message"] == "" {
		t.Fatalf("a failure was reported without a message: %q", body)
	}
}
