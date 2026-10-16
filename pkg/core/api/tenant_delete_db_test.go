//go:build testutils

package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/service"
)

type deletingTenantManager struct{ auth.TenantManager }

func (deletingTenantManager) DeleteTenant(context.Context, string) error { return nil }

type tenantManagerProvider struct{ auth.AuthProvider }

func (tenantManagerProvider) GetTenantManager() auth.TenantManager { return deletingTenantManager{} }

// publishedWhileRowExisted records, at the moment of the broadcast, whether
// another replica reloading the tenant would still find its row.
type publishedWhileRowExisted struct {
	store interface {
		GetTenantByTenantID(context.Context, string) (repository.CoreTenant, error)
	}
	tenantID  string
	published bool
	rowExists bool
}

func (p *publishedWhileRowExisted) Publish(string, []byte) error {
	p.published = true
	_, err := p.store.GetTenantByTenantID(context.Background(), p.tenantID)
	p.rowExists = err == nil
	return nil
}

// hub#557. The invalidation went out before the row was deleted, so another
// replica could re-cache the tenant and serve it for the whole cache TTL.
func TestDeleteTenant_BroadcastsOnlyOnceTheRowIsGone(t *testing.T) {
	store := testutils.NewTestStore(t)
	tenant, err := store.CreateTenant(context.Background(), repository.CreateTenantParams{
		UserID: "owner", TenantID: "doomed", Name: "doomed", Subdomain: "doomed",
	})
	require.NoError(t, err)

	pub := &publishedWhileRowExisted{store: store, tenantID: tenant.TenantID}
	service.SetTenantInvalidationPublisher(pub)
	t.Cleanup(func() { service.SetTenantInvalidationPublisher(nil) })

	h := &TenantHandler{authProvider: tenantManagerProvider{}, multiTenantService: service.NewMultitenantService(store), store: store}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/superadmin-api/v1/tenants/"+tenant.ID.String(), nil)
	c.Set(auth.AUTH_CLAIMS, map[string]interface{}{"SUPER_ADMIN": true})

	h.DeleteTenant(c, tenant.ID)

	require.Equal(t, http.StatusNoContent, c.Writer.Status(), w.Body.String())
	require.True(t, pub.published, "the deletion must be broadcast")
	require.False(t, pub.rowExists, "the broadcast went out while the row still existed")
}
