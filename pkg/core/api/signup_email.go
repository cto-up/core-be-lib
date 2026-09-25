package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/jackc/pgx/v5"
)

// The sign-up page offers "Resend" after the same cooldown, so the two must move together.
const (
	signupEmailCooldownSeconds = 60
	signupEmailWindowSeconds   = 3600
	signupEmailMaxPerWindow    = 5
)

type signupEmailClaimer interface {
	ClaimSignupEmailSend(ctx context.Context, arg repository.ClaimSignupEmailSendParams) (string, error)
}

// claimSignupEmailSend reports whether a sign-up email may go to this address now.
func claimSignupEmailSend(ctx context.Context, q signupEmailClaimer, email string) (bool, error) {
	sum := sha256.Sum256([]byte(email))
	_, err := q.ClaimSignupEmailSend(ctx, repository.ClaimSignupEmailSendParams{
		EmailHash:       hex.EncodeToString(sum[:]),
		WindowSeconds:   signupEmailWindowSeconds,
		CooldownSeconds: signupEmailCooldownSeconds,
		MaxPerWindow:    signupEmailMaxPerWindow,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type signupEmailKind int

const (
	// signupEmailAccess carries a one-click access link: the first sign-up, or a
	// repeat by someone who never used that link.
	signupEmailAccess signupEmailKind = iota
	// signupEmailAlreadyRegistered tells an activated account holder they already
	// have an account, with sign-in and password-reset links.
	signupEmailAlreadyRegistered
)

// signupEmailFor picks the email for a known identity. Using the access link
// verifies the address, so an unverified address means the first email never
// got used. When the provider cannot say, the access email is the safe answer:
// its link works for every account.
func signupEmailFor(uid string, activity map[string]auth.UserActivity, activityErr error) signupEmailKind {
	if activityErr != nil {
		return signupEmailAccess
	}
	a, ok := activity[uid]
	if !ok || !a.Found || !a.EmailVerified {
		return signupEmailAccess
	}
	return signupEmailAlreadyRegistered
}
