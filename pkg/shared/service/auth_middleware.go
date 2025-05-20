package service

import (
	"net/http"
	"strings"

	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

const (
	// Header key for API tokens
	XApiKeyHeader = "X-Api-Key"
)

// AuthMiddleware combines both API token and Firebase authentication
type AuthMiddleware struct {
	firebaseAuth *FirebaseAuthMiddleware
	apiToken     *ClientApplicationService
}

// NewAuthMiddleware creates a new combined authentication middleware
func NewAuthMiddleware(
	firebaseAuth *FirebaseAuthMiddleware,
	apiToken *ClientApplicationService,
) *AuthMiddleware {
	return &AuthMiddleware{
		firebaseAuth: firebaseAuth,
		apiToken:     apiToken,
	}
}

// MiddlewareFunc implements OR authentication logic
func (am *AuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for public endpoints
		if strings.HasPrefix(c.Request.URL.Path, "/public") {
			c.Next()
			return
		}

		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/users") &&
			!strings.HasPrefix(c.Request.URL.Path, "/superadmin-api") {
			// Check X-Api-Key header first
			token := c.GetHeader(XApiKeyHeader)

			// If X-Api-Key is not present, try legacy token extraction
			if token != "" {
				tokenRow, err := am.apiToken.VerifyAPIToken(c, token)
				if err == nil {
					// API token is valid, store info and continue
					c.Set("api_token", tokenRow)
					c.Set("api_token_scopes", tokenRow.Scopes)
					// c.Set(AUTH_EMAIL,)
					//c.Set(AUTH_CLAIMS, idToken.Claims)
					c.Set(AUTH_USER_ID, tokenRow.CreatedBy)
					c.Next()
					return
				} else {
					// API token is invalid
					c.JSON(http.StatusForbidden, gin.H{
						"status":  http.StatusForbidden,
						"message": "Invalid API token",
					})
					c.Abort()
					return
				}
			} else {
				_, failed := am.firebaseAuth.verifyToken(c)
				if failed {
					// No API token and Firebase auth failed
					c.JSON(http.StatusUnauthorized, gin.H{
						"status":  http.StatusUnauthorized,
						"message": "Authentication required",
					})
					c.Abort()
					return
				}
			}
		} else {
			idToken, failed := am.firebaseAuth.verifyToken(c)
			if failed {
				abort(am.firebaseAuth, c)
				return
			} else {
				if strings.HasPrefix(c.Request.URL.Path, "/api/v1/users") &&
					util.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, c.Request.Method) {

					if idToken.Claims["SUPER_ADMIN"] == true || idToken.Claims["ADMIN"] == true || idToken.Claims[FIREBASE_CLAIM_EMAIL] == "jcantonio@alineo.com" {
						// OK
					} else {
						c.JSON(http.StatusForbidden, gin.H{
							"status":  http.StatusForbidden,
							"message": "Need to be an ADMIN to perform such operation",
						})
						c.Abort()
						return
					}
				}

				if strings.HasPrefix(c.Request.URL.Path, "/superadmin-api") &&
					util.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, c.Request.Method) {
					if idToken.Claims["SUPER_ADMIN"] == true || idToken.Claims[FIREBASE_CLAIM_EMAIL] == "jcantonio@alineo.com" {
						// OK
					} else {
						c.JSON(http.StatusForbidden, gin.H{
							"status":  http.StatusForbidden,
							"message": "Need to be a SUPER_ADMIN to perform such operation",
						})
						c.Abort()
						return
					}
				}
				c.Next()
			}
		}

	}
}
