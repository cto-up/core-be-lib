package service

import (
	"context"
	"sync"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/util"
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
