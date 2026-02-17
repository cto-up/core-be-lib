package service

import (
	"context"
	"sync"

	"ctoup.com/coreapp/pkg/core/db"
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
	if subdomain == "" || subdomain == "www" || subdomain == "auth" {
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
