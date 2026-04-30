package service

import (
	"context"
	"sync"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetTenantFromContext returns the CoreTenant cached on the gin.Context by
// TenantMiddleware. Returns (zero, false) when the request did not pass
// through the middleware or the tenant is unset (root domain, public route).
func GetTenantFromContext(c *gin.Context) (repository.CoreTenant, bool) {
	v, exists := c.Get(auth.AUTH_TENANT)
	if !exists {
		return repository.CoreTenant{}, false
	}
	t, ok := v.(repository.CoreTenant)
	if !ok {
		return repository.CoreTenant{}, false
	}
	return t, true
}

// loadTenantPreferContext is the canonical fast-path tenant lookup for the
// MultitenantService helpers. When the caller's context is a *gin.Context
// populated by TenantMiddleware, it returns the already-loaded tenant for
// free; otherwise it falls back to the singleflight-deduped in-process cache.
func (uh *MultitenantService) loadTenantPreferContext(ctx context.Context, tenantID string) (repository.CoreTenant, error) {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		if tenant, ok := GetTenantFromContext(ginCtx); ok && tenant.TenantID == tenantID {
			return tenant, nil
		}
	}
	return getTenantCache().get(ctx, uh.store, tenantID)
}

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

// Map subdomain to tenant ID. On a cold subdomain miss the loaded tenant is
// also written into the tenant cache so a subsequent GetTenantByTenantIDCached
// in the same request does not re-query the DB.
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
		// Warm the tenant-record cache too — the caller almost always wants the
		// full tenant next (tenant_middleware, auth flow), and skipping this
		// would cost a redundant DB round-trip.
		getTenantCache().put(tenant)
	}
	return tenantID, nil
}

// GetTenantAllowSignUp returns the tenant's AllowSignUp flag.
func (uh *MultitenantService) GetTenantAllowSignUp(ctx context.Context, tenantID string) (bool, error) {
	if tenantID == "" {
		return false, nil
	}
	tenant, err := uh.loadTenantPreferContext(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return tenant.AllowSignUp, nil
}

// IsTenantDisabled returns true when the tenant's is_disabled flag is set.
func (uh *MultitenantService) IsTenantDisabled(ctx context.Context, tenantID string) (bool, error) {
	if tenantID == "" {
		return false, nil
	}
	tenant, err := uh.loadTenantPreferContext(ctx, tenantID)
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
	tenant, err := uh.loadTenantPreferContext(ctx, tenantID)
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
	tenant, err := uh.loadTenantPreferContext(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if !tenant.ResellerID.Valid || tenant.ResellerID.String == "" {
		return false, nil
	}
	// The reseller's tenant is a different row — fall through to the cached
	// lookup (it won't be on the gin context).
	resellerTenant, err := uh.GetTenantByTenantIDCached(ctx, tenant.ResellerID.String)
	if err != nil {
		return false, err
	}
	return resellerTenant.IsReseller, nil
}
