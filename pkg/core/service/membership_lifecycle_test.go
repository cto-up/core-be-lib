//go:build testutils

package service

import (
	"testing"

	"ctoup.com/coreapp/api/openapi/core"
	"github.com/stretchr/testify/assert"
)

// The decision gate is the one piece of leave logic with no database in it, and
// it is what stands between "leave" and a published course nobody owns.

func impact(key string, severity core.LeaveImpactSeverity) core.LeaveImpact {
	return core.LeaveImpact{Key: key, Severity: severity}
}

func decision(key string, action core.LeaveDecisionAction, target *string) core.LeaveDecision {
	return core.LeaveDecision{Key: key, Action: action, TargetUserId: target}
}

func TestHasOutstandingDecision(t *testing.T) {
	t.Run("no impacts at all leaves nothing to answer", func(t *testing.T) {
		assert.False(t, hasOutstandingDecision(nil, nil))
	})

	t.Run("informational impacts never block", func(t *testing.T) {
		impacts := []core.LeaveImpact{
			impact("lms.enrollments", core.Info),
			impact("lms.certificates", core.Info),
		}
		assert.False(t, hasOutstandingDecision(impacts, nil),
			"stating what someone loses must not require them to answer it")
	})

	t.Run("an unanswered decision blocks", func(t *testing.T) {
		impacts := []core.LeaveImpact{impact("lms.authoredPublishedCourses", core.Decision)}
		assert.True(t, hasOutstandingDecision(impacts, nil))
	})

	t.Run("an answered decision unblocks", func(t *testing.T) {
		impacts := []core.LeaveImpact{impact("lms.authoredPublishedCourses", core.Decision)}
		answers := []core.LeaveDecision{
			decision("lms.authoredPublishedCourses", core.LeaveDecisionActionUnpublish, nil),
		}
		assert.False(t, hasOutstandingDecision(impacts, answers))
	})

	t.Run("an answer to a different key does not count", func(t *testing.T) {
		impacts := []core.LeaveImpact{impact("lms.authoredPublishedCourses", core.Decision)}
		answers := []core.LeaveDecision{
			decision("lms.activeSubscription", core.LeaveDecisionActionKeep, nil),
		}
		assert.True(t, hasOutstandingDecision(impacts, answers),
			"answering the wrong question must not release the leave")
	})

	// The failure this guards against: a transfer that names no destination is
	// accepted, the membership ends, and the courses are owned by somebody who
	// is no longer in the workspace.
	t.Run("a transfer with no target is not an answer", func(t *testing.T) {
		impacts := []core.LeaveImpact{impact("lms.authoredPublishedCourses", core.Decision)}
		empty := ""
		for name, target := range map[string]*string{"nil": nil, "empty": &empty} {
			t.Run(name, func(t *testing.T) {
				answers := []core.LeaveDecision{
					decision("lms.authoredPublishedCourses", core.LeaveDecisionActionTransfer, target),
				}
				assert.True(t, hasOutstandingDecision(impacts, answers))
			})
		}
	})

	t.Run("a transfer with a target is an answer", func(t *testing.T) {
		impacts := []core.LeaveImpact{impact("lms.authoredPublishedCourses", core.Decision)}
		target := "user-42"
		answers := []core.LeaveDecision{
			decision("lms.authoredPublishedCourses", core.LeaveDecisionActionTransfer, &target),
		}
		assert.False(t, hasOutstandingDecision(impacts, answers))
	})

	t.Run("every decision must be answered, not just one", func(t *testing.T) {
		impacts := []core.LeaveImpact{
			impact("lms.authoredPublishedCourses", core.Decision),
			impact("lms.activeSubscription", core.Decision),
		}
		answers := []core.LeaveDecision{
			decision("lms.authoredPublishedCourses", core.LeaveDecisionActionKeep, nil),
		}
		assert.True(t, hasOutstandingDecision(impacts, answers))

		answers = append(answers,
			decision("lms.activeSubscription", core.LeaveDecisionActionCancelAtPeriodEnd, nil))
		assert.False(t, hasOutstandingDecision(impacts, answers))
	})
}

// The preview is read with no answers in hand, so canLeaveNow must be false the
// moment a decision exists — otherwise the dialog offers a confirm button that
// the server will refuse.
func TestPreviewBlocksWhileADecisionExists(t *testing.T) {
	impacts := []core.LeaveImpact{
		impact("lms.enrollments", core.Info),
		impact("lms.activeSubscription", core.Decision),
	}
	assert.True(t, hasOutstandingDecision(impacts, nil))
}
