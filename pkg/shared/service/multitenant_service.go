package service

import (
	"context"
	"sync"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/google/uuid"
)

type TenantMap struct {
	sync.RWMutex
	data map[string]string
}

var tenantMapInstance *TenantMap
var once sync.Once

// GetTenantMap returns the singleton instance of TenantMap
func GetTenantMap() *TenantMap {
	once.Do(func() {
		tenantMapInstance = &TenantMap{
			data: make(map[string]string),
		}
	})
	return tenantMapInstance
}

type MultitenantService struct {
	store *db.Store
}

func NewMultitenantService(store *db.Store) *MultitenantService {
	return &MultitenantService{store: store}
}

// GetStore returns the database store
func (uh *MultitenantService) GetStore() *db.Store {
	return uh.store
}

// Map subdomain to tenant ID
func (uh *MultitenantService) GetTenantIDWithSubdomain(ctx context.Context, subdomain string) (string, error) {
	if util.IsAdminSubdomain(subdomain) || subdomain == "auth" {
		return "", nil
	}

	tenantMap := GetTenantMap()

	tenantMap.RLock()
	tenantID, exists := tenantMap.data[subdomain]
	tenantMap.RUnlock()

	// If the subdomain is not found in the map, return an error
	if !exists {
		tenant, err := uh.store.GetTenantBySubdomain(ctx, subdomain)
		if err != nil {
			return "", err
		}

		tenantID = tenant.TenantID
		tenantMap.Lock()
		tenantMap.data[tenant.Subdomain] = tenantID
		tenantMap.Unlock()
	}
	return tenantID, nil
}

// IsTenantDisabled returns true when the tenant's is_disabled flag is set.
func (uh *MultitenantService) IsTenantDisabled(ctx context.Context, tenantID string) (bool, error) {
	if tenantID == "" {
		return false, nil
	}
	tenant, err := uh.store.GetTenantByTenantID(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return tenant.IsDisabled, nil
}

// Check if tenant is a reseller
func (uh *MultitenantService) IsReseller(ctx context.Context, tenantID string) (bool, error) {
	if tenantID == "" {
		return false, nil
	}
	tenant, err := uh.store.GetTenantByTenantID(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return tenant.IsReseller, nil
}

// GetTenantByTenantIDCached returns the tenant record for the given tenant_id.
// Results are cached with a TTL (see DefaultTenantCacheTTL) and deduplicated
// across concurrent callers. Use InvalidateTenant / InvalidateTenantByID on
// write paths to keep the cache consistent.
func (uh *MultitenantService) GetTenantByTenantIDCached(ctx context.Context, tenantID string) (repository.CoreTenant, error) {
	return getTenantCache().get(ctx, uh.store, tenantID)
}

// GetTenantBySubdomainCached returns the tenant record for the given subdomain.
// On warm paths the subdomain→tenant_id mapping (TenantMap) and the tenant
// record cache are both hit — no DB query. On cold paths it does a single
// GetTenantBySubdomain query and populates both caches.
func (uh *MultitenantService) GetTenantBySubdomainCached(ctx context.Context, subdomain string) (repository.CoreTenant, error) {
	tenantMap := GetTenantMap()
	tenantMap.RLock()
	tenantID, hasMapping := tenantMap.data[subdomain]
	tenantMap.RUnlock()

	if hasMapping {
		return getTenantCache().get(ctx, uh.store, tenantID)
	}

	tenant, err := uh.store.GetTenantBySubdomain(ctx, subdomain)
	if err != nil {
		return repository.CoreTenant{}, err
	}
	tenantMap.Lock()
	tenantMap.data[tenant.Subdomain] = tenant.TenantID
	tenantMap.Unlock()
	getTenantCache().put(tenant)
	return tenant, nil
}

// InvalidateTenant removes the cached tenant record for tenant_id.
func (uh *MultitenantService) InvalidateTenant(tenantID string) {
	getTenantCache().invalidate(tenantID)
}

// InvalidateTenantByID looks up the tenant_id for the given internal ID and
// removes the cached entry. Best-effort: on lookup failure the TTL will heal
// the cache eventually.
func (uh *MultitenantService) InvalidateTenantByID(ctx context.Context, id uuid.UUID) {
	tenant, err := uh.store.GetTenantByID(ctx, id)
	if err != nil {
		return
	}
	getTenantCache().invalidate(tenant.TenantID)
}

// IsActingReseller checks if a tenant is managed by a reseller.
// A tenant is reseller-managed when its reseller_id points to a tenant with is_reseller=true.
func (uh *MultitenantService) IsActingReseller(ctx context.Context, tenantID string) (bool, error) {
	if tenantID == "" {
		return false, nil
	}
	tenant, err := uh.store.GetTenantByTenantID(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if !tenant.ResellerID.Valid || tenant.ResellerID.String == "" {
		return false, nil
	}
	resellerTenant, err := uh.store.GetTenantByTenantID(ctx, tenant.ResellerID.String)
	if err != nil {
		return false, err
	}
	return resellerTenant.IsReseller, nil
}
