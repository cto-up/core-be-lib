package service

import (
	"context"
	"net/http"
	"os"
	"strings"

	"ctoup.com/coreapp/pkg/shared/util"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"google.golang.org/api/option"
)

const valName = "FIREBASE_ID_TOKEN"
const FIREBASE_CLAIM_EMAIL = "email"
const FIREBASE_CLAIM_EMAIL_VERIFIED = "email_verified"
const FIREBASE_CLAIM_USER_ID = "user_id"

const AUTH_EMAIL = "auth_email"
const AUTH_USER_ID = "auth_user_id"
const AUTH_CLAIMS = "auth_claims"

// FirebaseAuthMiddleware is middleware for Firebase Authentication
type FirebaseAuthMiddleware struct {
	tenantClientConnectionPool *FirebaseTenantClientConnectionPool
	unAuthorized               func(c *gin.Context)
	multitenantService         *MultitenantService
}

func (f *FirebaseAuthMiddleware) GetTenantClientConnectionPool(c *gin.Context) *FirebaseTenantClientConnectionPool {
	return f.tenantClientConnectionPool
}

// New is constructor of the middleware
func newFirebaseClient(ctx context.Context) (*auth.Client, error) {
	fcfg := os.Getenv("FIREBASE_CONFIG")

	if fcfg == "" || fcfg == "default" {
		log.Fatal().Msg("missing FIREBASE_CONFIG environment variable or firebase-config secret in vault")
	}

	opt := option.WithCredentialsJSON([]byte(fcfg))
	app, err := firebase.NewApp(context.Background(), nil, opt)

	if err != nil {
		return nil, err
	}
	return app.Auth(ctx)
}

// New is constructor of the middleware
func NewFirebaseMiddleware(unAuthorized func(c *gin.Context), pool *FirebaseTenantClientConnectionPool, multitenantService *MultitenantService) *FirebaseAuthMiddleware {
	return &FirebaseAuthMiddleware{
		tenantClientConnectionPool: pool,
		unAuthorized:               unAuthorized,
		multitenantService:         multitenantService,
	}
}

func (fam *FirebaseAuthMiddleware) verifyToken(c *gin.Context) (*auth.Token, bool) {
	// TODO: Choose either Authorization or Token header
	// idToken.Claims[FIREBASE_CLAIM_EMAIL_VERIFIED] email_verified:false
	authHeader := c.Request.Header.Get("Authorization")
	token := strings.Replace(authHeader, "Bearer ", "", 1)

	if token == "" {
		token = c.Request.Header.Get("Token")
	}

	if token == "" {
		log.Error().Msg("missing token")
		return nil, true
	}

	var idToken *auth.Token

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("failed to get subdomain")
		return nil, true
	}

	authClient, err := fam.tenantClientConnectionPool.GetBaseAuthClient(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("failed to get auth client")
		return nil, true
	}

	idToken, err = authClient.VerifyIDToken(context.Background(), token)
	if err != nil {
		log.Error().Err(err).Msg("failed to verify id token")
		return nil, true
	}

	c.Set(AUTH_EMAIL, idToken.Claims[FIREBASE_CLAIM_EMAIL])
	userID := idToken.Claims[FIREBASE_CLAIM_USER_ID]
	c.Set(AUTH_USER_ID, userID)
	c.Set(AUTH_CLAIMS, idToken.Claims)
	return idToken, false
}

func abort(fam *FirebaseAuthMiddleware, c *gin.Context) {
	if fam.unAuthorized != nil {
		fam.unAuthorized(c)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  http.StatusUnauthorized,
			"message": http.StatusText(http.StatusUnauthorized),
		})
	}
	c.Abort()
}

// ExtractClaims extracts claims
func ExtractClaims(c *gin.Context) *auth.Token {
	idToken, ok := c.Get(valName)
	if !ok {
		return new(auth.Token)
	}
	return idToken.(*auth.Token)
}

func GetCustomClaims(c *gin.Context) []string {
	claims, exists := c.Get(AUTH_CLAIMS)
	if !exists {
		return []string{}
	}
	customClaims := util.FilterMapToArray(claims.(map[string]interface{}), util.UppercaseOnly)
	return customClaims
}
