package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
)

func mapped(subdomain string) (string, bool) {
	m := GetTenantMap()
	m.RLock()
	defer m.RUnlock()
	id, ok := m.data[subdomain]
	return id, ok
}

func mapSubdomain(subdomain, tenantID string) {
	m := GetTenantMap()
	m.Lock()
	m.data[subdomain] = tenantID
	m.Unlock()
}

// hub#558. Invalidation cleared the tenant record but never the
// subdomain→tenant_id map, so a renamed subdomain kept resolving to the tenant
// for the life of the process, on every replica.
func TestInvalidatingATenantForgetsItsSubdomains(t *testing.T) {
	t.Cleanup(func() { SetTenantInvalidationPublisher(nil) })
	mapSubdomain("acme-558", "tenant-558")
	mapSubdomain("acme-558-alias", "tenant-558")
	mapSubdomain("other-558", "tenant-other")

	(&MultitenantService{}).InvalidateTenant("tenant-558")

	for _, sub := range []string{"acme-558", "acme-558-alias"} {
		if _, ok := mapped(sub); ok {
			t.Errorf("%s still maps to the invalidated tenant", sub)
		}
	}
	if id, ok := mapped("other-558"); !ok || id != "tenant-other" {
		t.Error("another tenant's mapping was dropped")
	}
}

func TestAnInvalidationFromAnotherReplicaForgetsTheSubdomain(t *testing.T) {
	mapSubdomain("remote-558", "tenant-remote")
	payload, _ := json.Marshal(TenantInvalidationEvent{TenantID: "tenant-remote"})

	ApplyTenantInvalidation(payload)

	if _, ok := mapped("remote-558"); ok {
		t.Error("the subdomain survived an invalidation from another replica")
	}
}

func TestARenamedSubdomainStopsResolvingAndCanBeTakenAgain(t *testing.T) {
	store := testutils.NewTestStore(t)
	ctx := context.Background()
	svc := NewMultitenantService(store)
	create := func(id, subdomain string) {
		_, err := store.CreateTenant(ctx, repository.CreateTenantParams{UserID: "owner", TenantID: id, Name: id, Subdomain: subdomain})
		require.NoError(t, err)
	}

	create("first-558", "rename-558")
	got, err := svc.GetTenantBySubdomainCached(ctx, "rename-558")
	require.NoError(t, err)
	require.Equal(t, "first-558", got.TenantID)

	_, err = store.ConnPool.Exec(ctx, `UPDATE core_tenants SET subdomain = 'renamed-558' WHERE tenant_id = 'first-558'`)
	require.NoError(t, err)
	svc.InvalidateTenant("first-558")

	_, err = svc.GetTenantBySubdomainCached(ctx, "rename-558")
	require.True(t, errors.Is(err, pgx.ErrNoRows), "the old subdomain still resolves: %v", err)

	create("second-558", "rename-558")
	got, err = svc.GetTenantBySubdomainCached(ctx, "rename-558")
	require.NoError(t, err)
	require.Equal(t, "second-558", got.TenantID, "the freed subdomain routes to the old tenant")
}
