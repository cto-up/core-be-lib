//go:build testutils

package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
)

type signupProvider struct {
	auth.AuthProvider
	client auth.AuthClient
}

func (signupProvider) GetProviderName() string { return "kratos" }
func (p signupProvider) GetAuthClientForSubdomain(context.Context, string) (auth.AuthClient, error) {
	return p.client, nil
}

// signupMailer counts the mails IdentifyUser tries to send: both mails start by
// asking for a recovery link, and failing it stops before any SMTP call.
type signupMailer struct {
	auth.AuthClient
	mails int
}

func (m *signupMailer) PasswordResetLinkWithSettings(context.Context, string, *auth.ActionCodeSettings) (string, error) {
	m.mails++
	return "", errors.New("no mail in tests")
}

func (*signupMailer) GetUserActivity(context.Context, []string) (map[string]auth.UserActivity, error) {
	return map[string]auth.UserActivity{}, nil
}

type signupUsers struct {
	access.UserService
	createFailures int
	creates, adds  int
	existing       *core.User
}

func (u *signupUsers) GetUserByEmailGlobal(context.Context, string) (*core.User, error) {
	if u.existing == nil {
		return nil, errors.New("not found")
	}
	return u.existing, nil
}

func (u *signupUsers) CreateUser(_ context.Context, _ auth.AuthClient, _ string, req core.NewUser, _ *string) (repository.CoreUser, error) {
	u.creates++
	if u.createFailures > 0 {
		u.createFailures--
		return repository.CoreUser{}, errors.New("kratos unavailable")
	}
	return repository.CoreUser{}, nil
}

func (u *signupUsers) AddUserToTenant(context.Context, auth.AuthClient, string, string, []core.Role, string) error {
	u.adds++
	return nil
}

func signupTenant(t *testing.T, store interface {
	CreateTenant(context.Context, repository.CreateTenantParams) (repository.CoreTenant, error)
}, id string) {
	_, err := store.CreateTenant(context.Background(), repository.CreateTenantParams{
		UserID: "owner", TenantID: id, Name: id, Subdomain: id, AllowSignUp: true,
	})
	require.NoError(t, err)
}

func identify(uh *UserHandler, tenantID, email string) int {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/public-api/v1/auth/identify",
		strings.NewReader(`{"email":"`+email+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Origin", "http://"+tenantID+".ctoup.localhost:5173")
	c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
	uh.IdentifyUser(c)
	return w.Code
}

// hub#556. The throttle slot was claimed before the account was created, so
// the retry of a failed sign-up was answered 200 and did nothing.
func TestIdentifyUser_AFailedSignUpCanBeRetriedAtOnce(t *testing.T) {
	store := testutils.NewTestStore(t)
	signupTenant(t, store, "retrytenant")
	mailer := &signupMailer{}
	users := &signupUsers{createFailures: 1}
	uh := &UserHandler{store: store, authProvider: signupProvider{client: mailer}, userService: users}

	require.Equal(t, http.StatusInternalServerError, identify(uh, "retrytenant", "retry@example.com"))
	require.Equal(t, http.StatusOK, identify(uh, "retrytenant", "retry@example.com"))
	require.Equal(t, 2, users.creates, "the retry must create the account")
	require.Equal(t, 1, mailer.mails, "the retry must send the mail")
}

// hub#556. The throttle was keyed on the address alone, so signing up on a
// second tenant inside the cooldown never added the user to it.
func TestIdentifyUser_ASecondTenantIsNotSwallowedByTheFirstsCooldown(t *testing.T) {
	store := testutils.NewTestStore(t)
	signupTenant(t, store, "tenanta")
	signupTenant(t, store, "tenantb")
	mailer := &signupMailer{}
	users := &signupUsers{}
	uh := &UserHandler{store: store, authProvider: signupProvider{client: mailer}, userService: users}

	require.Equal(t, http.StatusOK, identify(uh, "tenanta", "both@example.com"))
	users.existing = &core.User{Id: "user-both", Email: "both@example.com"}
	require.Equal(t, http.StatusOK, identify(uh, "tenantb", "both@example.com"))
	require.Equal(t, 1, users.adds, "the user must be added to the second tenant")
	require.Equal(t, 2, mailer.mails, "each tenant sends its own mail")

	require.Equal(t, http.StatusOK, identify(uh, "tenantb", "both@example.com"))
	require.Equal(t, 2, mailer.mails, "a resubmission inside the cooldown is still throttled")
}
