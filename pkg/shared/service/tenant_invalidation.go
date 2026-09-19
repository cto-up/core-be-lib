package service

import (
	"encoding/json"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

// Cross-replica invalidation for the tenant cache (lms#13).
//
// The cache is dropped in the process that served the write. Production runs
// more than one replica, so the other kept serving the old tenant record —
// theme, allow_sign_up, status — for up to DefaultTenantCacheTTL. Sixty seconds
// is short enough to look like a browser cache and long enough to be reported
// as a ghost.
//
// The shape is provider-lib's, which has carried it for credentials for a
// while: a subject constant so consumers cannot drift on the string, a
// one-method publisher interface that *nats.Conn satisfies with no adapter, and
// an Apply entry point so the host's subscriber is two lines that know nothing
// about the cache.
//
// core-be-lib has no NATS dependency and does not gain one here. It says what
// needs doing; the host supplies the transport it already runs.
//
// Package-level rather than a field on MultitenantService, deliberately: the
// cache behind it is itself a package-level singleton (getTenantCache), the
// service is constructed inside this library rather than by the host, and a
// second instance holding a different publisher would be a bug with no upside.

// TenantInvalidationSubject is the NATS subject tenant invalidations travel on.
//
// Consume it as a plain fan-out subscription, NEVER a queue group: a queue
// group delivers to exactly one member, which is indistinguishable from the
// bug this exists to fix.
const TenantInvalidationSubject = "core.tenant.invalidated"

// TenantInvalidationEvent names the tenant whose cached record is stale.
type TenantInvalidationEvent struct {
	TenantID string `json:"tenant_id"`
}

// TenantInvalidationPublisher is the slice of *nats.Conn this package needs.
type TenantInvalidationPublisher interface {
	Publish(subject string, data []byte) error
}

var tenantInvalidationPublisher atomic.Pointer[TenantInvalidationPublisher]

// SetTenantInvalidationPublisher makes tenant writes announce themselves to the
// other replicas. Call once at boot; optional.
//
// Without it every process still clears its own cache on write and self-heals
// on DefaultTenantCacheTTL, which is what that TTL was always for — so a
// single-replica deployment, or one with no broker, behaves exactly as before.
func SetTenantInvalidationPublisher(p TenantInvalidationPublisher) {
	if p == nil {
		tenantInvalidationPublisher.Store(nil)
		return
	}
	tenantInvalidationPublisher.Store(&p)
}

// publishTenantInvalidation is best-effort by design. A broker that is down
// must never fail the write that already succeeded: the local cache is clear
// and every other replica still heals on the TTL. Failing here would turn a
// degraded broker into "you cannot rename a tenant".
func publishTenantInvalidation(tenantID string) {
	p := tenantInvalidationPublisher.Load()
	if p == nil || tenantID == "" {
		return
	}
	payload, err := json.Marshal(TenantInvalidationEvent{TenantID: tenantID})
	if err != nil {
		return
	}
	if err := (*p).Publish(TenantInvalidationSubject, payload); err != nil {
		log.Warn().Err(err).Str("tenant_id", tenantID).
			Msg("core: tenant invalidation not broadcast; other replicas will self-heal on the cache TTL")
	}
}

// ApplyTenantInvalidation applies an event received from another replica.
//
// It drops the entry LOCALLY and publishes nothing. That is the whole reason it
// exists rather than the host calling InvalidateTenant: InvalidateTenant is the
// write surface and broadcasts, so a subscriber calling it would re-publish
// what it just received and two replicas would bounce the message forever.
func ApplyTenantInvalidation(payload []byte) {
	var ev TenantInvalidationEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		log.Warn().Err(err).Msg("core: malformed tenant invalidation event")
		return
	}
	if ev.TenantID == "" {
		return
	}
	getTenantCache().invalidate(ev.TenantID)
}
