package service

import (
	"encoding/json"
	"errors"
	"testing"

	"ctoup.com/coreapp/pkg/core/db/repository"
)

type capturingTenantPublisher struct {
	subject string
	events  []TenantInvalidationEvent
	err     error
}

func (p *capturingTenantPublisher) Publish(subject string, data []byte) error {
	if p.err != nil {
		return p.err
	}
	p.subject = subject
	var ev TenantInvalidationEvent
	_ = json.Unmarshal(data, &ev)
	p.events = append(p.events, ev)
	return nil
}

func cached(tenantID string) bool {
	c := getTenantCache()
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.entries[tenantID]
	return ok
}

// The bug: a replica that never saw the write keeps serving the old tenant
// record — theme, allow_sign_up, status — until the 60s TTL expires.
func TestTenantInvalidationFromAnotherReplicaDropsTheEntry(t *testing.T) {
	t.Cleanup(func() { SetTenantInvalidationPublisher(nil) })
	getTenantCache().put(repository.CoreTenant{TenantID: "tenant-1"})
	if !cached("tenant-1") {
		t.Fatal("precondition: entry should be warm")
	}

	payload, _ := json.Marshal(TenantInvalidationEvent{TenantID: "tenant-1"})
	ApplyTenantInvalidation(payload)

	if cached("tenant-1") {
		t.Fatal("the entry survived an invalidation from another replica")
	}
}

// Applying is LOCAL ONLY. If it published, the receiving replica would
// re-broadcast what it just received and the two would bounce it forever.
func TestApplyingATenantInvalidationPublishesNothing(t *testing.T) {
	t.Cleanup(func() { SetTenantInvalidationPublisher(nil) })
	pub := &capturingTenantPublisher{}
	SetTenantInvalidationPublisher(pub)

	getTenantCache().put(repository.CoreTenant{TenantID: "tenant-2"})
	payload, _ := json.Marshal(TenantInvalidationEvent{TenantID: "tenant-2"})
	ApplyTenantInvalidation(payload)

	if len(pub.events) != 0 {
		t.Fatalf("apply published %d event(s); only the write surface may publish", len(pub.events))
	}
}

// The write surface DOES publish, on the agreed subject.
func TestInvalidateTenantBroadcasts(t *testing.T) {
	t.Cleanup(func() { SetTenantInvalidationPublisher(nil) })
	pub := &capturingTenantPublisher{}
	SetTenantInvalidationPublisher(pub)

	(&MultitenantService{}).InvalidateTenant("tenant-3")

	if pub.subject != TenantInvalidationSubject {
		t.Fatalf("subject %q, want %q", pub.subject, TenantInvalidationSubject)
	}
	if len(pub.events) != 1 || pub.events[0].TenantID != "tenant-3" {
		t.Fatalf("events: %+v", pub.events)
	}
}

// No publisher, or one that is down, must never fail the write that already
// succeeded — the other replicas still heal on the TTL.
func TestTenantInvalidationSurvivesABrokenPublisher(t *testing.T) {
	t.Cleanup(func() { SetTenantInvalidationPublisher(nil) })

	SetTenantInvalidationPublisher(nil)
	(&MultitenantService{}).InvalidateTenant("tenant-4")

	SetTenantInvalidationPublisher(&capturingTenantPublisher{err: errors.New("broker down")})
	(&MultitenantService{}).InvalidateTenant("tenant-4")
}

// The subject is public to anything on the bus, so a malformed or empty payload
// is ignored rather than panicked on — and an empty tenant must never clear an
// entry it does not name.
func TestMalformedTenantInvalidationIsIgnored(t *testing.T) {
	getTenantCache().put(repository.CoreTenant{TenantID: "tenant-5"})

	ApplyTenantInvalidation([]byte("{not json"))
	ApplyTenantInvalidation([]byte(`{"tenant_id":""}`))

	if !cached("tenant-5") {
		t.Fatal("a malformed event cleared an unrelated entry")
	}
}
