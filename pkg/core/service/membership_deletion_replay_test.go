//go:build testutils

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"ctoup.com/coreapp/api/openapi/core"
	sharedservice "ctoup.com/coreapp/pkg/shared/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type replayContributor struct {
	sharedservice.UserDataContributor
	name    string
	fail    error
	applied []string
}

func (c *replayContributor) Name() string { return c.name }
func (c *replayContributor) ApplyLeaveDecisions(_ context.Context, _, tenantID string, _ []core.LeaveDecision) error {
	c.applied = append(c.applied, tenantID)
	return c.fail
}

func storedDecisions(t *testing.T, tenants ...string) []byte {
	t.Helper()
	target := "heir"
	var out []core.TenantLeaveDecision
	for _, tenantID := range tenants {
		out = append(out, core.TenantLeaveDecision{
			TenantId: tenantID, Key: "lms.authoredPublished",
			Action: core.TenantLeaveDecisionAction(core.LeaveDecisionActionTransfer), TargetUserId: &target,
		})
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	return raw
}

// A failed transfer used to be logged and the deletion carried on into the
// module erase, which deletes the very courses the person asked to hand over.
func TestReplayDeletionDecisions_AFailedTransferHoldsTheDeletion(t *testing.T) {
	lms := &replayContributor{name: "lms", fail: errors.New("target member left the tenant")}

	err := replayDeletionDecisions(context.Background(), "u1", storedDecisions(t, "t1"), []sharedservice.UserDataContributor{lms})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "contributor lms, tenant t1")
	assert.Contains(t, err.Error(), "target member left the tenant")
}

func TestReplayDeletionDecisions_UnreadableDecisionsHoldTheDeletion(t *testing.T) {
	err := replayDeletionDecisions(context.Background(), "u1", []byte("{not json"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable")
}

func TestReplayDeletionDecisions_EveryTenantIsAttemptedAndSuccessPasses(t *testing.T) {
	lms := &replayContributor{name: "lms"}
	require.NoError(t, replayDeletionDecisions(context.Background(), "u1", storedDecisions(t, "t1", "t2"), []sharedservice.UserDataContributor{lms}))
	assert.ElementsMatch(t, []string{"t1", "t2"}, lms.applied)

	require.NoError(t, replayDeletionDecisions(context.Background(), "u1", nil, []sharedservice.UserDataContributor{lms}))
}
