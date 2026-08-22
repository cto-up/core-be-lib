package service

import (
	"context"
	"sync"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/util"
)

const (
	// How stale core_users.last_seen_at is allowed to get while someone is
	// actively using the app. The column answers "roughly when was this person
	// last here", which a users list renders as "2 hours ago" — minutes of lag
	// are invisible there, and the interval is what keeps a busy tenant from
	// turning every request into a write.
	lastSeenInterval = 5 * time.Minute

	// Entries are only a write-throttle hint, so the map can be dropped whole
	// rather than swept. Rebuilding it costs one extra UPDATE per active user.
	lastSeenMaxEntries = 50_000
)

// LastSeenRecorder stamps core_users.last_seen_at as people use the app,
// throttled so a burst of requests from one user causes one write.
//
// This exists because the auth provider cannot answer it. Kratos records
// authenticated_at — the last time credentials were entered — and nothing that
// moves when a page is loaded; with a 30-day session lifespan a daily user
// still reads as having signed in weeks ago. Last activity has to be observed
// on the request path or not at all.
type LastSeenRecorder struct {
	store    *db.Store
	interval time.Duration

	mu     sync.Mutex
	seenAt map[string]time.Time
}

func NewLastSeenRecorder(store *db.Store) *LastSeenRecorder {
	return &LastSeenRecorder{
		store:    store,
		interval: lastSeenInterval,
		seenAt:   make(map[string]time.Time),
	}
}

// Touch records that userID is active now. It returns immediately: the write,
// when one is due, happens on its own goroutine with its own context, because
// this runs on every authenticated request and must never add latency to it —
// nor fail it. A dropped stamp is a cosmetic loss.
func (r *LastSeenRecorder) Touch(ctx context.Context, userID string) {
	if r == nil || r.store == nil || userID == "" {
		return
	}
	if !r.due(userID, time.Now()) {
		return
	}

	logger := util.GetLoggerFromCtx(ctx)
	go func() {
		// Deliberately not the request context — the client disconnecting must
		// not cancel a write we have already decided to make.
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := r.store.TouchSharedUserLastSeen(writeCtx, userID); err != nil {
			// Let the next request past the interval try again.
			r.forget(userID)
			logger.Debug().Err(err).Str("user_id", userID).
				Msg("Failed to stamp last_seen_at")
		}
	}()
}

// due reports whether userID is out of its throttle window, and claims the
// window if so, so concurrent requests from the same user produce one write.
func (r *LastSeenRecorder) due(userID string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if last, ok := r.seenAt[userID]; ok && now.Sub(last) < r.interval {
		return false
	}
	if len(r.seenAt) >= lastSeenMaxEntries {
		r.seenAt = make(map[string]time.Time)
	}
	r.seenAt[userID] = now
	return true
}

func (r *LastSeenRecorder) forget(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.seenAt, userID)
}
