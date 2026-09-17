package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func ctxWithHeaders(t *testing.T, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

func TestExtractSessionCredential(t *testing.T) {
	tests := []struct {
		name      string
		headers   map[string]string
		wantValue string
		wantKind  CredentialKind
		wantOK    bool
	}{
		{
			name:      "native token arrives in X-Session-Token",
			headers:   map[string]string{"X-Session-Token": "native-tok"},
			wantValue: "native-tok",
			wantKind:  CredentialNativeToken,
			wantOK:    true,
		},
		{
			name:      "browser cookie arrives in the Cookie header",
			headers:   map[string]string{"Cookie": "ory_kratos_session=cookie-val"},
			wantValue: "cookie-val",
			wantKind:  CredentialCookie,
			wantOK:    true,
		},
		{
			name:      "bearer is a native token, not a cookie",
			headers:   map[string]string{"Authorization": "Bearer bearer-tok"},
			wantValue: "bearer-tok",
			wantKind:  CredentialNativeToken,
			wantOK:    true,
		},
		{
			name:    "no credential at all",
			headers: nil,
			wantOK:  false,
		},
		{
			// This precedence is the rule that produced hub#182: a browser
			// attaches its cookie to every request, so whatever sits in
			// X-Session-Token shadows a perfectly good cookie.
			name: "the header wins over the cookie",
			headers: map[string]string{
				"X-Session-Token": "native-tok",
				"Cookie":          "ory_kratos_session=cookie-val",
			},
			wantValue: "native-tok",
			wantKind:  CredentialNativeToken,
			wantOK:    true,
		},
		{
			name: "the cookie wins over bearer",
			headers: map[string]string{
				"Cookie":        "ory_kratos_session=cookie-val",
				"Authorization": "Bearer bearer-tok",
			},
			wantValue: "cookie-val",
			wantKind:  CredentialCookie,
			wantOK:    true,
		},
		{
			name:    "an empty bearer is not a credential",
			headers: map[string]string{"Authorization": "Bearer "},
			wantOK:  false,
		},
		{
			name:    "a non-bearer Authorization scheme is ignored",
			headers: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExtractSessionCredential(ctxWithHeaders(t, tc.headers))

			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (credential %+v)", ok, tc.wantOK, got)
			}
			if got.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", got.Value, tc.wantValue)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.wantKind)
			}
		})
	}
}
