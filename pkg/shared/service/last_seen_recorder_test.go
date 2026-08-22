package service

import (
	"testing"
	"time"
)

// The whole point of the recorder is that a busy user does not turn every
// request into an UPDATE. due() is what enforces that, so it is tested
// directly — Touch() only adds a goroutine and the DB call around it.
func TestLastSeenDueThrottlesWithinInterval(t *testing.T) {
	r := &LastSeenRecorder{interval: 5 * time.Minute, seenAt: map[string]time.Time{}}
	base := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	if !r.due("u1", base) {
		t.Fatal("first sighting must write")
	}
	for _, after := range []time.Duration{time.Second, time.Minute, 4 * time.Minute} {
		if r.due("u1", base.Add(after)) {
			t.Errorf("wrote again %v into a 5m window", after)
		}
	}
	if !r.due("u1", base.Add(5*time.Minute)) {
		t.Error("must write again once the window has elapsed")
	}
}

// Throttling is per user: one chatty user must not suppress everyone else.
func TestLastSeenDueIsPerUser(t *testing.T) {
	r := &LastSeenRecorder{interval: 5 * time.Minute, seenAt: map[string]time.Time{}}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	if !r.due("u1", now) || !r.due("u2", now) {
		t.Fatal("each user's first sighting must write")
	}
	if r.due("u1", now) || r.due("u2", now) {
		t.Error("second sighting in-window must not write")
	}
}

// A failed write is forgotten so the next request retries, rather than the
// user going unstamped for a whole interval.
func TestLastSeenForgetAllowsRetry(t *testing.T) {
	r := &LastSeenRecorder{interval: 5 * time.Minute, seenAt: map[string]time.Time{}}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	r.due("u1", now)
	r.forget("u1")
	if !r.due("u1", now) {
		t.Error("after a failed write the next request must retry")
	}
}

// The map is a throttle hint, not state worth preserving; it must be bounded
// so a long-lived process cannot grow it without limit.
func TestLastSeenMapIsBounded(t *testing.T) {
	r := &LastSeenRecorder{interval: time.Hour, seenAt: map[string]time.Time{}}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	for i := 0; i < lastSeenMaxEntries+10; i++ {
		r.due(string(rune(i%1000))+time.Duration(i).String(), now)
	}
	if len(r.seenAt) > lastSeenMaxEntries {
		t.Errorf("map grew to %d, past the %d cap", len(r.seenAt), lastSeenMaxEntries)
	}
}

// A zero recorder must be inert rather than panic — Touch runs on every
// authenticated request, including in setups that never wired a store.
func TestLastSeenTouchIsSafeWhenDisabled(t *testing.T) {
	var r *LastSeenRecorder
	r.Touch(t.Context(), "u1")                      // nil receiver
	NewLastSeenRecorder(nil).Touch(t.Context(), "u1") // no store
	NewLastSeenRecorder(nil).Touch(t.Context(), "")   // no user
}
