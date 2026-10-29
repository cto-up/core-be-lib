package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ctoup.com/coreapp/pkg/core/db/repository"
)

func TestInvitationResponseCarriesTheEmailOutcome(t *testing.T) {
	failed, err := json.Marshal(invitationResponse{
		CoreUserTenantMembership: repository.CoreUserTenantMembership{UserID: "u1"},
		emailOutcome:             emailOutcomeOf(errors.New("send invitation email: smtp: 535 auth failed")),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"user_id":"u1"`, `"email_sent":false`, `"email_error":"send invitation email: smtp: 535 auth failed"`} {
		if !strings.Contains(string(failed), want) {
			t.Errorf("response %s lacks %s", failed, want)
		}
	}

	sent, _ := json.Marshal(invitationResponse{emailOutcome: emailOutcomeOf(nil)})
	if !strings.Contains(string(sent), `"email_sent":true`) || strings.Contains(string(sent), "email_error") {
		t.Errorf("a sent mail must report email_sent:true and no error, got %s", sent)
	}
}
