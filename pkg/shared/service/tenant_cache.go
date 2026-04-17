package service

import (
	"context"
	"sync"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"golang.org/x/sync/singleflight"
)

// DefaultTenantCacheTTL is the time-to-live for a cached tenant record.
// Direct DB mutations that bypass MultitenantService.InvalidateTenant
// self-heal after this window.
const DefaultTenantCacheTTL = 60 * time.Second

type tenantCacheEntry struct {
	tenant    repository.CoreTenant
	expiresAt time.Time
}

type tenantCache struct {
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]tenantCacheEntry
	sf      singleflight.Group
}

var (
	tenantCacheInstance *tenantCache
	tenantCacheOnce     sync.Once
)

func getTenantCache() *tenantCache {
	tenantCacheOnce.Do(func() {
		tenantCacheInstance = &tenantCache{
			ttl:     DefaultTenantCacheTTL,
			entries: make(map[string]tenantCacheEntry),
		}
		go tenantCacheInstance.janitor(DefaultTenantCacheTTL)
	})
	return tenantCacheInstance
}

func (c *tenantCache) get(ctx context.Context, store *db.Store, tenantID string) (repository.CoreTenant, error) {
	c.mu.RLock()
	e, ok := c.entries[tenantID]
	c.mu.RUnlock()
	if ok && time.Now().Before(e.expiresAt) {
		return e.tenant, nil
	}

	val, err, _ := c.sf.Do(tenantID, func() (any, error) {
		// Double-check: another goroutine may have populated the cache
		// while we were waiting on singleflight.
		c.mu.RLock()
		e, ok := c.entries[tenantID]
		c.mu.RUnlock()
		if ok && time.Now().Before(e.expiresAt) {
			return e.tenant, nil
		}

		tenant, err := store.GetTenantByTenantID(ctx, tenantID)
		if err != nil {
			return repository.CoreTenant{}, err
		}
		c.put(tenant)
		return tenant, nil
	})
	if err != nil {
		return repository.CoreTenant{}, err
	}
	return val.(repository.CoreTenant), nil
}

// janitor periodically sweeps expired entries to bound memory growth when the
// set of tenant_ids accessed over the process lifetime is unbounded (deleted
// tenants, transient IDs, etc.). Invoked once per process from getTenantCache.
func (c *tenantCache) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for id, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, id)
			}
		}
		c.mu.Unlock()
	}
}

func (c *tenantCache) put(tenant repository.CoreTenant) {
	c.mu.Lock()
	c.entries[tenant.TenantID] = tenantCacheEntry{
		tenant:    tenant,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *tenantCache) invalidate(tenantID string) {
	c.mu.Lock()
	delete(c.entries, tenantID)
	c.mu.Unlock()
}
