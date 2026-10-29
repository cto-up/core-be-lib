package core

import (
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
)

// emailOutcome rides beside the result of a write that also sends a mail. The
// write stands whether or not the mail went out, so a failed mail does not fail
// the request — but the caller must hear that nobody was told, and why.
type emailOutcome struct {
	EmailSent  bool   `json:"email_sent"`
	EmailError string `json:"email_error,omitempty"`
}

func emailOutcomeOf(err error) emailOutcome {
	if err != nil {
		return emailOutcome{EmailError: err.Error()}
	}
	return emailOutcome{EmailSent: true}
}

type userWithEmail struct {
	core.User
	emailOutcome
}

type signupResponse struct {
	repository.CoreUser
	emailOutcome
}
