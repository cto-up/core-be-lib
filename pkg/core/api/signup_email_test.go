package core

import (
	"errors"
	"testing"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/stretchr/testify/assert"
)

func TestSignupEmailFor(t *testing.T) {
	const uid = "0b6f1c7e-5d0f-4a8e-9b2e-3c1d2e4f5a6b"

	t.Run("an address that never used its access link gets the access link again", func(t *testing.T) {
		activity := map[string]auth.UserActivity{uid: {Found: true, State: "active", EmailVerified: false}}
		assert.Equal(t, signupEmailAccess, signupEmailFor(uid, activity, nil))
	})

	t.Run("an activated account is told it already exists", func(t *testing.T) {
		activity := map[string]auth.UserActivity{uid: {Found: true, State: "active", EmailVerified: true}}
		assert.Equal(t, signupEmailAlreadyRegistered, signupEmailFor(uid, activity, nil))
	})

	t.Run("a provider error still sends a link that works", func(t *testing.T) {
		assert.Equal(t, signupEmailAccess, signupEmailFor(uid, nil, errors.New("kratos down")))
	})

	t.Run("an identity the provider does not hold gets the access link", func(t *testing.T) {
		activity := map[string]auth.UserActivity{uid: {State: auth.UserActivityStateMissing}}
		assert.Equal(t, signupEmailAccess, signupEmailFor(uid, activity, nil))
		assert.Equal(t, signupEmailAccess, signupEmailFor(uid, map[string]auth.UserActivity{}, nil))
	})
}
